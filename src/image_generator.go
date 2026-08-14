// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type imageGenerationPlan struct {
	DestinationFull string
	Provider        string
	Endpoint        string
	Format          string
	Width           int
	Height          int
	Prompt          string
}

func validateImageGenerationRequest(project string, cfg Config, path, prompt string, width, height int) (imageGenerationPlan, error) {
	destinationFull, err := ensureWithinRoot(project, path)
	if err != nil {
		return imageGenerationPlan{}, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return imageGenerationPlan{}, errors.New("generate_image_asset requires a non-empty prompt in content")
	}
	if len([]byte(prompt)) > 16<<10 {
		return imageGenerationPlan{}, errors.New("image generation prompt exceeds 16 KiB")
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.ImageGeneratorProvider))
	if provider == "" {
		provider = "automatic1111"
	}
	if provider == "disabled" {
		return imageGenerationPlan{}, errors.New("image generation provider is disabled")
	}
	if provider != "automatic1111" {
		return imageGenerationPlan{}, fmt.Errorf("unsupported image generation provider: %s", provider)
	}
	baseURL, err := normalizeLocalImageGeneratorURL(cfg.ImageGeneratorURL)
	if err != nil {
		return imageGenerationPlan{}, err
	}
	width, height = normalizeGeneratedImageDimensions(width, height)
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	format := strings.TrimPrefix(ext, ".")
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".ico":
	default:
		return imageGenerationPlan{}, errors.New("generate_image_asset destination must be .png, .jpg, .jpeg, .webp, or .ico")
	}
	return imageGenerationPlan{
		DestinationFull: destinationFull,
		Provider:        provider,
		Endpoint:        baseURL + "/sdapi/v1/txt2img",
		Format:          format,
		Width:           width,
		Height:          height,
		Prompt:          prompt,
	}, nil
}

func normalizeGeneratedImageDimensions(width, height int) (int, int) {
	if width <= 0 {
		width = 512
	}
	if height <= 0 {
		height = 512
	}
	if width < 64 {
		width = 64
	}
	if height < 64 {
		height = 64
	}
	if width > 2048 {
		width = 2048
	}
	if height > 2048 {
		height = 2048
	}
	return width, height
}

func normalizeLocalImageGeneratorURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "http://127.0.0.1:7860"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid image generator URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("image generator URL must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("image generator URL must not contain credentials")
	}
	host := parsed.Hostname()
	if !isLoopbackHost(host) {
		return "", errors.New("image generator URL must point to localhost, 127.0.0.1, or ::1")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func generateImageAsset(ctx context.Context, project string, cfg Config, path, prompt string, width, height int) (string, error) {
	plan, err := validateImageGenerationRequest(project, cfg, path, prompt, width, height)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}
	genCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	steps := cfg.ImageGeneratorSteps
	if steps < 1 || steps > 80 {
		steps = 20
	}
	cfgScale := cfg.ImageGeneratorCFGScale
	if cfgScale < 1 || cfgScale > 30 {
		cfgScale = 7.0
	}
	payload := map[string]any{
		"prompt":      plan.Prompt,
		"steps":       steps,
		"cfg_scale":   cfgScale,
		"width":       plan.Width,
		"height":      plan.Height,
		"batch_size":  1,
		"n_iter":      1,
		"send_images": true,
		"save_images": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(genCtx, http.MethodPost, plan.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("local image generator request failed: %w", err)
	}
	defer resp.Body.Close()
	responseData, err := io.ReadAll(io.LimitReader(resp.Body, (maxRasterAssetBytes*2)+1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("local image generator returned HTTP %d: %s", resp.StatusCode, truncateText(string(responseData), 2000))
	}
	var decoded struct {
		Images []string `json:"images"`
	}
	if err := json.Unmarshal(responseData, &decoded); err != nil {
		return "", fmt.Errorf("invalid local image generator JSON response: %w", err)
	}
	if len(decoded.Images) == 0 || strings.TrimSpace(decoded.Images[0]) == "" {
		return "", errors.New("local image generator returned no image")
	}
	imageData, _, err := validateImageAsset("generated.png", decoded.Images[0])
	if err != nil {
		return "", fmt.Errorf("local image generator returned invalid image data: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return "", fmt.Errorf("generated image cannot be decoded: %w", err)
	}
	var output []byte
	var info imageAssetInfo
	var rendererDetail string
	if strings.EqualFold(filepath.Ext(path), ".webp") {
		output, info, rendererDetail, err = encodeWebPForDestination(genCtx, cfg, img, path, plan.Width, plan.Height)
	} else {
		output, info, err = encodeImageForDestination(img, path, plan.Width, plan.Height)
	}
	if err != nil {
		return "", err
	}
	result, err := writeBinaryProjectFile(project, path, output)
	if err != nil {
		return "", err
	}
	if rendererDetail != "" {
		rendererDetail = "\nRenderer: " + rendererDetail
	}
	return fmt.Sprintf("IMAGE ASSET GENERATED\nProvider: %s\nEndpoint: %s\nValidation: format=%s dimensions=%dx%d bytes=%d%s\n\n%s", plan.Provider, plan.Endpoint, info.Format, info.Width, info.Height, info.Bytes, rendererDetail, result), nil
}

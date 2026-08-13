// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxRasterAssetBytes = 16 << 20

var renderPNGWithChromium = renderPNGWithChromiumDefault
var renderWebPWithChromium = renderWebPWithChromiumDefault

func validateSVGAsset(path, content string) error {
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".svg") {
		return errors.New("create_svg_asset requires a .svg path")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("svg content is empty")
	}
	if len(content) > 4<<20 {
		return errors.New("svg content exceeds 4 MiB")
	}
	decoder := xml.NewDecoder(strings.NewReader(content))
	foundRoot := false
	hasSize := false
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("invalid SVG XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		isRoot := !foundRoot
		if !foundRoot {
			if name != "svg" {
				return errors.New("svg root element must be <svg>")
			}
			foundRoot = true
		}
		if name == "script" {
			return errors.New("svg scripts are not allowed")
		}
		for _, attr := range start.Attr {
			attrName := strings.ToLower(attr.Name.Local)
			attrValue := strings.ToLower(strings.TrimSpace(attr.Value))
			if isRoot && (attrName == "viewbox" || attrName == "width" || attrName == "height") {
				hasSize = true
			}
			if strings.HasPrefix(attrName, "on") || strings.Contains(attrValue, "javascript:") {
				return errors.New("svg event handlers and javascript URLs are not allowed")
			}
		}
	}
	if !foundRoot {
		return errors.New("svg root element is missing")
	}
	if !hasSize {
		return errors.New("svg must define viewBox, width, or height")
	}
	return nil
}

func createSVGAsset(project, path, content string) (string, error) {
	if err := validateSVGAsset(path, content); err != nil {
		return "", err
	}
	result, err := writeProjectFile(project, path, strings.TrimSpace(content)+"\n")
	if err != nil {
		return "", err
	}
	return "SVG ASSET CREATED\nValidation: XML root <svg>, no script/event/javascript URL markers\n\n" + result, nil
}

type imageAssetInfo struct {
	Format string
	Width  int
	Height int
	Bytes  int
}

func decodeImageAssetContent(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("image content is empty")
	}
	if strings.HasPrefix(strings.ToLower(content), "data:") {
		comma := strings.Index(content, ",")
		if comma < 0 {
			return nil, errors.New("image data URL is missing comma separator")
		}
		header := strings.ToLower(content[:comma])
		if !strings.Contains(header, ";base64") {
			return nil, errors.New("image data URL must use base64 encoding")
		}
		if !strings.HasPrefix(header, "data:image/") {
			return nil, errors.New("image data URL must use an image media type")
		}
		content = content[comma+1:]
	}
	compact := strings.Join(strings.Fields(content), "")
	if compact == "" {
		return nil, errors.New("image base64 payload is empty")
	}
	data, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(compact)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid image base64 content: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("decoded image content is empty")
	}
	if len(data) > maxRasterAssetBytes {
		return nil, fmt.Errorf("decoded image content exceeds %d bytes", maxRasterAssetBytes)
	}
	return data, nil
}

func validateImageAsset(path, content string) ([]byte, imageAssetInfo, error) {
	data, err := decodeImageAssetContent(content)
	if err != nil {
		return nil, imageAssetInfo{}, err
	}
	info, err := inspectImageAsset(path, data)
	if err != nil {
		return nil, imageAssetInfo{}, err
	}
	return data, info, nil
}

func inspectImageAsset(path string, data []byte) (imageAssetInfo, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	info := imageAssetInfo{Bytes: len(data)}
	switch ext {
	case ".png":
		if !bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
			return info, errors.New("png asset has invalid PNG signature")
		}
		info.Format = "png"
		return decodeStandardImageConfig(data, info)
	case ".jpg", ".jpeg":
		if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
			return info, errors.New("jpeg asset has invalid JPEG signature")
		}
		info.Format = "jpeg"
		return decodeStandardImageConfig(data, info)
	case ".gif":
		if !bytes.HasPrefix(data, []byte("GIF87a")) && !bytes.HasPrefix(data, []byte("GIF89a")) {
			return info, errors.New("gif asset has invalid GIF signature")
		}
		info.Format = "gif"
		return decodeStandardImageConfig(data, info)
	case ".bmp":
		if !bytes.HasPrefix(data, []byte("BM")) {
			return info, errors.New("bmp asset has invalid BMP signature")
		}
		info.Format = "bmp"
		width, height, err := bmpDimensions(data)
		info.Width, info.Height = width, height
		return validateImageDimensions(info, err)
	case ".ico":
		info.Format = "ico"
		width, height, err := icoDimensions(data)
		info.Width, info.Height = width, height
		return validateImageDimensions(info, err)
	case ".webp":
		info.Format = "webp"
		width, height, err := webpDimensions(data)
		info.Width, info.Height = width, height
		return validateImageDimensions(info, err)
	default:
		return info, errors.New("create_image_asset requires a .png, .jpg, .jpeg, .gif, .webp, .ico, or .bmp path")
	}
}

func decodeStandardImageConfig(data []byte, info imageAssetInfo) (imageAssetInfo, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return info, fmt.Errorf("invalid %s image data: %w", info.Format, err)
	}
	info.Width = cfg.Width
	info.Height = cfg.Height
	return validateImageDimensions(info, nil)
}

func validateImageDimensions(info imageAssetInfo, err error) (imageAssetInfo, error) {
	if err != nil {
		return info, err
	}
	if info.Width <= 0 || info.Height <= 0 {
		return info, fmt.Errorf("%s asset has invalid dimensions %dx%d", info.Format, info.Width, info.Height)
	}
	if info.Width > 16384 || info.Height > 16384 {
		return info, fmt.Errorf("%s asset dimensions exceed 16384px: %dx%d", info.Format, info.Width, info.Height)
	}
	return info, nil
}

func bmpDimensions(data []byte) (int, int, error) {
	if len(data) < 26 {
		return 0, 0, errors.New("bmp asset is too small")
	}
	width := int(int32(binary.LittleEndian.Uint32(data[18:22])))
	height := int(int32(binary.LittleEndian.Uint32(data[22:26])))
	if height < 0 {
		height = -height
	}
	return width, height, nil
}

func icoDimensions(data []byte) (int, int, error) {
	if len(data) < 22 {
		return 0, 0, errors.New("ico asset is too small")
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return 0, 0, errors.New("ico asset has invalid ICO signature")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count <= 0 {
		return 0, 0, errors.New("ico asset contains no images")
	}
	if len(data) < 6+count*16 {
		return 0, 0, errors.New("ico asset directory is truncated")
	}
	width := int(data[6])
	height := int(data[7])
	if width == 0 {
		width = 256
	}
	if height == 0 {
		height = 256
	}
	return width, height, nil
}

func webpDimensions(data []byte) (int, int, error) {
	if len(data) < 20 || !bytes.HasPrefix(data, []byte("RIFF")) || string(data[8:12]) != "WEBP" {
		return 0, 0, errors.New("webp asset has invalid WebP RIFF signature")
	}
	offset := 12
	for offset+8 <= len(data) {
		chunkType := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		payload := offset + 8
		if chunkSize < 0 || payload+chunkSize > len(data) {
			return 0, 0, errors.New("webp asset chunk is truncated")
		}
		chunk := data[payload : payload+chunkSize]
		switch chunkType {
		case "VP8X":
			if len(chunk) < 10 {
				return 0, 0, errors.New("webp VP8X chunk is too small")
			}
			width := 1 + int(chunk[4]) + int(chunk[5])<<8 + int(chunk[6])<<16
			height := 1 + int(chunk[7]) + int(chunk[8])<<8 + int(chunk[9])<<16
			return width, height, nil
		case "VP8L":
			if len(chunk) < 5 || chunk[0] != 0x2f {
				return 0, 0, errors.New("webp VP8L chunk is invalid")
			}
			bits := uint32(chunk[1]) | uint32(chunk[2])<<8 | uint32(chunk[3])<<16 | uint32(chunk[4])<<24
			width := int(bits&0x3fff) + 1
			height := int((bits>>14)&0x3fff) + 1
			return width, height, nil
		case "VP8 ":
			if len(chunk) < 10 || !bytes.Equal(chunk[3:6], []byte{0x9d, 0x01, 0x2a}) {
				return 0, 0, errors.New("webp VP8 chunk is invalid")
			}
			width := int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff)
			height := int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)
			return width, height, nil
		}
		offset = payload + chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	return 0, 0, errors.New("webp asset has no supported VP8 chunk")
}

func writeBinaryProjectFile(projectRoot, path string, data []byte) (string, error) {
	full, err := ensureWithinRoot(projectRoot, path)
	if err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(full); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("path is a directory: %s", path)
		}
		mode = info.Mode().Perm()
		if err := backupFile(projectRoot, full); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, mode); err != nil {
		return "", err
	}
	return "POSTCONDITION:\n" + describePathState("target", full), nil
}

func createImageAsset(project, path, content string) (string, error) {
	data, info, err := validateImageAsset(path, content)
	if err != nil {
		return "", err
	}
	result, err := writeBinaryProjectFile(project, path, data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("IMAGE ASSET CREATED\nValidation: format=%s dimensions=%dx%d bytes=%d\n\n%s", info.Format, info.Width, info.Height, info.Bytes, result), nil
}

type renderAssetPlan struct {
	SourceFull      string
	DestinationFull string
	SourceExt       string
	DestinationExt  string
	Width           int
	Height          int
}

func validateRenderAsset(project, source, destination string, width, height int) (renderAssetPlan, error) {
	sourceFull, err := ensureWithinRoot(project, source)
	if err != nil {
		return renderAssetPlan{}, err
	}
	destinationFull, err := ensureWithinRoot(project, destination)
	if err != nil {
		return renderAssetPlan{}, err
	}
	sourceExt := strings.ToLower(filepath.Ext(strings.TrimSpace(source)))
	destinationExt := strings.ToLower(filepath.Ext(strings.TrimSpace(destination)))
	switch sourceExt {
	case ".svg", ".html", ".htm":
	default:
		return renderAssetPlan{}, errors.New("render_asset source must be .svg, .html, or .htm")
	}
	switch destinationExt {
	case ".png", ".jpg", ".jpeg", ".ico", ".webp":
	default:
		return renderAssetPlan{}, errors.New("render_asset destination must be .png, .jpg, .jpeg, .webp, or .ico")
	}
	info, err := os.Stat(sourceFull)
	if err != nil {
		return renderAssetPlan{}, err
	}
	if info.IsDir() {
		return renderAssetPlan{}, fmt.Errorf("render_asset source is a directory: %s", source)
	}
	if info.Size() > 4<<20 {
		return renderAssetPlan{}, fmt.Errorf("render_asset source exceeds 4 MiB: %s", source)
	}
	data, err := os.ReadFile(sourceFull)
	if err != nil {
		return renderAssetPlan{}, err
	}
	if !isProbablyText(data) {
		return renderAssetPlan{}, fmt.Errorf("render_asset source must be text: %s", source)
	}
	content := string(data)
	if sourceExt == ".svg" {
		if err := validateSVGAsset(source, content); err != nil {
			return renderAssetPlan{}, err
		}
	} else if err := validateHTMLRenderSource(content); err != nil {
		return renderAssetPlan{}, err
	}
	width, height = normalizeRenderDimensions(content, sourceExt, destinationExt, width, height)
	return renderAssetPlan{SourceFull: sourceFull, DestinationFull: destinationFull, SourceExt: sourceExt, DestinationExt: destinationExt, Width: width, Height: height}, nil
}

func validateHTMLRenderSource(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return errors.New("html render source is empty")
	}
	if regexp.MustCompile(`(?i)\bhttps?://|src\s*=\s*["']\s*//|href\s*=\s*["']\s*//`).MatchString(trimmed) {
		return errors.New("html render source contains external network references")
	}
	return nil
}

func normalizeRenderDimensions(content, sourceExt, destinationExt string, width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		if sourceExt == ".svg" {
			if svgWidth, svgHeight := svgRenderDimensions(content); svgWidth > 0 && svgHeight > 0 {
				if width <= 0 {
					width = svgWidth
				}
				if height <= 0 {
					height = svgHeight
				}
			}
		}
	}
	if width <= 0 {
		if destinationExt == ".ico" {
			width = 256
		} else {
			width = 1024
		}
	}
	if height <= 0 {
		if destinationExt == ".ico" {
			height = 256
		} else {
			height = 768
		}
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if width > 4096 {
		width = 4096
	}
	if height > 4096 {
		height = 4096
	}
	if destinationExt == ".ico" && (width > 256 || height > 256) {
		if width >= height {
			height = maxRenderInt(1, height*256/width)
			width = 256
		} else {
			width = maxRenderInt(1, width*256/height)
			height = 256
		}
	}
	return width, height
}

func maxRenderInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func svgRenderDimensions(content string) (int, int) {
	re := regexp.MustCompile(`(?i)\bviewBox\s*=\s*["']\s*[-+]?[0-9.]+\s+[-+]?[0-9.]+\s+([-+]?[0-9.]+)\s+([-+]?[0-9.]+)`)
	if m := re.FindStringSubmatch(content); len(m) == 3 {
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		if w > 0 && h > 0 {
			return int(w + 0.5), int(h + 0.5)
		}
	}
	attr := func(name string) int {
		re := regexp.MustCompile(`(?i)\b` + name + `\s*=\s*["']\s*([0-9.]+)`)
		if m := re.FindStringSubmatch(content); len(m) == 2 {
			v, _ := strconv.ParseFloat(m[1], 64)
			if v > 0 {
				return int(v + 0.5)
			}
		}
		return 0
	}
	return attr("width"), attr("height")
}

func renderPNGWithChromiumDefault(ctx context.Context, cfg Config, sourceFull, targetPNG string, width, height int) (string, error) {
	sourceURI, err := fileURI(sourceFull)
	if err != nil {
		return "", err
	}
	profileDir, err := os.MkdirTemp("", "localcode-render-profile-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)
	baseArgs := []string{
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-extensions",
		"--hide-scrollbars",
		"--mute-audio",
		"--proxy-server=http://127.0.0.1:9",
		"--host-resolver-rules=MAP * 0.0.0.0",
		"--user-data-dir=" + profileDir,
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--screenshot=" + targetPNG,
		"--virtual-time-budget=1000",
		sourceURI,
	}
	var diagnostics strings.Builder
	seen := map[string]bool{}
	for _, browser := range chromiumBrowserCandidates() {
		browser = strings.TrimSpace(browser)
		key := strings.ToLower(browser)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if st, err := os.Stat(browser); err != nil || st.IsDir() {
			continue
		}
		for _, headlessFlag := range []string{"--headless=new", "--headless"} {
			args := append([]string{headlessFlag}, baseArgs...)
			cmd := exec.CommandContext(ctx, browser, args...)
			cmd.Env = commandEnvironment(cfg)
			hideCommandWindow(cmd)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return "Chromium renderer: " + browser + " " + headlessFlag, nil
			}
			fmt.Fprintf(&diagnostics, "%s %s: %v\n%s\n", browser, headlessFlag, err, strings.TrimSpace(string(out)))
		}
	}
	if diagnostics.Len() == 0 {
		return "", errors.New("no supported Chromium browser found for render_asset")
	}
	return diagnostics.String(), errors.New("Chromium render failed")
}

func renderWebPWithChromiumDefault(ctx context.Context, cfg Config, sourceFull, targetWebP string, width, height int) (string, error) {
	sourceURI, err := fileURI(sourceFull)
	if err != nil {
		return "", err
	}
	profileDir, err := os.MkdirTemp("", "localcode-render-webp-profile-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)
	var diagnostics strings.Builder
	seen := map[string]bool{}
	for _, browser := range chromiumBrowserCandidates() {
		browser = strings.TrimSpace(browser)
		key := strings.ToLower(browser)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if st, err := os.Stat(browser); err != nil || st.IsDir() {
			continue
		}
		for _, headlessFlag := range []string{"--headless=new", "--headless"} {
			if err := captureWebPWithChromiumScreenshot(ctx, cfg, browser, headlessFlag, sourceURI, profileDir, targetWebP, width, height); err == nil {
				return "Chromium renderer: " + browser + " " + headlessFlag, nil
			} else {
				fmt.Fprintf(&diagnostics, "%s %s: %v\n", browser, headlessFlag, err)
			}
		}
	}
	if diagnostics.Len() == 0 {
		return "", errors.New("no supported Chromium browser found for render_asset")
	}
	return diagnostics.String(), errors.New("Chromium WebP render failed")
}

func captureWebPWithChromiumScreenshot(ctx context.Context, cfg Config, browser, headlessFlag, sourceURI, profileDir, targetWebP string, width, height int) error {
	_ = os.Remove(targetWebP)
	args := []string{
		headlessFlag,
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-sync",
		"--disable-extensions",
		"--hide-scrollbars",
		"--mute-audio",
		"--proxy-server=http://127.0.0.1:9",
		"--host-resolver-rules=MAP * 0.0.0.0",
		"--user-data-dir=" + profileDir,
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--screenshot=" + targetWebP,
		"--virtual-time-budget=1000",
		sourceURI,
	}
	cmd := exec.CommandContext(ctx, browser, args...)
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	info, err := validateRenderedWebP(targetWebP, width, height)
	if err != nil {
		return err
	}
	if info.Format != "webp" {
		return fmt.Errorf("direct screenshot produced %s instead of webp", info.Format)
	}
	return nil
}

func validateRenderedPNG(path string, expectedWidth, expectedHeight int) (imageAssetInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return imageAssetInfo{}, err
	}
	info, err := inspectImageAsset("render.png", data)
	if err != nil {
		return imageAssetInfo{}, err
	}
	if info.Width != expectedWidth || info.Height != expectedHeight {
		return info, fmt.Errorf("rendered PNG dimensions are %dx%d, expected %dx%d", info.Width, info.Height, expectedWidth, expectedHeight)
	}
	return info, nil
}

func validateRenderedWebP(path string, expectedWidth, expectedHeight int) (imageAssetInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return imageAssetInfo{}, err
	}
	info, err := inspectImageAsset("render.webp", data)
	if err != nil {
		return imageAssetInfo{}, err
	}
	if info.Width != expectedWidth || info.Height != expectedHeight {
		return info, fmt.Errorf("rendered WebP dimensions are %dx%d, expected %dx%d", info.Width, info.Height, expectedWidth, expectedHeight)
	}
	return info, nil
}

func pngToICO(pngData []byte, width, height int) ([]byte, error) {
	if _, err := inspectImageAsset("icon.png", pngData); err != nil {
		return nil, err
	}
	if width > 256 || height > 256 {
		return nil, errors.New("ico destination supports rendered dimensions up to 256x256")
	}
	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	if width == 256 {
		out.WriteByte(0)
	} else {
		out.WriteByte(byte(width))
	}
	if height == 256 {
		out.WriteByte(0)
	} else {
		out.WriteByte(byte(height))
	}
	out.WriteByte(0)
	out.WriteByte(0)
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(32))
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(pngData)))
	_ = binary.Write(&out, binary.LittleEndian, uint32(22))
	out.Write(pngData)
	return out.Bytes(), nil
}

func decodeConvertibleImage(data []byte, source string) (image.Image, imageAssetInfo, error) {
	info, err := inspectImageAsset(source, data)
	if err != nil {
		return nil, imageAssetInfo{}, err
	}
	decodeData := data
	if strings.EqualFold(filepath.Ext(source), ".ico") {
		pngData, err := icoPNGPayload(data)
		if err != nil {
			return nil, imageAssetInfo{}, err
		}
		decodeData = pngData
	}
	img, _, err := image.Decode(bytes.NewReader(decodeData))
	if err != nil {
		return nil, imageAssetInfo{}, fmt.Errorf("source image cannot be decoded for conversion: %w", err)
	}
	return img, info, nil
}

func icoPNGPayload(data []byte) ([]byte, error) {
	if len(data) < 22 {
		return nil, errors.New("ico asset is too small")
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 || binary.LittleEndian.Uint16(data[4:6]) < 1 {
		return nil, errors.New("ico asset has invalid ICO header")
	}
	size := int(binary.LittleEndian.Uint32(data[14:18]))
	offset := int(binary.LittleEndian.Uint32(data[18:22]))
	if size <= 0 || offset < 0 || offset+size > len(data) {
		return nil, errors.New("ico asset image data is truncated")
	}
	payload := data[offset : offset+size]
	if !bytes.HasPrefix(payload, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return nil, errors.New("ico conversion currently supports PNG-in-ICO sources")
	}
	return payload, nil
}

func normalizeConvertDimensions(src image.Image, width, height int) (int, int) {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if width <= 0 {
		width = srcW
	}
	if height <= 0 {
		height = srcH
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if width > 4096 {
		width = 4096
	}
	if height > 4096 {
		height = 4096
	}
	return width, height
}

func resizeNearest(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == width && srcH == height {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := bounds.Min.Y + y*srcH/height
		for x := 0; x < width; x++ {
			sx := bounds.Min.X + x*srcW/width
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func flattenForJPEG(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Over)
	return dst
}

func encodeImageForDestination(img image.Image, destination string, width, height int) ([]byte, imageAssetInfo, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(destination)))
	width, height = normalizeConvertDimensions(img, width, height)
	if ext == ".ico" && (width > 256 || height > 256) {
		if width >= height {
			height = maxRenderInt(1, height*256/width)
			width = 256
		} else {
			width = maxRenderInt(1, width*256/height)
			height = 256
		}
	}
	img = resizeNearest(img, width, height)
	var buf bytes.Buffer
	switch ext {
	case ".png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, imageAssetInfo{}, err
		}
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(&buf, flattenForJPEG(img), &jpeg.Options{Quality: 92}); err != nil {
			return nil, imageAssetInfo{}, err
		}
	case ".ico":
		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, img); err != nil {
			return nil, imageAssetInfo{}, err
		}
		ico, err := pngToICO(pngBuf.Bytes(), width, height)
		if err != nil {
			return nil, imageAssetInfo{}, err
		}
		buf.Write(ico)
	default:
		return nil, imageAssetInfo{}, errors.New("image conversion destination must be .png, .jpg, .jpeg, or .ico")
	}
	data := buf.Bytes()
	info, err := inspectImageAsset(destination, data)
	if err != nil {
		return nil, imageAssetInfo{}, err
	}
	if info.Width != width || info.Height != height {
		return nil, info, fmt.Errorf("converted image dimensions are %dx%d, expected %dx%d", info.Width, info.Height, width, height)
	}
	return data, info, nil
}

func convertImageAsset(project, source, destination string, width, height int) (string, error) {
	sourceFull, err := ensureWithinRoot(project, source)
	if err != nil {
		return "", err
	}
	if _, err := ensureWithinRoot(project, destination); err != nil {
		return "", err
	}
	data, err := os.ReadFile(sourceFull)
	if err != nil {
		return "", err
	}
	if len(data) > maxRasterAssetBytes {
		return "", fmt.Errorf("source image exceeds %d bytes", maxRasterAssetBytes)
	}
	img, sourceInfo, err := decodeConvertibleImage(data, source)
	if err != nil {
		return "", err
	}
	out, destInfo, err := encodeImageForDestination(img, destination, width, height)
	if err != nil {
		return "", err
	}
	result, err := writeBinaryProjectFile(project, destination, out)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("IMAGE ASSET CONVERTED\nSource: %s (%s %dx%d)\nDestination: %s (%s %dx%d, %d bytes)\n\n%s", source, sourceInfo.Format, sourceInfo.Width, sourceInfo.Height, destination, destInfo.Format, destInfo.Width, destInfo.Height, len(out), result), nil
}

func renderAsset(ctx context.Context, project string, cfg Config, source, destination string, width, height int) (string, error) {
	plan, err := validateRenderAsset(project, source, destination, width, height)
	if err != nil {
		return "", err
	}
	timeout := time.Duration(cfg.CommandTimeout) * time.Second
	if timeout <= 0 || timeout > 2*time.Minute {
		timeout = 2 * time.Minute
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if plan.DestinationExt == ".webp" {
		tempWebP, err := os.CreateTemp("", "localcode-render-*.webp")
		if err != nil {
			return "", err
		}
		tempWebPPath := tempWebP.Name()
		_ = tempWebP.Close()
		defer os.Remove(tempWebPPath)
		rendererDetail, err := renderWebPWithChromium(rctx, cfg, plan.SourceFull, tempWebPPath, plan.Width, plan.Height)
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		info, err := validateRenderedWebP(tempWebPPath, plan.Width, plan.Height)
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		data, err := os.ReadFile(tempWebPPath)
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		result, err := writeBinaryProjectFile(project, destination, data)
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		return fmt.Sprintf("ASSET RENDERED\nSource: %s\nDestination: %s\nRenderer: %s\nFormat: %s\nDimensions: %dx%d\n\n%s", source, destination, strings.TrimSpace(rendererDetail), info.Format, info.Width, info.Height, result), nil
	}
	tempPNG, err := os.CreateTemp("", "localcode-render-*.png")
	if err != nil {
		return "", err
	}
	tempPNGPath := tempPNG.Name()
	_ = tempPNG.Close()
	defer os.Remove(tempPNGPath)
	rendererDetail, err := renderPNGWithChromium(rctx, cfg, plan.SourceFull, tempPNGPath, plan.Width, plan.Height)
	if err != nil {
		return strings.TrimSpace(rendererDetail), err
	}
	info, err := validateRenderedPNG(tempPNGPath, plan.Width, plan.Height)
	if err != nil {
		return strings.TrimSpace(rendererDetail), err
	}
	data, err := os.ReadFile(tempPNGPath)
	if err != nil {
		return strings.TrimSpace(rendererDetail), err
	}
	if plan.DestinationExt != ".png" {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		encoded, encodedInfo, err := encodeImageForDestination(img, destination, plan.Width, plan.Height)
		if err != nil {
			return strings.TrimSpace(rendererDetail), err
		}
		data = encoded
		info = encodedInfo
	}
	result, err := writeBinaryProjectFile(project, destination, data)
	if err != nil {
		return strings.TrimSpace(rendererDetail), err
	}
	return fmt.Sprintf("ASSET RENDERED\nSource: %s\nDestination: %s\nRenderer: %s\nFormat: %s\nDimensions: %dx%d\n\n%s", source, destination, strings.TrimSpace(rendererDetail), info.Format, info.Width, info.Height, result), nil
}

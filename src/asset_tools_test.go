// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tinyPNGBase64(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(tinyPNGBytes(t, 1, 1))
}

func tinyPNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if width > 0 && height > 0 {
		img = image.NewNRGBA(image.Rect(0, 0, width, height))
	}
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fakeWebPBytes(width, height int) []byte {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	w := width - 1
	h := height - 1
	data := []byte{
		'R', 'I', 'F', 'F',
		22, 0, 0, 0,
		'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X',
		10, 0, 0, 0,
		0, 0, 0, 0,
		byte(w), byte(w >> 8), byte(w >> 16),
		byte(h), byte(h >> 8), byte(h >> 16),
	}
	return data
}

func withFakeRenderer(t *testing.T) {
	t.Helper()
	originalPNG := renderPNGWithChromium
	originalWebP := renderWebPWithChromium
	renderPNGWithChromium = func(ctx context.Context, cfg Config, sourceFull, targetPNG string, width, height int) (string, error) {
		return "fake chromium", os.WriteFile(targetPNG, tinyPNGBytes(t, width, height), 0o644)
	}
	renderWebPWithChromium = func(ctx context.Context, cfg Config, sourceFull, targetWebP string, width, height int) (string, error) {
		return "fake chromium webp", os.WriteFile(targetWebP, fakeWebPBytes(width, height), 0o644)
	}
	t.Cleanup(func() {
		renderPNGWithChromium = originalPNG
		renderWebPWithChromium = originalWebP
	})
}

func TestCreateSVGAssetWritesValidatedFile(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><circle cx="32" cy="32" r="24" fill="#f6c945"/></svg>`

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "create_svg_asset", Path: "assets/icon.svg", Content: svg})
	if err != nil {
		t.Fatalf("create_svg_asset failed: %v", err)
	}
	if !strings.Contains(result, "SVG ASSET CREATED") || !strings.Contains(result, "POSTCONDITION") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "icon.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<circle") {
		t.Fatalf("svg content was not written: %s", string(data))
	}
}

func TestCreateSVGAssetRejectsUnsafeOrInvalidContent(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content string
	}{
		{"wrong extension", "assets/icon.png", `<svg viewBox="0 0 1 1"></svg>`},
		{"missing size", "assets/icon.svg", `<svg><rect width="1" height="1"/></svg>`},
		{"script element", "assets/icon.svg", `<svg viewBox="0 0 1 1"><script>alert(1)</script></svg>`},
		{"event handler", "assets/icon.svg", `<svg viewBox="0 0 1 1" onload="alert(1)"></svg>`},
		{"javascript url", "assets/icon.svg", `<svg viewBox="0 0 1 1"><a href="javascript:alert(1)"/></svg>`},
		{"bad xml", "assets/icon.svg", `<svg viewBox="0 0 1 1"><rect></svg>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSVGAsset(tc.path, tc.content); err == nil {
				t.Fatal("expected SVG validation error")
			}
		})
	}
}

func TestParseAgentActionCreateSVGAsset(t *testing.T) {
	if _, err := parseAgentAction(`{"action":"create_svg_asset","message":"icon","path":"assets/icon.svg"}`); err == nil {
		t.Fatal("create_svg_asset without content must be rejected")
	}
	a, err := parseAgentAction(`{"action":"create_svg_asset","message":"icon","arguments":{"path":"assets/icon.svg","content":"<svg viewBox=\"0 0 1 1\"></svg>"}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.Path != "assets/icon.svg" || !strings.Contains(a.Content, "<svg") {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
}

func TestCreateImageAssetWritesValidatedPNG(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	content := "data:image/png;base64," + tinyPNGBase64(t)

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "create_image_asset", Path: "assets/pixel.png", Content: content})
	if err != nil {
		t.Fatalf("create_image_asset failed: %v", err)
	}
	if !strings.Contains(result, "IMAGE ASSET CREATED") || !strings.Contains(result, "dimensions=1x1") || !strings.Contains(result, "POSTCONDITION") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "pixel.png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("written png is not decodable: %v", err)
	}
}

func TestCreateImageAssetRejectsUnsafeOrInvalidContent(t *testing.T) {
	pngData := tinyPNGBase64(t)
	cases := []struct {
		name    string
		path    string
		content string
	}{
		{"wrong extension", "assets/pixel.svg", pngData},
		{"mismatched signature", "assets/pixel.gif", pngData},
		{"bad base64", "assets/pixel.png", "!not-base64!"},
		{"non image data", "assets/pixel.png", base64.StdEncoding.EncodeToString([]byte("not an image"))},
		{"non base64 data url", "assets/pixel.png", "data:image/png,abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := validateImageAsset(tc.path, tc.content); err == nil {
				t.Fatal("expected image validation error")
			}
		})
	}
}

func TestParseAgentActionCreateImageAsset(t *testing.T) {
	if _, err := parseAgentAction(`{"action":"create_image_asset","message":"icon","path":"assets/icon.png"}`); err == nil {
		t.Fatal("create_image_asset without content must be rejected")
	}
	a, err := parseAgentAction(`{"action":"create_image_asset","message":"icon","arguments":{"path":"assets/icon.png","content":"aW1hZ2U="}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.Path != "assets/icon.png" || a.Content != "aW1hZ2U=" {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
}

func TestGenerateImageAssetUsesLocalAutomatic1111API(t *testing.T) {
	project := t.TempDir()
	var gotRequest struct {
		Prompt    string  `json:"prompt"`
		Steps     int     `json:"steps"`
		CFGScale  float64 `json:"cfg_scale"`
		Width     int     `json:"width"`
		Height    int     `json:"height"`
		BatchSize int     `json:"batch_size"`
		NIter     int     `json:"n_iter"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdapi/v1/txt2img" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"images": []string{base64.StdEncoding.EncodeToString(tinyPNGBytes(t, 1, 1))},
		})
	}))
	defer server.Close()

	cfg := defaultConfig()
	cfg.ImageGeneratorProvider = "automatic1111"
	cfg.ImageGeneratorURL = server.URL
	cfg.ImageGeneratorSteps = 3
	cfg.ImageGeneratorCFGScale = 4.5
	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "generate_image_asset", Path: "assets/generated.png", Content: "small red square", Width: 64, Height: 96})
	if err != nil {
		t.Fatalf("generate_image_asset failed: %v", err)
	}
	if gotRequest.Prompt != "small red square" || gotRequest.Steps != 3 || gotRequest.CFGScale != 4.5 || gotRequest.Width != 64 || gotRequest.Height != 96 || gotRequest.BatchSize != 1 || gotRequest.NIter != 1 {
		t.Fatalf("unexpected request: %#v", gotRequest)
	}
	if !strings.Contains(result, "IMAGE ASSET GENERATED") || !strings.Contains(result, "format=png") || !strings.Contains(result, "dimensions=64x96") || !strings.Contains(result, "POSTCONDITION") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "generated.png"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("generated.png", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 64 || info.Height != 96 {
		t.Fatalf("unexpected generated dimensions: %#v", info)
	}
}

func TestGenerateImageAssetRejectsNonLocalEndpoint(t *testing.T) {
	cfg := defaultConfig()
	cfg.ImageGeneratorURL = "https://example.com:7860"
	if _, err := validateImageGenerationRequest(t.TempDir(), cfg, "out.png", "prompt", 128, 128); err == nil {
		t.Fatal("expected non-local endpoint to be rejected")
	}
}

func TestParseAgentActionGenerateImageAsset(t *testing.T) {
	if _, err := parseAgentAction(`{"action":"generate_image_asset","message":"image","path":"assets/out.png"}`); err == nil {
		t.Fatal("generate_image_asset without content must be rejected")
	}
	a, err := parseAgentAction(`{"action":"generate_image_asset","message":"image","arguments":{"path":"assets/out.webp","content":"neon icon","width":"256","height":128}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.Path != "assets/out.webp" || a.Content != "neon icon" || a.Width != 256 || a.Height != 128 {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
}

func TestConvertImageAssetConvertsPNGToJPEG(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	if err := os.MkdirAll(filepath.Join(project, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "assets", "source.png"), tinyPNGBytes(t, 4, 2), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "convert_image_asset", Source: "assets/source.png", Destination: "assets/source.jpg", Width: 8, Height: 4})
	if err != nil {
		t.Fatalf("convert_image_asset failed: %v", err)
	}
	if !strings.Contains(result, "IMAGE ASSET CONVERTED") || !strings.Contains(result, "jpeg 8x4") || !strings.Contains(result, "POSTCONDITION") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "source.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("source.jpg", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "jpeg" || info.Width != 8 || info.Height != 4 {
		t.Fatalf("unexpected jpeg info: %#v", info)
	}
}

func TestConvertImageAssetConvertsPNGToICO(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	if err := os.WriteFile(filepath.Join(project, "source.png"), tinyPNGBytes(t, 512, 512), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "convert_image_asset", Source: "source.png", Destination: "icon.ico"})
	if err != nil {
		t.Fatalf("convert_image_asset ico failed: %v", err)
	}
	if !strings.Contains(result, "ico 256x256") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("icon.ico", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 256 || info.Height != 256 {
		t.Fatalf("unexpected ico dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestConvertImageAssetConvertsPNGToWebP(t *testing.T) {
	withFakeRenderer(t)
	project := t.TempDir()
	cfg := defaultConfig()
	if err := os.WriteFile(filepath.Join(project, "source.png"), tinyPNGBytes(t, 4, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "convert_image_asset", Source: "source.png", Destination: "out.webp", Width: 6, Height: 3})
	if err != nil {
		t.Fatalf("convert_image_asset webp failed: %v", err)
	}
	if !strings.Contains(result, "webp 6x3") || !strings.Contains(result, "Renderer: fake chromium webp") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "out.webp"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("out.webp", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 6 || info.Height != 3 {
		t.Fatalf("unexpected webp dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestParseAgentActionConvertImageAsset(t *testing.T) {
	if _, err := parseAgentAction(`{"action":"convert_image_asset","message":"convert","source":"a.png"}`); err == nil {
		t.Fatal("convert_image_asset without destination must be rejected")
	}
	a, err := parseAgentAction(`{"action":"convert_image_asset","message":"convert","arguments":{"source":"a.png","destination":"b.webp","width":"12","height":6}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.Source != "a.png" || a.Destination != "b.webp" || a.Width != 12 || a.Height != 6 {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
}

func TestRenderAssetRendersSVGToPNG(t *testing.T) {
	withFakeRenderer(t)
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 32"><rect width="64" height="32" fill="#e54"/></svg>`
	if err := os.MkdirAll(filepath.Join(project, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "assets", "source.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "render_asset", Source: "assets/source.svg", Destination: "assets/source.png"})
	if err != nil {
		t.Fatalf("render_asset failed: %v", err)
	}
	if !strings.Contains(result, "ASSET RENDERED") || !strings.Contains(result, "Dimensions: 64x32") || !strings.Contains(result, "POSTCONDITION") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "source.png"))
	if err != nil {
		t.Fatal(err)
	}
	cfgPNG, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfgPNG.Width != 64 || cfgPNG.Height != 32 {
		t.Fatalf("unexpected png dimensions: %dx%d", cfgPNG.Width, cfgPNG.Height)
	}
}

func TestRenderAssetRendersSVGToJPEG(t *testing.T) {
	withFakeRenderer(t)
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 16"><rect width="32" height="16" fill="#e54"/></svg>`
	if err := os.WriteFile(filepath.Join(project, "source.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "render_asset", Source: "source.svg", Destination: "out.jpg"})
	if err != nil {
		t.Fatalf("render_asset jpeg failed: %v", err)
	}
	if !strings.Contains(result, "Format: jpeg") || !strings.Contains(result, "Dimensions: 32x16") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "out.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("out.jpg", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 32 || info.Height != 16 {
		t.Fatalf("unexpected jpeg dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestRenderAssetCreatesICOFromRenderedPNG(t *testing.T) {
	withFakeRenderer(t)
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512"><circle cx="256" cy="256" r="220" fill="#267"/></svg>`
	if err := os.MkdirAll(filepath.Join(project, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "assets", "icon.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "render_asset", Source: "assets/icon.svg", Destination: "assets/icon.ico"})
	if err != nil {
		t.Fatalf("render_asset ico failed: %v", err)
	}
	if !strings.Contains(result, "Format: ico") || !strings.Contains(result, "Dimensions: 256x256") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "assets", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("icon.ico", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 256 || info.Height != 256 {
		t.Fatalf("unexpected ico dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestRenderAssetRendersSVGToWebP(t *testing.T) {
	withFakeRenderer(t)
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20"><rect width="40" height="20" fill="#267"/></svg>`
	if err := os.WriteFile(filepath.Join(project, "source.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := executeAction(context.Background(), project, cfg, AgentAction{Action: "render_asset", Source: "source.svg", Destination: "out.webp"})
	if err != nil {
		t.Fatalf("render_asset webp failed: %v", err)
	}
	if !strings.Contains(result, "Format: webp") || !strings.Contains(result, "Dimensions: 40x20") {
		t.Fatalf("unexpected result: %s", result)
	}
	data, err := os.ReadFile(filepath.Join(project, "out.webp"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectImageAsset("out.webp", data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 40 || info.Height != 20 {
		t.Fatalf("unexpected webp dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestRenderAssetRejectsExternalHTMLReferences(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte(`<img src="https://example.com/a.png">`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateRenderAsset(project, "index.html", "out.png", 100, 100); err == nil {
		t.Fatal("expected external HTML reference rejection")
	}
}

func TestParseAgentActionRenderAsset(t *testing.T) {
	if _, err := parseAgentAction(`{"action":"render_asset","message":"render","source":"icon.svg"}`); err == nil {
		t.Fatal("render_asset without destination must be rejected")
	}
	a, err := parseAgentAction(`{"action":"render_asset","message":"render","arguments":{"source":"icon.svg","destination":"icon.png","width":128,"height":"64"}}`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if a.Source != "icon.svg" || a.Destination != "icon.png" || a.Width != 128 || a.Height != 64 {
		t.Fatalf("arguments were not normalized: %#v", a)
	}
}

func TestRenderAssetWithInstalledChromium(t *testing.T) {
	if os.Getenv("LOCALCODE_RUN_BROWSER_RENDER_TEST") != "1" {
		t.Skip("set LOCALCODE_RUN_BROWSER_RENDER_TEST=1 to run the installed Chromium render smoke")
	}
	if len(chromiumBrowserCandidates()) == 0 {
		t.Skip("no Chromium browser candidate found")
	}
	project := t.TempDir()
	cfg := defaultConfig()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 32"><rect width="64" height="32" fill="#e54"/></svg>`
	if err := os.WriteFile(filepath.Join(project, "source.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := renderAsset(context.Background(), project, cfg, "source.svg", "out.png", 64, 32)
	if err != nil {
		t.Fatalf("installed Chromium render failed: %v\n%s", err, result)
	}
	data, err := os.ReadFile(filepath.Join(project, "out.png"))
	if err != nil {
		t.Fatal(err)
	}
	cfgPNG, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if cfgPNG.Width != 64 || cfgPNG.Height != 32 {
		t.Fatalf("unexpected installed Chromium render dimensions: %dx%d", cfgPNG.Width, cfgPNG.Height)
	}
	result, err = renderAsset(context.Background(), project, cfg, "source.svg", "out.webp", 64, 32)
	if err != nil {
		t.Fatalf("installed Chromium WebP render failed: %v\n%s", err, result)
	}
	webpData, err := os.ReadFile(filepath.Join(project, "out.webp"))
	if err != nil {
		t.Fatal(err)
	}
	webpInfo, err := inspectImageAsset("out.webp", webpData)
	if err != nil {
		t.Fatal(err)
	}
	if webpInfo.Width != 64 || webpInfo.Height != 32 {
		t.Fatalf("unexpected installed Chromium WebP dimensions: %dx%d", webpInfo.Width, webpInfo.Height)
	}
	if err := os.WriteFile(filepath.Join(project, "convert-source.png"), tinyPNGBytes(t, 16, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = convertImageAsset(context.Background(), project, cfg, "convert-source.png", "converted.webp", 32, 16)
	if err != nil {
		t.Fatalf("installed Chromium WebP conversion failed: %v\n%s", err, result)
	}
	convertedData, err := os.ReadFile(filepath.Join(project, "converted.webp"))
	if err != nil {
		t.Fatal(err)
	}
	convertedInfo, err := inspectImageAsset("converted.webp", convertedData)
	if err != nil {
		t.Fatal(err)
	}
	if convertedInfo.Width != 32 || convertedInfo.Height != 16 {
		t.Fatalf("unexpected installed Chromium converted WebP dimensions: %dx%d", convertedInfo.Width, convertedInfo.Height)
	}
}

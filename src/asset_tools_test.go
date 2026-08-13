// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
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

func withFakeRenderer(t *testing.T) {
	t.Helper()
	original := renderPNGWithChromium
	renderPNGWithChromium = func(ctx context.Context, cfg Config, sourceFull, targetPNG string, width, height int) (string, error) {
		return "fake chromium", os.WriteFile(targetPNG, tinyPNGBytes(t, width, height), 0o644)
	}
	t.Cleanup(func() { renderPNGWithChromium = original })
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
}

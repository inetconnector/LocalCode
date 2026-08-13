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
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
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

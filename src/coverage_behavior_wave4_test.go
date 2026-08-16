// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentActionValidationAndClassificationMatrix(t *testing.T) {
	invalid := []AgentAction{
		{Action: "read_file"},
		{Action: "delete_file"},
		{Action: "search_text"},
		{Action: "replace_text"},
		{Action: "replace_text", Path: "x.txt"},
		{Action: "write_file"},
		{Action: "write_file", Path: "x.txt"},
		{Action: "create_svg_asset"},
		{Action: "create_svg_asset", Path: "x.svg"},
		{Action: "create_image_asset"},
		{Action: "generate_image_asset", Path: "x.png"},
		{Action: "convert_image_asset"},
		{Action: "convert_image_asset", Source: "a.png"},
		{Action: "render_asset"},
		{Action: "render_asset", Source: "a.svg"},
		{Action: "subagent_analyze"},
		{Action: "command_read"},
		{Action: "run_tool"},
		{Action: "discover_tool"},
		{Action: "run_command"},
		{Action: "open_terminal"},
		{Action: "web_fetch"},
		{Action: "mcp_call_tool"},
		{Action: "mcp_call_tool", Server: "server"},
		{Action: "skill_read"},
		{Action: "skill_list_resources"},
		{Action: "skill_read_resource"},
		{Action: "skill_read_resource", Skill: "s"},
		{Action: "skill_copy_resource"},
		{Action: "skill_copy_resource", Skill: "s"},
		{Action: "skill_copy_resource", Skill: "s", Resource: "r"},
		{Action: "skill_run_script"},
		{Action: "skill_run_script", Skill: "s"},
		{Action: "memory_remember"},
		{Action: "memory_forget"},
	}
	for _, action := range invalid {
		if err := validateAgentAction(action); err == nil {
			t.Fatalf("expected validation error for %#v", action)
		}
	}
	valid := []AgentAction{
		{Action: "read_file", Path: "x"},
		{Action: "delete_file", Path: "x"},
		{Action: "search_text", Query: "q"},
		{Action: "replace_text", Path: "x", OldText: "old"},
		{Action: "write_file", Path: "x", Content: "new"},
		{Action: "create_svg_asset", Path: "x.svg", Content: "svg"},
		{Action: "create_image_asset", Path: "x.png", Content: "data"},
		{Action: "generate_image_asset", Path: "x.png", Content: "prompt"},
		{Action: "convert_image_asset", Source: "a.png", Destination: "b.jpg"},
		{Action: "render_asset", Source: "a.svg", Destination: "b.png"},
		{Action: "subagent_analyze", Task: "inspect"},
		{Action: "command_read", Name: "review"},
		{Action: "run_tool", Tool: "go"},
		{Action: "discover_tool", Tool: "go"},
		{Action: "run_command", Command: "go test ./..."},
		{Action: "open_terminal", Command: "go test ./..."},
		{Action: "web_fetch", URL: "https://example.invalid"},
		{Action: "mcp_call_tool", Server: "s", Tool: "t"},
		{Action: "skill_read", Skill: "s"},
		{Action: "skill_list_resources", Skill: "s"},
		{Action: "skill_read_resource", Skill: "s", Resource: "r"},
		{Action: "skill_copy_resource", Skill: "s", Resource: "r", Destination: "d"},
		{Action: "skill_run_script", Skill: "s", Script: "run.ps1"},
		{Action: "memory_remember", Content: "remember"},
		{Action: "memory_forget", MemoryID: "id"},
		{Action: "finish"},
	}
	for _, action := range valid {
		if err := validateAgentAction(action); err != nil {
			t.Fatalf("unexpected validation error for %#v: %v", action, err)
		}
	}

	u := &url.URL{Scheme: "https", Host: "example.com", Path: "/x"}
	values := map[string]any{
		"string": "value", "stringer": u, "float": float64(7), "int": 8,
		"number": json.Number("9"), "numeric_string": " 10 ", "bad": true,
	}
	if stringMapArg(nil, "x") != "" || stringMapArg(values, "missing") != "" || stringMapArg(values, "string") != "value" || stringMapArg(values, "stringer") != u.String() || stringMapArg(values, "bad") != "" {
		t.Fatal("stringMapArg branch matrix failed")
	}
	for key, want := range map[string]int{"float": 7, "int": 8, "number": 9, "numeric_string": 10} {
		if got := intMapArg(values, key); got != want {
			t.Fatalf("intMapArg(%s)=%d want %d", key, got, want)
		}
	}
	if intMapArg(nil, "x") != 0 || intMapArg(values, "missing") != 0 || intMapArg(values, "bad") != 0 {
		t.Fatal("intMapArg zero branches failed")
	}

	pathCases := []struct {
		a    AgentAction
		want []string
	}{
		{AgentAction{Action: "write_file", Path: "a"}, []string{"a"}},
		{AgentAction{Action: "convert_image_asset", Destination: "b"}, []string{"b"}},
		{AgentAction{Action: "render_asset", Destination: "c"}, []string{"c"}},
		{AgentAction{Action: "skill_copy_resource", Destination: "d"}, []string{"d"}},
		{AgentAction{Action: "skill_run_script"}, []string{"."}},
		{AgentAction{Action: "copy_path", Destination: "e"}, []string{"e"}},
		{AgentAction{Action: "move_path", Source: "f", Destination: "g"}, []string{"f", "g"}},
		{AgentAction{Action: "engine_edit"}, []string{"."}},
		{AgentAction{Action: "git_commit"}, []string{"."}},
		{AgentAction{Action: "noop"}, nil},
	}
	for _, tc := range pathCases {
		got := mutatedActionPaths(tc.a)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Fatalf("mutatedActionPaths(%s)=%v want %v", tc.a.Action, got, tc.want)
		}
	}

	verification := []struct {
		a    AgentAction
		task string
		want bool
	}{
		{AgentAction{Action: "build_project"}, "", true},
		{AgentAction{Action: "deploy_android"}, "", true},
		{AgentAction{Action: "run_tool", Tool: "go", Args: []string{"test", "./..."}}, "", true},
		{AgentAction{Action: "run_tool", Tool: "echo", Args: []string{"x"}}, "", false},
		{AgentAction{Action: "run_command", Command: "go test ./..."}, "", true},
		{AgentAction{Action: "open_terminal", Command: "echo x"}, "", false},
		{AgentAction{Action: "read_file", Path: "README.md"}, "prüfe README.md", true},
		{AgentAction{Action: "read_file", Path: "README.md"}, "ändere README.md", false},
		{AgentAction{Action: "search_text", Query: "needle"}, "prüfe ob needle vorhanden ist", true},
		{AgentAction{Action: "search_text"}, "prüfe die dateien", false},
		{AgentAction{Action: "finish"}, "prüfe", false},
	}
	for _, tc := range verification {
		if got := actionVerifiesProject(tc.a, tc.task); got != tc.want {
			t.Fatalf("actionVerifiesProject(%#v,%q)=%v want %v", tc.a, tc.task, got, tc.want)
		}
	}
}

func webPFixture(chunkType string, payload []byte) []byte {
	data := make([]byte, 12+8+len(payload))
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WEBP")
	copy(data[12:16], chunkType)
	binary.LittleEndian.PutUint32(data[16:20], uint32(len(payload)))
	copy(data[20:], payload)
	return data
}

func TestAssetValidationBranchMatrix(t *testing.T) {
	for _, tc := range []struct {
		path, content string
		wantErr       bool
	}{
		{"x.txt", `<svg viewBox="0 0 1 1"></svg>`, true},
		{"x.svg", "", true},
		{"x.svg", `<not-svg/>`, true},
		{"x.svg", `<svg viewBox="0 0 1 1"><script/></svg>`, true},
		{"x.svg", `<svg viewBox="0 0 1 1"><rect onclick="x"/></svg>`, true},
		{"x.svg", `<svg viewBox="0 0 1 1"><a href="javascript:x"/></svg>`, true},
		{"x.svg", `<svg><rect/></svg>`, true},
		{"x.svg", `<svg viewBox="0 0 10 20"><rect width="1" height="2"/></svg>`, false},
	} {
		err := validateSVGAsset(tc.path, tc.content)
		if (err != nil) != tc.wantErr {
			t.Fatalf("validateSVGAsset(%q) err=%v wantErr=%v", tc.content, err, tc.wantErr)
		}
	}
	if err := validateSVGAsset("x.svg", `<svg viewBox="0 0 1 1"><rect>`); err == nil || !strings.Contains(err.Error(), "XML") {
		t.Fatalf("expected malformed XML error, got %v", err)
	}

	for _, tc := range []struct {
		content string
		wantErr bool
	}{
		{"", true},
		{`<img src="https://example.com/x.png">`, true},
		{`<link href="//example.com/x.css">`, true},
		{`<html><body>local</body></html>`, false},
	} {
		if err := validateHTMLRenderSource(tc.content); (err != nil) != tc.wantErr {
			t.Fatalf("validateHTMLRenderSource(%q) err=%v wantErr=%v", tc.content, err, tc.wantErr)
		}
	}

	if _, err := validateImageDimensions(imageAssetInfo{Format: "x"}, errors.New("fixture")); err == nil {
		t.Fatal("expected propagated dimension error")
	}
	if _, err := validateImageDimensions(imageAssetInfo{Format: "x", Width: 0, Height: 1}, nil); err == nil {
		t.Fatal("expected zero dimension error")
	}
	if _, err := validateImageDimensions(imageAssetInfo{Format: "x", Width: 20000, Height: 1}, nil); err == nil {
		t.Fatal("expected oversized dimension error")
	}
	if info, err := validateImageDimensions(imageAssetInfo{Format: "x", Width: 2, Height: 3}, nil); err != nil || info.Width != 2 {
		t.Fatalf("valid dimensions info=%#v err=%v", info, err)
	}

	if _, _, err := bmpDimensions(make([]byte, 25)); err == nil {
		t.Fatal("small BMP must fail")
	}
	bmp := make([]byte, 26)
	copy(bmp, "BM")
	binary.LittleEndian.PutUint32(bmp[18:22], uint32(12))
	binary.LittleEndian.PutUint32(bmp[22:26], uint32(int32(-34)))
	if w, h, err := bmpDimensions(bmp); err != nil || w != 12 || h != 34 {
		t.Fatalf("BMP dimensions=%dx%d err=%v", w, h, err)
	}

	if _, _, err := icoDimensions(make([]byte, 10)); err == nil {
		t.Fatal("small ICO must fail")
	}
	badICO := make([]byte, 22)
	binary.LittleEndian.PutUint16(badICO[2:4], 2)
	if _, _, err := icoDimensions(badICO); err == nil {
		t.Fatal("bad ICO signature must fail")
	}
	emptyICO := make([]byte, 22)
	binary.LittleEndian.PutUint16(emptyICO[2:4], 1)
	if _, _, err := icoDimensions(emptyICO); err == nil {
		t.Fatal("empty ICO must fail")
	}
	truncatedICO := make([]byte, 22)
	binary.LittleEndian.PutUint16(truncatedICO[2:4], 1)
	binary.LittleEndian.PutUint16(truncatedICO[4:6], 2)
	if _, _, err := icoDimensions(truncatedICO); err == nil {
		t.Fatal("truncated ICO must fail")
	}
	ico := make([]byte, 22)
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	if w, h, err := icoDimensions(ico); err != nil || w != 256 || h != 256 {
		t.Fatalf("ICO dimensions=%dx%d err=%v", w, h, err)
	}

	if _, _, err := webpDimensions([]byte("bad")); err == nil {
		t.Fatal("bad WebP must fail")
	}
	if _, _, err := webpDimensions(webPFixture("VP8X", []byte{1})); err == nil {
		t.Fatal("short VP8X must fail")
	}
	vp8x := make([]byte, 10)
	vp8x[4], vp8x[7] = 9, 19
	if w, h, err := webpDimensions(webPFixture("VP8X", vp8x)); err != nil || w != 10 || h != 20 {
		t.Fatalf("VP8X dimensions=%dx%d err=%v", w, h, err)
	}
	if _, _, err := webpDimensions(webPFixture("VP8L", []byte{0, 0, 0, 0, 0})); err == nil {
		t.Fatal("invalid VP8L must fail")
	}
	vp8l := []byte{0x2f, 1, 0, 0, 0}
	if w, h, err := webpDimensions(webPFixture("VP8L", vp8l)); err != nil || w != 2 || h != 1 {
		t.Fatalf("VP8L dimensions=%dx%d err=%v", w, h, err)
	}
	if _, _, err := webpDimensions(webPFixture("VP8 ", make([]byte, 10))); err == nil {
		t.Fatal("invalid VP8 must fail")
	}
	vp8 := make([]byte, 10)
	copy(vp8[3:6], []byte{0x9d, 0x01, 0x2a})
	binary.LittleEndian.PutUint16(vp8[6:8], 32)
	binary.LittleEndian.PutUint16(vp8[8:10], 24)
	if w, h, err := webpDimensions(webPFixture("VP8 ", vp8)); err != nil || w != 32 || h != 24 {
		t.Fatalf("VP8 dimensions=%dx%d err=%v", w, h, err)
	}
	if _, _, err := webpDimensions(webPFixture("JUNK", []byte{1, 2})); err == nil {
		t.Fatal("unsupported WebP chunks must fail")
	}

	if w, h := svgRenderDimensions(`<svg viewBox="0 0 100.4 50.6"></svg>`); w != 100 || h != 51 {
		t.Fatalf("viewBox dimensions=%dx%d", w, h)
	}
	if w, h := svgRenderDimensions(`<svg width="42.5" height="21.4"></svg>`); w != 43 || h != 21 {
		t.Fatalf("attribute dimensions=%dx%d", w, h)
	}
	if w, h := svgRenderDimensions(`<svg></svg>`); w != 0 || h != 0 {
		t.Fatalf("empty SVG dimensions=%dx%d", w, h)
	}
	if w, h := normalizeRenderDimensions(`<svg viewBox="0 0 100 50"></svg>`, ".svg", ".png", 0, 0); w != 100 || h != 50 {
		t.Fatalf("normalized SVG dimensions=%dx%d", w, h)
	}
	if w, h := normalizeRenderDimensions("", ".html", ".ico", 1000, 500); w != 256 || h != 128 {
		t.Fatalf("normalized ICO landscape=%dx%d", w, h)
	}
	if w, h := normalizeRenderDimensions("", ".html", ".ico", 500, 1000); w != 128 || h != 256 {
		t.Fatalf("normalized ICO portrait=%dx%d", w, h)
	}
	if w, h := normalizeRenderDimensions("", ".html", ".png", 9000, 9000); w != 4096 || h != 4096 {
		t.Fatalf("normalized capped dimensions=%dx%d", w, h)
	}
	if maxRenderInt(3, 2) != 3 || maxRenderInt(2, 3) != 3 {
		t.Fatal("maxRenderInt failed")
	}

	project := t.TempDir()
	if _, err := validateRenderAsset(project, "x.txt", "x.png", 0, 0); err == nil {
		t.Fatal("unsupported render source extension must fail")
	}
	if err := os.WriteFile(filepath.Join(project, "x.svg"), []byte(`<svg viewBox="0 0 12 13"></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateRenderAsset(project, "x.svg", "x.bin", 0, 0); err == nil {
		t.Fatal("unsupported render destination must fail")
	}
	plan, err := validateRenderAsset(project, "x.svg", "out.png", 0, 0)
	if err != nil || plan.Width != 12 || plan.Height != 13 || plan.SourceExt != ".svg" || plan.DestinationExt != ".png" {
		t.Fatalf("render plan=%#v err=%v", plan, err)
	}
	if err := os.Mkdir(filepath.Join(project, "dir.svg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := validateRenderAsset(project, "dir.svg", "out.png", 0, 0); err == nil {
		t.Fatal("render directory source must fail")
	}
}

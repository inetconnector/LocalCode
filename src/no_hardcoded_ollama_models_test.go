// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoHardcodedOllamaModelsInProductionSource enforces that production Go and
// frontend assets resolve Ollama models dynamically via /api/tags, user config,
// or environment variables instead of hardcoding specific Ollama model names.
func TestNoHardcodedOllamaModelsInProductionSource(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(qwen2\.5[^\s"'` + "`" + `]*|qwen3-vl|gemma3(?::\w+)?|gemma4|llama3(?:\.\d)?(?::\w+)?|deepseek-coder[^\s"'` + "`" + `]*|codellama(?::\w+)?|minicpm-v|llava(?::\w+)?|moondream(?::\w+)?|phi3(?::\w+)?|gpt-oss:20b)\b`)

	var violations []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".go", ".html", ".js":
		default:
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if matches := forbidden.FindAllString(string(data), -1); len(matches) > 0 {
			violations = append(violations, path+": "+strings.Join(matches, ", "))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("hardcoded Ollama model names found in production source:\n%s", strings.Join(violations, "\n"))
	}
}

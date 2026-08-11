// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

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

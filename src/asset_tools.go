// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxRasterAssetBytes = 16 << 20

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

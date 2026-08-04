package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxAttachmentCount      = 20
	maxAttachmentBytes      = 32 << 20
	maxTotalAttachmentBytes = 128 << 20
	maxExtractedPerFile     = 120000
	maxExtractedTotal       = 500000
)

var imageMIMEs = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true,
}

type preparedAttachments struct {
	Images  []Attachment
	Context string
	Dir     string
}

func validateAttachments(items []Attachment) ([]Attachment, error) {
	if len(items) > maxAttachmentCount {
		return nil, fmt.Errorf("maximal %d Dateien pro Anfrage", maxAttachmentCount)
	}
	clean := make([]Attachment, 0, len(items))
	total := 0
	for i, item := range items {
		item.Name = sanitizeAttachmentName(item.Name)
		item.MIME = strings.ToLower(strings.TrimSpace(item.MIME))
		item.Data = strings.TrimSpace(item.Data)
		if item.Name == "" {
			item.Name = fmt.Sprintf("Datei-%d", i+1)
		}
		decoded, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil {
			return nil, fmt.Errorf("ungültige Dateidaten bei %s: %w", item.Name, err)
		}
		if len(decoded) == 0 {
			return nil, fmt.Errorf("Datei %s ist leer", item.Name)
		}
		if len(decoded) > maxAttachmentBytes {
			return nil, fmt.Errorf("Datei %s ist größer als %d MiB", item.Name, maxAttachmentBytes>>20)
		}
		total += len(decoded)
		if total > maxTotalAttachmentBytes {
			return nil, fmt.Errorf("Dateien sind zusammen größer als %d MiB", maxTotalAttachmentBytes>>20)
		}
		if item.MIME == "" || item.MIME == "application/octet-stream" {
			item.MIME = mime.TypeByExtension(strings.ToLower(filepath.Ext(item.Name)))
			if item.MIME == "" {
				item.MIME = "application/octet-stream"
			}
		}
		item.Size = int64(len(decoded))
		clean = append(clean, item)
	}
	return clean, nil
}

func sanitizeAttachmentName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if len(name) > 180 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > 150 {
			base = base[:150]
		}
		name = base + ext
	}
	return name
}

func attachmentNames(items []Attachment) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

func isImageAttachment(a Attachment) bool {
	return imageMIMEs[strings.ToLower(a.MIME)] || strings.HasPrefix(strings.ToLower(a.MIME), "image/")
}

func prepareAttachments(ctx context.Context, items []Attachment) (preparedAttachments, error) {
	if len(items) == 0 {
		return preparedAttachments{}, nil
	}
	base := filepath.Join(appDataDir(), "attachments", newID())
	if err := os.MkdirAll(base, 0o700); err != nil {
		return preparedAttachments{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nANGEHÄNGTE DATEIEN (%d):\n", len(items))
	extractedTotal := 0
	images := make([]Attachment, 0)
	for _, item := range items {
		raw, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil {
			return preparedAttachments{}, err
		}
		path := uniqueAttachmentPath(base, item.Name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return preparedAttachments{}, err
		}
		fmt.Fprintf(&b, "\n- %s (%s, %d Bytes)\n  Lokaler Pfad: %s\n", item.Name, item.MIME, len(raw), path)
		if isImageAttachment(item) {
			images = append(images, item)
			continue
		}
		if extractedTotal >= maxExtractedTotal {
			continue
		}
		text, kind := extractAttachmentText(ctx, item.Name, item.MIME, raw, path)
		if strings.TrimSpace(text) == "" {
			fmt.Fprintf(&b, "  Analyse: Binärdatei gespeichert; der Agent kann sie bei Bedarf mit lokalen Werkzeugen untersuchen.\n")
			continue
		}
		room := maxExtractedTotal - extractedTotal
		if room <= 0 {
			continue
		}
		if len(text) > maxExtractedPerFile {
			text = text[:maxExtractedPerFile] + "\n...[Dateiauszug gekürzt]"
		}
		if len(text) > room {
			text = text[:room] + "\n...[Gesamtauszug gekürzt]"
		}
		extractedTotal += len(text)
		fmt.Fprintf(&b, "  Extraktion: %s\n--- BEGIN %s ---\n%s\n--- END %s ---\n", kind, item.Name, text, item.Name)
	}
	return preparedAttachments{Images: images, Context: b.String(), Dir: base}, nil
}

func uniqueAttachmentPath(dir, name string) string {
	base := filepath.Join(dir, sanitizeAttachmentName(name))
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, newID()+ext)
}

func extractAttachmentText(ctx context.Context, name, mimeType string, raw []byte, path string) (string, string) {
	ext := strings.ToLower(filepath.Ext(name))
	if isTextLike(mimeType, ext, raw) {
		return normalizeText(raw), "Text"
	}
	switch ext {
	case ".docx":
		if t, err := extractOfficeXML(raw, []string{"word/document.xml", "word/header", "word/footer"}); err == nil {
			return t, "DOCX-Text"
		}
	case ".pptx":
		if t, err := extractOfficeXML(raw, []string{"ppt/slides/slide"}); err == nil {
			return t, "PPTX-Text"
		}
	case ".xlsx", ".xlsm":
		if t, err := extractOfficeXML(raw, []string{"xl/sharedStrings.xml", "xl/worksheets/sheet"}); err == nil {
			return t, "XLSX-Text"
		}
	case ".zip", ".jar", ".apk", ".aab":
		if t, err := listZip(raw); err == nil {
			return t, "Archivinhalt"
		}
	case ".pdf":
		if tool, err := exec.LookPath("pdftotext"); err == nil {
			outPath := path + ".txt"
			cmd := exec.CommandContext(ctx, tool, "-layout", path, outPath)
			hideCommandWindow(cmd)
			if err := cmd.Run(); err == nil {
				if data, err := os.ReadFile(outPath); err == nil {
					_ = os.Remove(outPath)
					return normalizeText(data), "PDF-Text"
				}
			}
		}
		if t := extractPrintableStrings(raw); len(t) > 80 {
			return t, "PDF-Rohtext"
		}
	}
	return "", ""
}

func isTextLike(mimeType, ext string, raw []byte) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	textExt := map[string]bool{
		".md": true, ".txt": true, ".log": true, ".json": true, ".jsonl": true, ".xml": true, ".html": true, ".htm": true,
		".css": true, ".scss": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".go": true, ".rs": true, ".py": true,
		".java": true, ".kt": true, ".kts": true, ".cs": true, ".cpp": true, ".c": true, ".h": true, ".hpp": true, ".sql": true,
		".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".properties": true, ".gradle": true, ".sh": true, ".bat": true,
		".ps1": true, ".csv": true, ".tsv": true, ".svg": true, ".env": true, ".gitignore": true,
	}
	if textExt[ext] {
		return true
	}
	if !utf8.Valid(raw) {
		return false
	}
	sample := raw
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	controls := 0
	for _, c := range sample {
		if c == 0 {
			return false
		}
		if c < 9 || (c > 13 && c < 32) {
			controls++
		}
	}
	return controls < len(sample)/100
}

func normalizeText(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func listZip(raw []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		suffix := ""
		if !f.FileInfo().IsDir() {
			suffix = fmt.Sprintf(" (%d Bytes)", f.UncompressedSize64)
		}
		names = append(names, f.Name+suffix)
		if len(names) >= 1000 {
			names = append(names, "...[Archivliste gekürzt]")
			break
		}
	}
	sort.Strings(names)
	return strings.Join(names, "\n"), nil
}

func extractOfficeXML(raw []byte, prefixes []string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, f := range zr.File {
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(f.Name, p) && strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		dec := xml.NewDecoder(io.LimitReader(rc, 16<<20))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			if ch, ok := tok.(xml.CharData); ok {
				t := strings.TrimSpace(string(ch))
				if t != "" {
					out.WriteString(t)
					out.WriteByte('\n')
				}
			}
			if out.Len() > maxExtractedPerFile {
				break
			}
		}
		_ = rc.Close()
		if out.Len() > maxExtractedPerFile {
			break
		}
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("kein extrahierbarer Office-Text")
	}
	return strings.TrimSpace(out.String()), nil
}

func extractPrintableStrings(raw []byte) string {
	var out strings.Builder
	var current strings.Builder
	flush := func() {
		if current.Len() >= 6 {
			out.WriteString(current.String())
			out.WriteByte('\n')
		}
		current.Reset()
	}
	for _, c := range raw {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 32 && c <= 126) {
			current.WriteByte(c)
		} else {
			flush()
		}
		if out.Len() > maxExtractedPerFile {
			break
		}
	}
	flush()
	return strings.TrimSpace(out.String())
}

func attachmentSummaries(items []Attachment) []AttachmentSummary {
	out := make([]AttachmentSummary, 0, len(items))
	for _, item := range items {
		out = append(out, AttachmentSummary{Name: item.Name, MIME: item.MIME, Size: item.Size})
	}
	return out
}

// Backward-compatible image helpers retained for older tests and configs.
type ImageAttachment = Attachment

func validateImages(images []ImageAttachment) ([]ImageAttachment, error) {
	items, err := validateAttachments(images)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if !isImageAttachment(item) {
			return nil, fmt.Errorf("nicht unterstütztes Bildformat %q bei %s", item.MIME, item.Name)
		}
	}
	if len(items) > 6 {
		return nil, fmt.Errorf("maximal 6 Bilder pro Anfrage")
	}
	return items, nil
}

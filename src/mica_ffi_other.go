// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

// MicaFFIThreatResult holds the live in-process verdict from the compiled Rust MICA kernel.
type MicaFFIThreatResult struct {
	Available   bool   `json:"available"`
	IsClean     bool   `json:"is_clean"`
	ThreatCode  int32  `json:"threat_code"`
	Version     string `json:"version,omitempty"`
	LibraryPath string `json:"library_path,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ClassifyThreatWithMICA reports unavailable on non-Windows CGO-free builds.
func ClassifyThreatWithMICA(text string) MicaFFIThreatResult {
	_ = text
	return MicaFFIThreatResult{
		Available:  false,
		IsClean:    true,
		ThreatCode: -1,
		Error:      "in-process LazyDLL MICA FFI is Windows-only in CGO-free builds",
	}
}

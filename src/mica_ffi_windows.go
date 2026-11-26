// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

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

type micaFFIRuntime struct {
	mu          sync.Mutex
	initialized bool
	available   bool
	libPath     string
	sha256Hex   string
	version     string
	initErr     string
	dll         *syscall.LazyDLL
	procVersion *syscall.LazyProc
	procCtxNew  *syscall.LazyProc
	procCtxFree *syscall.LazyProc
	procThreat  *syscall.LazyProc
	ctxPtr      uintptr
}

var globalMicaFFI micaFFIRuntime

func findMicaFFILibraryWindows() string {
	if env := strings.TrimSpace(os.Getenv("MICA_LIB_PATH")); env != "" {
		if info, err := os.Stat(env); err == nil && !info.IsDir() {
			return env
		}
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "mica_ffi.dll"),
			filepath.Join(exeDir, "..", "NomaMica", "MICA", "target", "release", "mica_ffi.dll"),
			filepath.Join(exeDir, "..", "NomaMica", "NOMA", "mica_ffi.dll"),
			filepath.Join(exeDir, "..", "..", "NomaMica", "MICA", "target", "release", "mica_ffi.dll"),
			filepath.Join(exeDir, "..", "..", "NomaMica", "NOMA", "mica_ffi.dll"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "mica_ffi.dll"),
			filepath.Join(cwd, "..", "NomaMica", "MICA", "target", "release", "mica_ffi.dll"),
			filepath.Join(cwd, "..", "NomaMica", "NOMA", "mica_ffi.dll"),
			filepath.Join(cwd, "..", "..", "NomaMica", "MICA", "target", "release", "mica_ffi.dll"),
			filepath.Join(cwd, "..", "..", "NomaMica", "NOMA", "mica_ffi.dll"),
		)
	}
	for _, c := range candidates {
		cleaned := filepath.Clean(c)
		if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
			return cleaned
		}
	}
	return ""
}

var (
	micaKernel32DLL    = syscall.NewLazyDLL("kernel32.dll")
	micaProcLstrlenA   = micaKernel32DLL.NewProc("lstrlenA")
	micaProcRtlMoveMem = micaKernel32DLL.NewProc("RtlMoveMemory")
)

func cStringPtrToGoString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	n, _, _ := micaProcLstrlenA.Call(ptr)
	if n == 0 {
		return ""
	}
	if n > 4096 {
		n = 4096
	}
	buf := make([]byte, n)
	_, _, _ = micaProcRtlMoveMem.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, n)
	return string(buf)
}

func (m *micaFFIRuntime) ensureInitLocked() {
	if m.initialized {
		return
	}
	m.initialized = true
	path := findMicaFFILibraryWindows()
	if path == "" {
		m.initErr = "mica_ffi.dll not found"
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		m.initErr = err.Error()
		return
	}
	sum := sha256.Sum256(raw)
	m.sha256Hex = hex.EncodeToString(sum[:])
	m.libPath = path

	dll := syscall.NewLazyDLL(path)
	if loadErr := dll.Load(); loadErr != nil {
		m.initErr = loadErr.Error()
		return
	}
	procVersion := dll.NewProc("mica_version")
	procCtxNew := dll.NewProc("mica_context_new")
	procCtxFree := dll.NewProc("mica_context_free")
	procThreat := dll.NewProc("mica_classify_threat")
	if err := procVersion.Find(); err != nil {
		m.initErr = err.Error()
		return
	}
	if err := procCtxNew.Find(); err != nil {
		m.initErr = err.Error()
		return
	}
	if err := procThreat.Find(); err != nil {
		m.initErr = err.Error()
		return
	}
	ctxPtr, _, _ := procCtxNew.Call()
	if ctxPtr == 0 {
		m.initErr = "mica_context_new returned NULL"
		return
	}
	verPtr, _, _ := procVersion.Call()
	m.version = cStringPtrToGoString(verPtr)
	m.dll = dll
	m.procVersion = procVersion
	m.procCtxNew = procCtxNew
	m.procCtxFree = procCtxFree
	m.procThreat = procThreat
	m.ctxPtr = ctxPtr
	m.available = true
}

// ClassifyThreatWithMICA calls the compiled Rust MICA kernel (`mica_classify_threat`) in-process without CGO.
func ClassifyThreatWithMICA(text string) MicaFFIThreatResult {
	globalMicaFFI.mu.Lock()
	defer globalMicaFFI.mu.Unlock()
	globalMicaFFI.ensureInitLocked()

	res := MicaFFIThreatResult{
		Available:   globalMicaFFI.available,
		IsClean:     true,
		ThreatCode:  -1,
		Version:     globalMicaFFI.version,
		LibraryPath: globalMicaFFI.libPath,
		SHA256:      globalMicaFFI.sha256Hex,
		Error:       globalMicaFFI.initErr,
	}
	if !globalMicaFFI.available || globalMicaFFI.ctxPtr == 0 {
		return res
	}
	// NUL-terminated UTF-8 byte buffer (strip any embedded NUL bytes)
	clean := strings.ReplaceAll(text, "\x00", " ")
	cBuf := append([]byte(clean), 0)
	var threatCode int32 = -1
	rc, _, _ := globalMicaFFI.procThreat.Call(
		globalMicaFFI.ctxPtr,
		uintptr(unsafe.Pointer(&cBuf[0])),
		uintptr(unsafe.Pointer(&threatCode)),
	)
	if int32(rc) != 0 {
		res.Error = "mica_classify_threat returned non-zero status"
		res.ThreatCode = int32(rc)
		return res
	}
	res.ThreatCode = threatCode
	res.IsClean = threatCode == 0
	return res
}

// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	modShell32Sys        = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW = modShell32Sys.NewProc("Shell_NotifyIconW")
	procExtractIconExW   = modShell32Sys.NewProc("ExtractIconExW")

	modUser32Sys            = syscall.NewLazyDLL("user32.dll")
	procRegisterClassExW    = modUser32Sys.NewProc("RegisterClassExW")
	procCreateWindowExW     = modUser32Sys.NewProc("CreateWindowExW")
	procDefWindowProcW      = modUser32Sys.NewProc("DefWindowProcW")
	procDestroyWindow       = modUser32Sys.NewProc("DestroyWindow")
	procPostQuitMessage     = modUser32Sys.NewProc("PostQuitMessage")
	procGetMessageW         = modUser32Sys.NewProc("GetMessageW")
	procTranslateMessage    = modUser32Sys.NewProc("TranslateMessage")
	procDispatchMessageW    = modUser32Sys.NewProc("DispatchMessageW")
	procPostMessageW        = modUser32Sys.NewProc("PostMessageW")
	procCreatePopupMenu     = modUser32Sys.NewProc("CreatePopupMenu")
	procAppendMenuW         = modUser32Sys.NewProc("AppendMenuW")
	procTrackPopupMenu      = modUser32Sys.NewProc("TrackPopupMenu")
	procDestroyMenu         = modUser32Sys.NewProc("DestroyMenu")
	procGetCursorPos        = modUser32Sys.NewProc("GetCursorPos")
	procSetForegroundWindow = modUser32Sys.NewProc("SetForegroundWindow")
	procLoadIconW           = modUser32Sys.NewProc("LoadIconW")
	procDestroyIcon         = modUser32Sys.NewProc("DestroyIcon")
	procSetMenuDefaultItem  = modUser32Sys.NewProc("SetMenuDefaultItem")
	procLoadImageW          = modUser32Sys.NewProc("LoadImageW")

	modKernel32Sys       = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW = modKernel32Sys.NewProc("GetModuleHandleW")
)

const (
	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIM_SETVERSION = 0x00000004

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004
	NIF_STATE   = 0x00000008
	NIF_INFO    = 0x00000010
	NIF_GUID    = 0x00000020

	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_QUIT          = 0x0012
	WM_COMMAND       = 0x0111
	WM_APP           = 0x8000
	WM_TRAY_CALLBACK = WM_APP + 1

	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_CONTEXTMENU   = 0x007B

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x0010
	LR_DEFAULTSIZE  = 0x0040

	IDI_APPLICATION = 32512
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVer    uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

var (
	activeTrayMu sync.RWMutex
	activeTray   *TrayManager
)

type TrayManager struct {
	url            string
	language       string
	startMinimized bool
	hwnd           syscall.Handle
	nid            NOTIFYICONDATAW
	hIcon          syscall.Handle
	running        atomic.Bool
	stopOnce       sync.Once
	readyCh        chan struct{}
}

func NewTrayManager(url, language string, startMinimized bool) *TrayManager {
	return &TrayManager{
		url:            url,
		language:       language,
		startMinimized: startMinimized,
		readyCh:        make(chan struct{}),
	}
}

func trayWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAY_CALLBACK:
		activeTrayMu.RLock()
		tm := activeTray
		activeTrayMu.RUnlock()
		if tm != nil {
			switch lParam {
			case WM_LBUTTONDBLCLK:
				tm.OpenUI()
				return 0
			case WM_RBUTTONUP, WM_CONTEXTMENU:
				tm.showContextMenu()
				return 0
			}
		}
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func loadAppIcon() syscall.Handle {
	exePath, err := os.Executable()
	if err == nil {
		var hIconSmall syscall.Handle
		pExe, _ := syscall.UTF16PtrFromString(exePath)
		r, _, _ := procExtractIconExW.Call(
			uintptr(unsafe.Pointer(pExe)),
			0,
			0,
			uintptr(unsafe.Pointer(&hIconSmall)),
			1,
		)
		if r > 0 && hIconSmall != 0 {
			return hIconSmall
		}
	}

	candidates := []string{}
	if err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "localcode.ico"))
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "..", "assets", "localcode.ico"))
	}
	candidates = append(candidates, filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "LocalCode", "localcode.ico"))
	candidates = append(candidates, "assets/localcode.ico", "localcode.ico")

	for _, cand := range candidates {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			pCand, _ := syscall.UTF16PtrFromString(cand)
			hIcon, _, _ := procLoadImageW.Call(
				0,
				uintptr(unsafe.Pointer(pCand)),
				IMAGE_ICON,
				16,
				16,
				LR_LOADFROMFILE,
			)
			if hIcon != 0 {
				return syscall.Handle(hIcon)
			}
		}
	}

	hIcon, _, _ := procLoadIconW.Call(0, uintptr(IDI_APPLICATION))
	return syscall.Handle(hIcon)
}

var (
	openBrowserMaximizedHook func(url string) error
	exitAppHook              func(code int)
)

func (tm *TrayManager) showContextMenu() {
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(tm.hwnd))

	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	openLabel := localizeConfigText(Config{Language: tm.language}, "Öffnen", "Open")
	exitLabel := localizeConfigText(Config{Language: tm.language}, "Beenden", "Exit")

	openPtr, _ := syscall.UTF16PtrFromString(openLabel)
	exitPtr, _ := syscall.UTF16PtrFromString(exitLabel)

	const (
		cmdOpen = 1001
		cmdExit = 1002
	)

	procAppendMenuW.Call(hMenu, MF_STRING, cmdOpen, uintptr(unsafe.Pointer(openPtr)))
	procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)
	procAppendMenuW.Call(hMenu, MF_STRING, cmdExit, uintptr(unsafe.Pointer(exitPtr)))

	procSetMenuDefaultItem.Call(hMenu, cmdOpen, 0)

	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		TPM_RIGHTBUTTON|TPM_RETURNCMD,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(tm.hwnd),
		0,
	)

	procPostMessageW.Call(uintptr(tm.hwnd), 0, 0, 0)

	switch cmd {
	case cmdOpen:
		tm.OpenUI()
	case cmdExit:
		tm.ExitApp()
	}
}

func (tm *TrayManager) OpenUI() {
	log.Printf("Tray: opening UI at %s (maximized)", tm.url)
	if openBrowserMaximizedHook != nil {
		if err := openBrowserMaximizedHook(tm.url); err != nil {
			log.Printf("Tray: failed opening browser: %v", err)
		}
		return
	}
	if err := openBrowserMaximized(tm.url); err != nil {
		log.Printf("Tray: failed opening browser: %v", err)
	}
}

func (tm *TrayManager) ExitApp() {
	log.Printf("Tray: exit requested by user")
	tm.Stop()
	if exitAppHook != nil {
		exitAppHook(0)
		return
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
}

func (tm *TrayManager) Stop() {
	tm.stopOnce.Do(func() {
		tm.running.Store(false)
		if tm.hwnd != 0 {
			procShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&tm.nid)))
			procPostMessageW.Call(uintptr(tm.hwnd), WM_CLOSE, 0, 0)
			procPostQuitMessage.Call(0)
		}
		if tm.hIcon != 0 {
			procDestroyIcon.Call(uintptr(tm.hIcon))
			tm.hIcon = 0
		}
	})
}

func openBrowserMaximized(url string) error {
	if err := openChromiumApp(url, false); err == nil {
		return nil
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	hideCommandWindow(cmd)
	return cmd.Start()
}

func (tm *TrayManager) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("LocalCodeTrayWindowClass")
	windowTitle, _ := syscall.UTF16PtrFromString("LocalCode System Tray")

	wndProcCallback := syscall.NewCallback(trayWndProc)

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = wndProcCallback
	wc.HInstance = syscall.Handle(hInstance)
	wc.LpszClassName = className
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	const HWND_MESSAGE = ^uintptr(2) // (HWND)-3 for message-only window

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0,
		0, 0, 0, 0,
		HWND_MESSAGE,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("failed creating tray host window")
	}

	tm.hwnd = syscall.Handle(hwnd)
	tm.hIcon = loadAppIcon()

	activeTrayMu.Lock()
	activeTray = tm
	activeTrayMu.Unlock()

	tm.nid = NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(tm.nid)),
		HWnd:             tm.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAY_CALLBACK,
		HIcon:            tm.hIcon,
	}

	tip := "LocalCode"
	copy(tm.nid.SzTip[:], syscall.StringToUTF16(tip))

	procShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&tm.nid)))

	tm.running.Store(true)
	close(tm.readyCh)

	var msg MSG
	for tm.running.Load() {
		r, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	tm.Stop()
	return nil
}

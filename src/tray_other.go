// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"log"
	"os"
)

var (
	openBrowserMaximizedHook func(url string) error
	exitAppHook              func(code int)
)

type TrayManager struct {
	url            string
	language       string
	startMinimized bool
}

func NewTrayManager(url, language string, startMinimized bool) *TrayManager {
	return &TrayManager{
		url:            url,
		language:       language,
		startMinimized: startMinimized,
	}
}

func (tm *TrayManager) Run() error {
	log.Printf("System tray is supported on Windows. Running in standard background mode.")
	select {}
}

func (tm *TrayManager) OpenUI() {
	if openBrowserMaximizedHook != nil {
		_ = openBrowserMaximizedHook(tm.url)
		return
	}
	_ = openBrowser(tm.url)
}

func (tm *TrayManager) ExitApp() {
	tm.Stop()
	if exitAppHook != nil {
		exitAppHook(0)
		return
	}
	os.Exit(0)
}

func (tm *TrayManager) Stop() {}

func openBrowserMaximized(url string) error {
	return openBrowser(url)
}

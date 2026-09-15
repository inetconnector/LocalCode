// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"log"
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
	_ = openBrowser(tm.url)
}

func (tm *TrayManager) ExitApp() {}

func (tm *TrayManager) Stop() {}

func openBrowserMaximized(url string) error {
	return openBrowser(url)
}

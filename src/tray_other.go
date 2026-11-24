// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	openBrowserMaximizedHook func(url string) error
	exitAppHook              func(code int)
)

type TrayManager struct {
	url            string
	language       string
	startMinimized bool
	stopCh         chan struct{}
	stopOnce       sync.Once
}

func NewTrayManager(url, language string, startMinimized bool) *TrayManager {
	return &TrayManager{
		url:            url,
		language:       language,
		startMinimized: startMinimized,
		stopCh:         make(chan struct{}),
	}
}

func (tm *TrayManager) Run() error {
	log.Printf("LocalCode headless background manager running for %s (SIGINT/SIGTERM aware).", tm.url)
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	select {
	case <-tm.stopCh:
		return nil
	case sig := <-sigCh:
		log.Printf("LocalCode background manager received signal %v, shutting down cleanly.", sig)
		return nil
	}
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

func (tm *TrayManager) Stop() {
	tm.stopOnce.Do(func() {
		if tm.stopCh != nil {
			close(tm.stopCh)
		}
	})
}

func openBrowserMaximized(url string) error {
	return openBrowser(url)
}

// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

import (
	"context"
	"errors"
	"strings"
)

type DesktopWindowInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	PID         int    `json:"pid"`
	Handle      int64  `json:"handle"`
	IsActive    bool   `json:"is_active,omitempty"`
}

func DesktopListWindows(ctx context.Context, cfg Config) (string, error) {
	return "", errors.New("desktop UI automation is only supported on Windows")
}

func DesktopInspect(ctx context.Context, cfg Config, windowTitle, selector string) (string, error) {
	return "", errors.New("desktop UI automation is only supported on Windows")
}

func DesktopClick(ctx context.Context, cfg Config, windowTitle, controlName string) (string, error) {
	return "", errors.New("desktop UI automation is only supported on Windows")
}

func DesktopType(ctx context.Context, cfg Config, windowTitle, controlName, text string) (string, error) {
	return "", errors.New("desktop UI automation is only supported on Windows")
}

func DesktopScreenshot(ctx context.Context, cfg Config, project, windowTitle, destination string) (string, error) {
	return "", errors.New("desktop UI automation is only supported on Windows")
}

var blockedDesktopWindows = []string{
	"task manager", "taskmgr", "windows security", "sicherheitscenter", "logonui", "uac",
	"benutzerkontensteuerung", "credential", "passwort", "anmeldeinformationen",
}

func isBlockedDesktopWindow(title, processName string) bool {
	lowerT := strings.ToLower(title)
	lowerP := strings.ToLower(processName)
	for _, blocked := range blockedDesktopWindows {
		if strings.Contains(lowerT, blocked) || strings.Contains(lowerP, blocked) {
			return true
		}
	}
	return false
}

func escapePowerShellString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

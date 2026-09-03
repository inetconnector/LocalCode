// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

import (
	"context"
	"errors"
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

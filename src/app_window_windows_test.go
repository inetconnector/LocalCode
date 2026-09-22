// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import "testing"

func TestLocalCodeAppWindowScope(t *testing.T) {
	for _, tc := range []struct {
		title, class, image string
		want                bool
	}{
		{"LocalCode", "Chrome_WidgetWin_1", `C:\Browser\msedge.exe`, true},
		{"LocalCode", "Chrome_WidgetWin_1", `c:\browser\MSEDGE.EXE`, true},
		{"LocalCode - Microsoft Edge", "Chrome_WidgetWin_1", `C:\Browser\msedge.exe`, false},
		{"Other app", "Chrome_WidgetWin_1", `C:\Browser\msedge.exe`, false},
		{"LocalCode", "OtherClass", `C:\Browser\msedge.exe`, false},
		{"LocalCode", "Chrome_WidgetWin_1", `C:\Other\msedge.exe`, false},
	} {
		if got := isLocalCodeAppWindow(tc.title, tc.class, tc.image, `C:\Browser\msedge.exe`); got != tc.want {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

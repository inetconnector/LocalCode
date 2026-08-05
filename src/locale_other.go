// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"os"
	"strings"
)

func detectSystemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "en"
}

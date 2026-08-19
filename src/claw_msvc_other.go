//go:build !windows

// SPDX-License-Identifier: Apache-2.0

package main

import "context"

func ensureClawMSVCToolchain(ctx context.Context, cfg Config) (string, string, error) {
	return "", "Visual C++ Build Tools are not required on this platform.", nil
}

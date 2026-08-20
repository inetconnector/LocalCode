//go:build !windows

// SPDX-License-Identifier: Apache-2.0

package main

import "context"

func ensureClawMSVCToolchain(ctx context.Context, cfg Config) (string, string, error) {
	return "", "Visual C++ Build Tools are not required on this platform.", nil
}

func runClawCargoBuild(ctx context.Context, cargoPath, rustRoot string, cfg Config) (string, int, error) {
	return runCapturedCommand(ctx, cargoPath, []string{"build", "--workspace", "--release"}, commandEnvironment(cfg), rustRoot)
}

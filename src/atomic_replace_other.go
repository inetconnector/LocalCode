// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package main

import "os"

func replaceFileAtomic(stagedPath, targetPath string, targetExists bool) error {
	// POSIX rename replaces an existing non-directory target atomically when
	// source and destination are on the same filesystem. Staging is always done
	// in targetPath's directory, so that requirement is satisfied here.
	return os.Rename(stagedPath, targetPath)
}

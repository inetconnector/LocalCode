// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
)

// projectHasGitMetadata reports whether project is inside a normal Git work
// tree without spawning git. It deliberately accepts both a .git directory
// and the .git file used by worktrees/submodules. The upward walk preserves
// projects that point at a repository subdirectory.
func projectHasGitMetadata(project string) bool {
	dir, err := filepath.Abs(project)
	if err != nil || dir == "" {
		return false
	}
	for {
		info, statErr := os.Lstat(filepath.Join(dir, ".git"))
		if statErr == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

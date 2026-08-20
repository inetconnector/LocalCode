// SPDX-License-Identifier: Apache-2.0

package main

import "os"

// writeFileAtomic is intentionally limited to LocalCode-managed installation
// targets. It reuses the same per-path lock, optimistic version token and
// same-directory atomic replacement as native project edits, so an external
// change cannot be silently overwritten between staging and commit.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	unlock := acquireEditPathLock(path)
	defer unlock()
	expected, err := readFileVersion(path)
	if err != nil {
		return err
	}
	return atomicWriteFileIfVersion(path, data, mode, expected)
}

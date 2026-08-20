// SPDX-License-Identifier: Apache-2.0

package main

// Claw is built from source, so Cargo/Rust are first-class managed build
// dependencies. Keep the generic registry data stable and enable the official
// Rustup WinGet package here as part of the optional Claw integration.
func init() {
	for index := range toolProfiles {
		switch toolProfiles[index].Name {
		case "cargo", "rustc":
			toolProfiles[index].InstallKind = "winget"
			toolProfiles[index].WingetID = "Rustlang.Rustup"
		}
	}
}

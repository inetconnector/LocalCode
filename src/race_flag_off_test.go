// SPDX-License-Identifier: Apache-2.0
//go:build !race

package main

import "runtime"

// The shared agent-stop helper already polls every 20 ms and exits immediately
// when the agent stops. Windows runners can experience multi-second scheduler
// stalls even without -race, so reuse the extended polling budget there as
// well. Non-Windows non-race tests keep the shorter budget.
var raceDetectorEnabled = runtime.GOOS == "windows"

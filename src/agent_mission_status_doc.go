// SPDX-License-Identifier: Apache-2.0

package main

// AgentMissionDesktopStatus is intentionally ephemeral observation data for the
// Desktop UI. It is not a Mission start/control API, does not grant capabilities
// and is not a recovery record. Durable Mission recovery must continue to extend
// run_journal.go rather than this status registry.

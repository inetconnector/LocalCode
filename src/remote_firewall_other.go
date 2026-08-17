// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

func ensureRemoteFirewallRule(port int) error {
	return nil
}

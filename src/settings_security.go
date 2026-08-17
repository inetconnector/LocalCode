// SPDX-License-Identifier: Apache-2.0

package main

// publicSettingsConfig returns the configuration shape exposed to the desktop
// settings UI. Paired-device credentials are server-owned security state and
// are managed through the dedicated remote-device API, not the generic
// settings document.
func publicSettingsConfig(cfg Config) Config {
	clone, err := cloneConfig(cfg)
	if err != nil {
		// Config cloning should not fail for the persisted schema. Fail closed for
		// credential exposure if it ever does.
		cfg.RemoteDevices = nil
		return cfg
	}
	clone.RemoteDevices = nil
	return clone
}

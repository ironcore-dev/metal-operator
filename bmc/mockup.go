// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishMockUps struct {
	BIOSSettingAttr    map[string]map[string]any
	PendingBIOSSetting map[string]map[string]any
}

func (r *RedfishMockUps) InitializeDefaults() {
	r.BIOSSettingAttr = map[string]map[string]any{
		"abc":       {"type": "string", "reboot": false, "value": "bar"},
		"fooreboot": {"type": "integer", "reboot": true, "value": 123},
	}
	r.PendingBIOSSetting = map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBIOSSettings() {
	r.BIOSSettingAttr = map[string]map[string]any{
		"abc":       {"type": "string", "reboot": false, "value": "bar"},
		"fooreboot": {"type": "integer", "reboot": true, "value": 123},
	}
	r.PendingBIOSSetting = map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetPendingBIOSSetting() {
	r.PendingBIOSSetting = map[string]map[string]any{}
}

func InitMockUp() {
	UnitTestMockUps = &RedfishMockUps{}
	UnitTestMockUps.InitializeDefaults()
}

var UnitTestMockUps *RedfishMockUps

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import "github.com/stmcginnis/gofish/redfish"

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishMockUps struct {
	BIOSSettingAttr     map[string]map[string]any
	PendingBIOSSetting  map[string]map[string]any
	ComputeSystemMock   map[string]*redfish.ComputerSystem
	SystemProcessorMock map[string][]*redfish.Processor
	SystemStorageMock   map[string][]*redfish.SimpleStorage
}

func (r *RedfishMockUps) InitializeDefaults() {
	r.BIOSSettingAttr = map[string]map[string]any{
		"abc":       {"type": "string", "reboot": false, "value": "bar"},
		"fooreboot": {"type": "integer", "reboot": true, "value": 123},
	}
	r.PendingBIOSSetting = map[string]map[string]any{}
	r.ComputeSystemMock = map[string]*redfish.ComputerSystem{}
	r.SystemProcessorMock = map[string][]*redfish.Processor{}
	r.SystemStorageMock = map[string][]*redfish.SimpleStorage{}
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

func CreateMockUp() {
	UnitTestMockUps = &RedfishMockUps{}
	UnitTestMockUps.InitializeDefaults()
}

var UnitTestMockUps *RedfishMockUps

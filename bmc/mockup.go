// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"sync"

	"github.com/stmcginnis/gofish/redfish"
)

// RedfishMockUps is an implementation of the BMC interface for Redfish.
type RedfishMockUps struct {
	mu sync.RWMutex

	BIOSSettingAttr       map[string]map[string]map[string]any
	PendingBIOSSetting    map[string]map[string]map[string]any
	BIOSVersion           map[string]string
	BIOSUpgradingVersion  map[string]string
	BIOSUpgradeTaskIndex  map[string]int
	BIOSUpgradeTaskStatus []redfish.Task

	BMCSettingAttr    map[string]map[string]map[string]any
	PendingBMCSetting map[string]map[string]map[string]any

	BMCVersion           map[string]string
	BMCUpgradingVersion  map[string]string
	BMCUpgradeTaskIndex  map[string]int
	BMCUpgradeTaskStatus []redfish.Task

	SimulateUnvailableBMC map[string]bool
}

func (r *RedfishMockUps) InitializeDefaults() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.BIOSSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": "string", "reboot": false, "value": "bar"},
			"fooreboot": {"type": "integer", "reboot": true, "value": 123},
		},
	}
	r.BMCSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": redfish.StringAttributeType, "reboot": false, "value": "bar"},
			"fooreboot": {"type": redfish.IntegerAttributeType, "reboot": true, "value": 123},
		},
	}
	r.PendingBIOSSetting = map[string]map[string]map[string]any{}
	r.BIOSVersion = map[string]string{
		"default": "",
	}
	r.BIOSUpgradingVersion = map[string]string{
		"default": "",
	}

	r.BIOSUpgradeTaskIndex = map[string]int{
		"default": 0,
	}
	r.BIOSUpgradeTaskStatus = []redfish.Task{
		{
			TaskState:       redfish.NewTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.PendingTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.StartingTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 10,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 20,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 100,
		},
		{
			TaskState:       redfish.CompletedTaskState,
			PercentComplete: 100,
		},
	}

	r.PendingBMCSetting = map[string]map[string]map[string]any{}

	r.BMCVersion = map[string]string{
		"default": "",
	}
	r.BMCUpgradingVersion = map[string]string{
		"default": "",
	}

	r.BMCUpgradeTaskIndex = map[string]int{
		"default": 0,
	}
	r.BMCUpgradeTaskStatus = []redfish.Task{
		{
			TaskState:       redfish.NewTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.PendingTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.StartingTaskState,
			PercentComplete: 0,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 10,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 20,
		},
		{
			TaskState:       redfish.RunningTaskState,
			PercentComplete: 100,
		},
		{
			TaskState:       redfish.CompletedTaskState,
			PercentComplete: 100,
		},
	}

	r.SimulateUnvailableBMC = map[string]bool{
		"default": false,
	}
}

func (r *RedfishMockUps) ResetBIOSSettings() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.BIOSSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": "string", "reboot": false, "value": "bar"},
			"fooreboot": {"type": "integer", "reboot": true, "value": 123},
		},
	}
	r.PendingBIOSSetting = map[string]map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetPendingBIOSSetting() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.PendingBIOSSetting = map[string]map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBIOSVersionUpdate() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// NOTE: We do not call r.ResetBIOSSettings() here to avoid locking twice (deadlock/panic).
	// Instead, we copy the logic directly, or ensure ResetBIOSSettings() does not lock itself.

	r.BIOSSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": "string", "reboot": false, "value": "bar"},
			"fooreboot": {"type": "integer", "reboot": true, "value": 123},
		},
	}
	r.PendingBIOSSetting = map[string]map[string]map[string]any{}

	r.BIOSUpgradeTaskIndex = map[string]int{
		"default": 0,
	}
	r.BIOSUpgradingVersion = map[string]string{
		"default": "",
	}
	r.BIOSVersion = map[string]string{
		"default": "",
	}
}

func (r *RedfishMockUps) ResetPendingBMCSetting() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.PendingBMCSetting = map[string]map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBMCSettings() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.BMCSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": redfish.StringAttributeType, "reboot": false, "value": "bar"},
			"fooreboot": {"type": redfish.IntegerAttributeType, "reboot": true, "value": 123},
		},
	}
	r.PendingBMCSetting = map[string]map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBMCVersionUpdate() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// NOTE: Copying logic from ResetBMCSettings to avoid double-locking.
	r.BMCSettingAttr = map[string]map[string]map[string]any{
		"default": {
			"abc":       {"type": redfish.StringAttributeType, "reboot": false, "value": "bar"},
			"fooreboot": {"type": redfish.IntegerAttributeType, "reboot": true, "value": 123},
		},
	}
	r.PendingBMCSetting = map[string]map[string]map[string]any{}

	r.BMCVersion = map[string]string{
		"default": "",
	}
	r.BMCUpgradingVersion = map[string]string{
		"default": "",
	}
	r.BMCUpgradeTaskIndex = map[string]int{
		"default": 0,
	}
}

func InitMockUp() {
	UnitTestMockUps = &RedfishMockUps{}
	UnitTestMockUps.InitializeDefaults()
}

var UnitTestMockUps *RedfishMockUps

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// RedfishMockUps is an implementation of the BMC interface for Redfish.
type RedfishMockUps struct {
	BIOSSettingAttr       map[string]map[string]any
	PendingBIOSSetting    map[string]map[string]any
	BIOSVersion           string
	BIOSUpgradingVersion  string
	BIOSUpgradeTaskIndex  int
	BIOSUpgradeTaskStatus []schemas.Task

	BMCSettingAttr    map[string]map[string]any
	PendingBMCSetting map[string]map[string]any

	BMCVersion           string
	BMCUpgradingVersion  string
	BMCUpgradeTaskIndex  int
	BMCUpgradeTaskStatus []schemas.Task

	Accounts              map[string]*schemas.ManagerAccount
	SimulateUnvailableBMC bool

	// MockDelays controls timing for simulated async operations.
	MockDelays MockDelays
}

// MockDelays holds configurable delays for mock BMC operations.
type MockDelays struct {
	UpgradeTaskInit     time.Duration
	UpgradeTaskStep     time.Duration
	ResetSettingsApply  time.Duration
	PowerStateChange    time.Duration
	PendingSettingApply time.Duration
}

func (r *RedfishMockUps) InitializeDefaults() {
	r.BIOSSettingAttr = map[string]map[string]any{
		"abc":       {"type": "string", "reboot": false, "value": "bar"},
		"fooreboot": {"type": "integer", "reboot": true, "value": 123},
	}
	r.BMCSettingAttr = map[string]map[string]any{
		"abc":       {"type": schemas.StringAttributeType, "reboot": false, "value": "bar"},
		"fooreboot": {"type": schemas.IntegerAttributeType, "reboot": true, "value": 123},
	}
	r.PendingBIOSSetting = map[string]map[string]any{}
	r.BIOSVersion = ""
	r.BIOSUpgradingVersion = ""

	r.BIOSUpgradeTaskIndex = 0
	r.BIOSUpgradeTaskStatus = []schemas.Task{
		{
			TaskState:       schemas.NewTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.PendingTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.StartingTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(10)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(20)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(100)),
		},
		{
			TaskState:       schemas.CompletedTaskState,
			PercentComplete: gofish.ToRef(uint(100)),
		},
	}

	r.PendingBMCSetting = map[string]map[string]any{}

	r.BMCVersion = ""
	r.BMCUpgradingVersion = ""

	r.BMCUpgradeTaskIndex = 0
	r.BMCUpgradeTaskStatus = []schemas.Task{
		{
			TaskState:       schemas.NewTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.PendingTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.StartingTaskState,
			PercentComplete: gofish.ToRef(uint(0)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(10)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(20)),
		},
		{
			TaskState:       schemas.RunningTaskState,
			PercentComplete: gofish.ToRef(uint(100)),
		},
		{
			TaskState:       schemas.CompletedTaskState,
			PercentComplete: gofish.ToRef(uint(100)),
		},
	}

	r.Accounts = map[string]*schemas.ManagerAccount{
		"foo": {
			Entity: schemas.Entity{
				ID: "0",
			},
			UserName: "foo",
			Enabled:  true,
			RoleID:   "ReadOnly",
			Locked:   false,
			Password: "bar",
		},
		"admin": {
			Entity: schemas.Entity{
				ID: "1",
			},

			UserName: "admin",
			Enabled:  true,
			RoleID:   "Administrator",
			Locked:   false,
			Password: "adminpass",
		},
		"user": {
			Entity: schemas.Entity{
				ID: "2",
			},
			UserName: "user",
			Enabled:  true,
			RoleID:   "ReadOnly",
			Locked:   false,
			Password: "userpass",
		},
	}
	r.SimulateUnvailableBMC = false
	r.MockDelays = MockDelays{
		UpgradeTaskInit:     20 * time.Millisecond,
		UpgradeTaskStep:     5 * time.Millisecond,
		ResetSettingsApply:  150 * time.Millisecond,
		PowerStateChange:    150 * time.Millisecond,
		PendingSettingApply: 50 * time.Millisecond,
	}
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

func (r *RedfishMockUps) ResetBIOSVersionUpdate() {
	r.ResetBIOSSettings()
	r.BIOSUpgradeTaskIndex = 0
	r.BIOSUpgradingVersion = ""
	r.BIOSVersion = ""
}

func (r *RedfishMockUps) ResetPendingBMCSetting() {
	r.PendingBMCSetting = map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBMCSettings() {
	r.BMCSettingAttr = map[string]map[string]any{
		"abc":       {"type": schemas.StringAttributeType, "reboot": false, "value": "bar"},
		"fooreboot": {"type": schemas.IntegerAttributeType, "reboot": true, "value": 123},
	}
	r.PendingBMCSetting = map[string]map[string]any{}
}

func (r *RedfishMockUps) ResetBMCVersionUpdate() {
	r.ResetBMCSettings()
	r.BMCVersion = ""
	r.BMCUpgradingVersion = ""
	r.BMCUpgradeTaskIndex = 0
}

func InitMockUp() {
	UnitTestMockUps = &RedfishMockUps{}
	UnitTestMockUps.InitializeDefaults()
}

var UnitTestMockUps *RedfishMockUps

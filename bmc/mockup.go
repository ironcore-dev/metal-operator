// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// RedfishMockUps is an implementation of the BMC interface for Redfish.
type RedfishMockUps struct {
	BIOSSettingAttr             map[string]map[string]any
	PendingBIOSSetting          map[string]map[string]any
	BIOSVersion                 string
	BIOSUpgradingVersion        string
	BIOSUpgradeTaskIndex        int
	BIOSUpgradeTaskStatus       []redfish.Task
	BIOSUpgradeTaskFailedStatus []redfish.Task

	BMCSettingAttr    map[string]map[string]any
	PendingBMCSetting map[string]map[string]any

	BMCVersion                 string
	BMCUpgradingVersion        string
	BMCUpgradeTaskIndex        int
	BMCUpgradeTaskStatus       []redfish.Task
	BMCUpgradeTaskFailedStatus []redfish.Task

	Accounts              map[string]*redfish.ManagerAccount
	SimulateUnvailableBMC bool
}

func (r *RedfishMockUps) InitializeDefaults() {
	r.BIOSSettingAttr = map[string]map[string]any{
		"abc":       {"type": "string", "reboot": false, "value": "bar"},
		"fooreboot": {"type": "integer", "reboot": true, "value": 123},
	}
	r.BMCSettingAttr = map[string]map[string]any{
		"abc":       {"type": redfish.StringAttributeType, "reboot": false, "value": "bar"},
		"fooreboot": {"type": redfish.IntegerAttributeType, "reboot": true, "value": 123},
	}
	r.PendingBIOSSetting = map[string]map[string]any{}
	r.BIOSVersion = ""
	r.BIOSUpgradingVersion = ""

	r.BIOSUpgradeTaskIndex = 0
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
	r.BIOSUpgradeTaskFailedStatus = []redfish.Task{
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
			PercentComplete: 98,
		},
		{
			TaskState:       redfish.ExceptionTaskState,
			PercentComplete: 99,
		},
	}

	r.PendingBMCSetting = map[string]map[string]any{}

	r.BMCVersion = ""
	r.BMCUpgradingVersion = ""

	r.BMCUpgradeTaskIndex = 0
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

	r.BMCUpgradeTaskFailedStatus = []redfish.Task{
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
			PercentComplete: 98,
		},
		{
			TaskState:       redfish.ExceptionTaskState,
			PercentComplete: 99,
		},
	}

	r.Accounts = map[string]*redfish.ManagerAccount{
		"foo": {
			Entity: common.Entity{
				ID: "0",
			},
			UserName: "foo",
			Enabled:  true,
			RoleID:   "ReadOnly",
			Locked:   false,
			Password: "bar",
		},
		"admin": {
			Entity: common.Entity{
				ID: "1",
			},

			UserName: "admin",
			Enabled:  true,
			RoleID:   "Administrator",
			Locked:   false,
			Password: "adminpass",
		},
		"user": {
			Entity: common.Entity{
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
		"abc":       {"type": redfish.StringAttributeType, "reboot": false, "value": "bar"},
		"fooreboot": {"type": redfish.IntegerAttributeType, "reboot": true, "value": 123},
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

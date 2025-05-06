// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

var DefaultMockedBIOSSetting = map[string]map[string]any{
	"abc":       {"type": "string", "reboot": false, "value": "bar"},
	"fooreboot": {"type": "integer", "reboot": true, "value": 123},
}

var PendingMockedBIOSSetting = map[string]map[string]any{}

var MockedBIOSVersion = "123.5"
var MockedBIOSUpgradingVersion = "123.5"

var MockedBIOSUpgradeTaskIndex = 0
var MockedBIOSUpgradeTaskStatus = []redfish.Task{
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

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
	StoredBIOSSettingData map[string]map[string]any
	StoredBMCSettingData  map[string]map[string]any
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(
	ctx context.Context,
	options BMCOptions,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r RedfishLocalBMC) PowerOn(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}
	system.PowerState = redfish.OnPowerState
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OnPowerState, systemUUID, err)
	}

	// mock the bmc update here
	if len(PendingMockedBIOSSetting) > 0 {

		for key, data := range PendingMockedBIOSSetting {
			if _, ok := DefaultMockedBIOSSetting[key]; ok {
				DefaultMockedBIOSSetting[key] = data
			}
		}
		PendingMockedBIOSSetting = map[string]map[string]any{}
		r.StoredBIOSSettingData = DefaultMockedBIOSSetting
	}
	return nil
}

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}
	system.PowerState = redfish.OffPowerState
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OffPowerState, systemUUID, err)
	}
	return nil
}

func (r *RedfishLocalBMC) GetBiosPendingAttributeValues(
	ctx context.Context,
	systemUUID string,
) (
	redfish.SettingsAttributes,
	error,
) {
	if len(PendingMockedBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(PendingMockedBIOSSetting))

	for key, data := range PendingMockedBIOSSetting {
		result[key] = data["value"]
	}

	return result, nil
}

// mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBiosAttributesOnReset(
	ctx context.Context,
	systemUUID string,
	attributes redfish.SettingsAttributes,
) error {
	if len(DefaultMockedBIOSSetting) == 0 {
		DefaultMockedBIOSSetting = map[string]map[string]any{}
	}

	PendingMockedBIOSSetting = map[string]map[string]any{}
	for key, attrData := range attributes {
		if AttributesData, ok := DefaultMockedBIOSSetting[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				PendingMockedBIOSSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	r.StoredBIOSSettingData = DefaultMockedBIOSSetting

	return nil
}

func (r *RedfishLocalBMC) getMockedBIOSSettingData() map[string]map[string]any {

	if len(r.StoredBIOSSettingData) > 0 {
		return r.StoredBIOSSettingData
	}
	return DefaultMockedBIOSSetting

}

func (r *RedfishLocalBMC) GetBiosAttributeValues(
	ctx context.Context,
	systemUUID string,
	attributes []string,
) (
	redfish.SettingsAttributes,
	error,
) {

	if len(attributes) == 0 {
		return nil, nil
	}

	mockedAttributes := r.getMockedBIOSSettingData()

	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := mockedAttributes[name]; ok {
				result[name] = AttributesData["value"]
			}
		}
	}
	return result, nil
}

func (r *RedfishLocalBMC) getFilteredBiosRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	map[string]RegistryEntryAttributes,
	error,
) {
	mockedAttributes := r.getMockedBIOSSettingData()
	filtered := make(map[string]RegistryEntryAttributes)
	if len(mockedAttributes) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for name, AttributesData := range mockedAttributes {
		data := RegistryEntryAttributes{}
		data.AttributeName = name
		data.Immutable = immutable
		data.ReadOnly = readOnly
		data.Type = AttributesData["type"].(string)
		data.ResetRequired = AttributesData["reboot"].(bool)
		filtered[name] = data
	}
	return filtered, nil
}

// check if the attributes need to reboot when changed, and are correct type.
// supported attrType, bmc and bios
func (r *RedfishLocalBMC) CheckBiosAttributes(attrs redfish.SettingsAttributes) (bool, error) {
	reset := false
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)

	if err != nil {
		return reset, err
	}

	if len(filtered) == 0 {
		return reset, err
	}
	return r.checkAttribues(attrs, filtered)
}

func (r *RedfishLocalBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	return MockedBIOSVersion, nil
}

func (r *RedfishLocalBMC) UpgradeBiosVersion(
	ctx context.Context,
	manufacturer string,
	parameters *redfish.SimpleUpdateParameters,
) (string, bool, error) {
	MockedBIOSUpgradeTaskIndex = 0
	// note, ImageURI is mocked for testing upgrading to version
	MockedBIOSUpgradingVersion = parameters.ImageURI
	// this go routine mocks the upgrade progress
	go func() {
		time.Sleep(20 * time.Millisecond)
		for MockedBIOSUpgradeTaskIndex < len(MockedBIOSUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			MockedBIOSUpgradeTaskIndex = MockedBIOSUpgradeTaskIndex + 1
		}
	}()
	return "dummyTask", false, nil
}

func (r *RedfishLocalBMC) GetBiosUpgradeTask(
	ctx context.Context,
	manufacturer string,
	taskURI string,
) (*redfish.Task, error) {
	if MockedBIOSUpgradeTaskIndex > len(MockedBIOSUpgradeTaskStatus)-1 {
		MockedBIOSUpgradeTaskIndex = len(MockedBIOSUpgradeTaskStatus) - 1
	}
	if MockedBIOSUpgradeTaskStatus[MockedBIOSUpgradeTaskIndex].TaskState == redfish.CompletedTaskState {
		MockedBIOSVersion = MockedBIOSUpgradingVersion
	}
	return &MockedBIOSUpgradeTaskStatus[MockedBIOSUpgradeTaskIndex], nil
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	"github.com/stmcginnis/gofish/redfish"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ BMC = (*RedfishLocalBMC)(nil)

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(
	ctx context.Context,
	options Options,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r RedfishLocalBMC) PowerOn(ctx context.Context, systemURI string) error {

	go func() {
		time.Sleep(250 * time.Millisecond)
		system, err := r.getSystemFromUri(ctx, systemURI)
		if err != nil {
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Error(err, "failed to get system")
			return
		}
		system.PowerState = redfish.OnPowerState
		system.RawData = nil
		systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
		if err := system.Patch(systemURI, system); err != nil {
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Error(err, "failed to Patch system to power on", "systemUUID", systemURI)
			return
		}

		// mock the bmc update here
		if len(UnitTestMockUps.PendingBIOSSetting) > 0 {
			time.Sleep(150 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBIOSSetting {
				if _, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
					UnitTestMockUps.BIOSSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBIOSSetting()
		}
	}()
	return nil
}

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemURI string) error {

	go func() {
		time.Sleep(250 * time.Millisecond)
		system, err := r.getSystemFromUri(ctx, systemURI)
		if err != nil {
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Error(err, "failed to get system")
			return
		}
		system.PowerState = redfish.OffPowerState
		system.RawData = nil
		systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
		if err := system.Patch(systemURI, system); err != nil {
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Error(err, "failed to Patch system to power off", "systemURI", systemURI)
			return
		}
	}()
	return nil
}

func (r *RedfishLocalBMC) GetBiosPendingAttributeValues(
	ctx context.Context,
	systemUUID string,
) (
	redfish.SettingsAttributes,
	error,
) {
	if len(UnitTestMockUps.PendingBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBIOSSetting))

	for key, data := range UnitTestMockUps.PendingBIOSSetting {
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

	UnitTestMockUps.ResetPendingBIOSSetting()
	for key, attrData := range attributes {
		if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				UnitTestMockUps.PendingBIOSSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	return nil
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

	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[name]; ok {
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
	filtered := make(map[string]RegistryEntryAttributes)
	if len(UnitTestMockUps.BIOSSettingAttr) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for name, AttributesData := range UnitTestMockUps.BIOSSettingAttr {
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
	if UnitTestMockUps.BIOSVersion == "" {
		var err error
		UnitTestMockUps.BIOSVersion, err = r.RedfishBMC.GetBiosVersion(ctx, systemUUID)
		if err != nil {
			return "", err
		}
	}
	return UnitTestMockUps.BIOSVersion, nil
}

func (r *RedfishLocalBMC) UpgradeBiosVersion(
	ctx context.Context,
	manufacturer string,
	parameters *redfish.SimpleUpdateParameters,
) (string, bool, error) {
	UnitTestMockUps.BIOSUpgradeTaskIndex = 0
	// note, ImageURI is mocked for testing upgrading to version
	UnitTestMockUps.BIOSUpgradingVersion = parameters.ImageURI
	// this go routine mocks the upgrade progress
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BIOSUpgradeTaskIndex < len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BIOSUpgradeTaskIndex = UnitTestMockUps.BIOSUpgradeTaskIndex + 1
		}
	}()
	return "dummyTask", false, nil
}

func (r *RedfishLocalBMC) GetBiosUpgradeTask(
	ctx context.Context,
	manufacturer string,
	taskURI string,
) (*redfish.Task, error) {
	if UnitTestMockUps.BIOSUpgradeTaskIndex > len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
		UnitTestMockUps.BIOSUpgradeTaskIndex = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}
	task := UnitTestMockUps.BIOSUpgradeTaskStatus[UnitTestMockUps.BIOSUpgradeTaskIndex].TaskState
	if task == redfish.CompletedTaskState {
		UnitTestMockUps.BIOSVersion = UnitTestMockUps.BIOSUpgradingVersion
	}
	return &UnitTestMockUps.BIOSUpgradeTaskStatus[UnitTestMockUps.BIOSUpgradeTaskIndex], nil
}

func (r *RedfishLocalBMC) ResetManager(ctx context.Context, UUID string, resetType redfish.ResetType) error {

	// mock the bmc update here with timed delay
	go func() {
		if len(UnitTestMockUps.PendingBMCSetting) > 0 {
			time.Sleep(150 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBMCSetting {
				if _, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
					UnitTestMockUps.BMCSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBMCSetting()
		}
	}()

	return nil
}

// mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBMCAttributesImediately(
	ctx context.Context,
	UUID string,
	attributes redfish.SettingsAttributes,
) (err error) {
	attrs := make(map[string]interface{}, len(attributes))
	for name, value := range attributes {
		attrs[name] = value
	}

	for key, attrData := range attributes {
		if AttributesData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				UnitTestMockUps.PendingBMCSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	return nil
}

func (r *RedfishLocalBMC) GetBMCAttributeValues(
	ctx context.Context,
	UUID string,
	attributes []string,
) (
	result redfish.SettingsAttributes,
	err error,
) {
	if len(attributes) == 0 {
		return
	}
	filteredAttr, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return
	}
	result = make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := UnitTestMockUps.BMCSettingAttr[name]; ok {
				result[name] = AttributesData["value"]
			}
		}
	}
	return result, nil
}

func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(
	ctx context.Context,
	systemUUID string,
) (
	redfish.SettingsAttributes,
	error,
) {
	if len(UnitTestMockUps.PendingBMCSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBMCSetting))

	for key, data := range UnitTestMockUps.PendingBMCSetting {
		result[key] = data["value"]
	}

	return result, nil
}

func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	filtered map[string]redfish.Attribute,
	err error,
) {
	filtered = make(map[string]redfish.Attribute)
	if len(UnitTestMockUps.BMCSettingAttr) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for name, AttributesData := range UnitTestMockUps.BMCSettingAttr {
		data := redfish.Attribute{}
		data.AttributeName = name
		data.Immutable = immutable
		data.ReadOnly = readOnly
		data.Type = AttributesData["type"].(redfish.AttributeType)
		data.ResetRequired = AttributesData["reboot"].(bool)
		filtered[name] = data
	}

	return filtered, err
}

// check if the arrtibutes need to reboot when changed, and are correct type.
// supported attrType, bmc and bios
func (r *RedfishLocalBMC) CheckBMCAttributes(UUID string, attrs redfish.SettingsAttributes) (reset bool, err error) {
	reset = false
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)

	if err != nil {
		return reset, err
	}

	if len(filtered) == 0 {
		return reset, err
	}
	return common.CheckAttribues(attrs, filtered)
}

func (r *RedfishLocalBMC) GetBMCVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BMCVersion == "" {
		var err error
		UnitTestMockUps.BMCVersion, err = r.RedfishBMC.GetBMCVersion(ctx, systemUUID)
		if err != nil {
			return "", err
		}
	}
	return UnitTestMockUps.BMCVersion, nil
}

func (r *RedfishLocalBMC) UpgradeBMCVersion(
	ctx context.Context,
	manufacturer string,
	parameters *redfish.SimpleUpdateParameters,
) (string, bool, error) {
	UnitTestMockUps.BMCUpgradeTaskIndex = 0
	// note, ImageURI is mocked for testing upgrading to version
	UnitTestMockUps.BMCUpgradingVersion = parameters.ImageURI
	// this go routine mocks the upgrade progress
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BMCUpgradeTaskIndex < len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BMCUpgradeTaskIndex = UnitTestMockUps.BMCUpgradeTaskIndex + 1
		}
	}()
	return "dummyTask", false, nil
}

func (r *RedfishLocalBMC) GetBMCUpgradeTask(
	ctx context.Context,
	manufacturer string,
	taskURI string,
) (*redfish.Task, error) {
	if UnitTestMockUps.BMCUpgradeTaskIndex > len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
		UnitTestMockUps.BMCUpgradeTaskIndex = len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
	}
	task := UnitTestMockUps.BMCUpgradeTaskStatus[UnitTestMockUps.BMCUpgradeTaskIndex].TaskState
	if task == redfish.CompletedTaskState {
		UnitTestMockUps.BMCVersion = UnitTestMockUps.BMCUpgradingVersion
	}
	return &UnitTestMockUps.BMCUpgradeTaskStatus[UnitTestMockUps.BMCUpgradeTaskIndex], nil
}

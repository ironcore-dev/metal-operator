// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(ctx context.Context, options Options) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r *RedfishLocalBMC) PowerOn(ctx context.Context, systemURI string) error {
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}

		system, err := r.getSystemFromUri(ctx, systemURI)
		if err != nil {
			ctrl.LoggerFrom(ctx).V(1).Error(err, "failed to get system")
			return
		}

		system.PowerState = redfish.OnPowerState
		system.RawData = nil

		if err := system.Patch(systemURI, system); err != nil {
			ctrl.LoggerFrom(ctx).V(1).Error(err, "failed to Patch system to power on", "SystemID", system.ID)
			return
		}

		// mock the bmc update here
		if len(UnitTestMockUps.PendingBIOSSetting) > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(150 * time.Millisecond):
			}

			for key, data := range UnitTestMockUps.PendingBIOSSetting {
				if _, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
					UnitTestMockUps.BIOSSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBIOSSetting()
		}
	}(ctx)
	return nil
}

func (r *RedfishLocalBMC) PowerOff(ctx context.Context, systemURI string) error {
	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}

		system, err := r.getSystemFromUri(ctx, systemURI)
		if err != nil {
			ctrl.LoggerFrom(ctx).V(1).Error(err, "failed to get system")
			return
		}

		system.PowerState = redfish.OffPowerState
		system.RawData = nil

		if err := system.Patch(systemURI, system); err != nil {
			ctrl.LoggerFrom(ctx).V(1).Error(err, "failed to Patch system to power off", "SystemID", system.ID)
			return
		}
	}(ctx)
	return nil
}

func (r *RedfishLocalBMC) GetBiosPendingAttributeValues(ctx context.Context, systemUUID string) (redfish.SettingsAttributes, error) {
	if len(UnitTestMockUps.PendingBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBIOSSetting))

	for key, data := range UnitTestMockUps.PendingBIOSSetting {
		result[key] = data["value"]
	}

	return result, nil
}

// SetBiosAttributesOnReset mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBiosAttributesOnReset(ctx context.Context, systemUUID string, attributes redfish.SettingsAttributes) error {
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

func (r *RedfishLocalBMC) GetBiosAttributeValues(ctx context.Context, systemUUID string, attributes []string) (redfish.SettingsAttributes, error) {
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

func (r *RedfishLocalBMC) getFilteredBiosRegistryAttributes(readOnly bool, immutable bool) (map[string]RegistryEntryAttributes, error) {
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

// CheckBiosAttributes checks if the attributes need to reboot when changed, and are correct type.
// Supported attrTypes are: bmc and bios.
func (r *RedfishLocalBMC) CheckBiosAttributes(attrs redfish.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)

	if err != nil {
		return false, err
	}

	if len(filtered) == 0 {
		return false, err
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

func (r *RedfishLocalBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BIOSUpgradeTaskIndex = 0
	UnitTestMockUps.BIOSUpgradingVersion = parameters.ImageURI

	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Millisecond):
		}

		for UnitTestMockUps.BIOSUpgradeTaskIndex < len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
			UnitTestMockUps.BIOSUpgradeTaskIndex++
		}
	}(ctx)

	return "dummyTask", false, nil
}

func (r *RedfishLocalBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*redfish.Task, error) {
	if UnitTestMockUps.BIOSUpgradeTaskIndex > len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
		UnitTestMockUps.BIOSUpgradeTaskIndex = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}
	task := UnitTestMockUps.BIOSUpgradeTaskStatus[UnitTestMockUps.BIOSUpgradeTaskIndex].TaskState
	if task == redfish.CompletedTaskState {
		UnitTestMockUps.BIOSVersion = UnitTestMockUps.BIOSUpgradingVersion
	}
	return &UnitTestMockUps.BIOSUpgradeTaskStatus[UnitTestMockUps.BIOSUpgradeTaskIndex], nil
}

// ResetManager resets the BMC manager with the given reset type.
func (r *RedfishLocalBMC) ResetManager(ctx context.Context, UUID string, resetType redfish.ResetType) error {
	go func(ctx context.Context) {
		if len(UnitTestMockUps.PendingBMCSetting) > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(150 * time.Millisecond):
			}

			for key, data := range UnitTestMockUps.PendingBMCSetting {
				if _, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
					UnitTestMockUps.BMCSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBMCSetting()
		}
	}(ctx)

	return nil
}

// SetBMCAttributesImmediately sets given bios attributes.
func (r *RedfishLocalBMC) SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes redfish.SettingsAttributes) (err error) {
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

// GetBMCAttributeValues retrieves the values of the specified BMC attributes.
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes []string) (result redfish.SettingsAttributes, err error) {
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

// GetBMCPendingAttributeValues retrieves the pending BMC attribute values.
func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(ctx context.Context, systemUUID string) (redfish.SettingsAttributes, error) {
	if len(UnitTestMockUps.PendingBMCSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBMCSetting))

	for key, data := range UnitTestMockUps.PendingBMCSetting {
		result[key] = data["value"]
	}

	return result, nil
}

func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(readOnly bool, immutable bool) (filtered map[string]redfish.Attribute, err error) {
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

// CheckBMCAttributes check if the attributes need to reboot when changed, and are of the correct type.
// Supported attrType are: bmc and bios
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

// GetBMCVersion retrieves the BMC version for the given system UUID.
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

// UpgradeBMCVersion upgrades the BMC version to the specified version.
func (r *RedfishLocalBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BMCUpgradeTaskIndex = 0
	UnitTestMockUps.BMCUpgradingVersion = parameters.ImageURI

	go func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Millisecond):
		}

		for UnitTestMockUps.BMCUpgradeTaskIndex < len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
			UnitTestMockUps.BMCUpgradeTaskIndex++
		}
	}(ctx)

	return "dummyTask", false, nil
}

// GetBMCUpgradeTask retrieves the status of the BMC upgrade task.
func (r *RedfishLocalBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*redfish.Task, error) {
	if UnitTestMockUps.BMCUpgradeTaskIndex > len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
		UnitTestMockUps.BMCUpgradeTaskIndex = len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
	}
	task := UnitTestMockUps.BMCUpgradeTaskStatus[UnitTestMockUps.BMCUpgradeTaskIndex].TaskState
	if task == redfish.CompletedTaskState {
		UnitTestMockUps.BMCVersion = UnitTestMockUps.BMCUpgradingVersion
	}
	return &UnitTestMockUps.BMCUpgradeTaskStatus[UnitTestMockUps.BMCUpgradeTaskIndex], nil
}

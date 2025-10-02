// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	gofishCommon "github.com/stmcginnis/gofish/common"

	"github.com/stmcginnis/gofish/redfish"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// A Mutex to protect the shared mock state.
var mockRWMutex sync.RWMutex

// RedfishLocalBMC implements the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(ctx context.Context, options Options) (BMC, error) {
	if UnitTestMockUps.SimulateUnvailableBMC[options.Username] {
		err := &gofishCommon.Error{
			HTTPReturnedStatusCode: 503,
		}
		return nil, err
	}
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create RedfishBMC client: %w", err)
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

// setSystemPowerState updates the power state of a system.
func (r *RedfishLocalBMC) setSystemPowerState(ctx context.Context, systemURI string, state redfish.PowerState) error {
	mockRWMutex.Lock()
	defer mockRWMutex.Unlock()

	time.Sleep(150 * time.Millisecond)

	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}

	system.PowerState = state
	system.RawData = nil
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to patch system to %s: %w", state, err)
	}
	return nil
}

// PowerOn powers on the system asynchronously.
func (r *RedfishLocalBMC) PowerOn(ctx context.Context, systemURI string) error {
	go func() {
		if err := r.setSystemPowerState(ctx, systemURI, redfish.OnPowerState); err != nil {
			log.FromContext(ctx).Error(err, "PowerOn failed", "systemURI", systemURI)
			return
		}

		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()

		// Apply pending BIOS settings after a delay (mock for testing).
		if len(UnitTestMockUps.PendingBIOSSetting[r.options.Username]) > 0 {
			time.Sleep(150 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBIOSSetting[r.options.Username] {
				if _, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username][key]; ok {
					UnitTestMockUps.BIOSSettingAttr[r.options.Username][key] = data
				}
			}
			delete(UnitTestMockUps.PendingBIOSSetting, r.options.Username)
		}
	}()
	return nil
}

// PowerOff powers off the system asynchronously.
func (r *RedfishLocalBMC) PowerOff(ctx context.Context, systemURI string) error {
	go func() {
		if err := r.setSystemPowerState(ctx, systemURI, redfish.OffPowerState); err != nil {
			log.FromContext(ctx).Error(err, "PowerOff failed", "systemURI", systemURI)
		}
	}()
	return nil
}

// GetBiosPendingAttributeValues returns pending BIOS attribute values.
func (r *RedfishLocalBMC) GetBiosPendingAttributeValues(ctx context.Context, systemUUID string) (redfish.SettingsAttributes, error) {
	mockRWMutex.RLock()
	defer mockRWMutex.RUnlock()

	if attr, ok := UnitTestMockUps.PendingBIOSSetting[r.options.Username]; !ok || len(attr) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBIOSSetting[r.options.Username]))

	for key, data := range UnitTestMockUps.PendingBIOSSetting[r.options.Username] {
		result[key] = data["value"]
	}

	return result, nil
}

// SetBiosAttributesOnReset sets BIOS attributes, applying them immediately or on next reset.
func (r *RedfishLocalBMC) SetBiosAttributesOnReset(ctx context.Context, systemUUID string, attributes redfish.SettingsAttributes) error {
	mockRWMutex.Lock()
	defer mockRWMutex.Unlock()

	if value, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username]; !ok || len(value) == 0 {
		UnitTestMockUps.BIOSSettingAttr[r.options.Username] = UnitTestMockUps.BIOSSettingAttr["default"]
	}
	if _, ok := UnitTestMockUps.PendingBIOSSetting[r.options.Username]; !ok {
		UnitTestMockUps.PendingBIOSSetting[r.options.Username] = map[string]map[string]any{}
	}
	for key, attrData := range attributes {
		if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username][key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				UnitTestMockUps.PendingBIOSSetting[r.options.Username][key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	return nil
}

// GetBiosAttributeValues retrieves specific BIOS attribute values.
func (r *RedfishLocalBMC) GetBiosAttributeValues(ctx context.Context, systemUUID string, attributes []string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	mockRWMutex.RLock()
	defer mockRWMutex.RUnlock()

	// The rest of the function now operates on a consistent, locked view of the data.
	if attr, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		UnitTestMockUps.BIOSSettingAttr[r.options.Username] = UnitTestMockUps.BIOSSettingAttr["default"]
	}
	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username][name]; ok {
				result[name] = AttributesData["value"]
			}
		}
	}
	return result, nil
}

// getFilteredBiosRegistryAttributes returns filtered BIOS registry attributes.
func (r *RedfishLocalBMC) getFilteredBiosRegistryAttributes(readOnly, immutable bool) (map[string]RegistryEntryAttributes, error) {
	if attr, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		return nil, fmt.Errorf("no bmc setting attributes found")
	}

	filtered := make(map[string]RegistryEntryAttributes)
	for name, attrData := range UnitTestMockUps.BIOSSettingAttr[r.options.Username] {
		filtered[name] = RegistryEntryAttributes{
			AttributeName: name,
			Immutable:     immutable,
			ReadOnly:      readOnly,
			Type:          attrData["type"].(string),
			ResetRequired: attrData["reboot"].(bool),
		}
	}
	return filtered, nil
}

// CheckBiosAttributes validates BIOS attributes.
func (r *RedfishLocalBMC) CheckBiosAttributes(attrs redfish.SettingsAttributes) (bool, error) {
	if attr, ok := UnitTestMockUps.BIOSSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		mockRWMutex.Lock()
		UnitTestMockUps.BIOSSettingAttr[r.options.Username] = UnitTestMockUps.BIOSSettingAttr["default"]
		mockRWMutex.Unlock()
	}
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return r.checkAttribues(attrs, filtered)
}

// GetBiosVersion retrieves the BIOS version.
func (r *RedfishLocalBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	if value, ok := UnitTestMockUps.BIOSVersion[r.options.Username]; !ok || value == "" {
		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()
		var err error
		UnitTestMockUps.BIOSVersion[r.options.Username], err = r.RedfishBMC.GetBiosVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BIOS version: %w", err)
		}
	}
	return UnitTestMockUps.BIOSVersion[r.options.Username], nil
}

// UpgradeBiosVersion initiates a BIOS upgrade.
func (r *RedfishLocalBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, params *redfish.SimpleUpdateParameters) (string, bool, error) {
	mockRWMutex.Lock()
	UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] = 0
	// note, ImageURI is mocked for testing upgrading to version
	UnitTestMockUps.BIOSUpgradingVersion[r.options.Username] = params.ImageURI
	mockRWMutex.Unlock()
	// this go routine mocks the upgrade progress
	go func() {
		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] < len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] = UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] + 1
		}
	}()
	return "dummyTask", false, nil
}

// GetBiosUpgradeTask retrieves the status of a BIOS upgrade task.
func (r *RedfishLocalBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*redfish.Task, error) {
	mockRWMutex.Lock()
	defer mockRWMutex.Unlock()

	if UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] > len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
		UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username] = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}

	index := UnitTestMockUps.BIOSUpgradeTaskIndex[r.options.Username]
	if index >= len(UnitTestMockUps.BIOSUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BIOSUpgradeTaskStatus[index]
	if task.TaskState == redfish.CompletedTaskState {
		UnitTestMockUps.BIOSVersion[r.options.Username] = UnitTestMockUps.BIOSUpgradingVersion[r.options.Username]
	}
	return task, nil
}

// ResetManager resets the BMC with a delay for pending settings.
func (r *RedfishLocalBMC) ResetManager(ctx context.Context, UUID string, resetType redfish.ResetType) error {
	go func() {
		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()

		if len(UnitTestMockUps.PendingBMCSetting[r.options.Username]) > 0 {
			time.Sleep(150 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBMCSetting[r.options.Username] {
				if _, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username][key]; ok {
					UnitTestMockUps.BMCSettingAttr[r.options.Username][key] = data
				}
			}
			delete(UnitTestMockUps.PendingBMCSetting, r.options.Username)
		}
	}()
	return nil
}

// SetBMCAttributesImmediately sets BMC attributes, applying them immediately or on reset.
func (r *RedfishLocalBMC) SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes redfish.SettingsAttributes) error {
	mockRWMutex.Lock()
	defer mockRWMutex.Unlock()
	if value, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username]; !ok || len(value) == 0 {
		UnitTestMockUps.BMCSettingAttr[r.options.Username] = UnitTestMockUps.BMCSettingAttr["default"]
	}
	if _, ok := UnitTestMockUps.PendingBMCSetting[r.options.Username]; !ok {
		UnitTestMockUps.PendingBMCSetting[r.options.Username] = map[string]map[string]any{}
	}
	for key, value := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username][key]; ok {
			if reboot, ok := attrData["reboot"].(bool); ok && !reboot {
				attrData["value"] = value
			} else {
				UnitTestMockUps.PendingBMCSetting[r.options.Username][key] = map[string]interface{}{
					"type":   attrData["type"],
					"reboot": attrData["reboot"],
					"value":  value,
				}
			}
		}
	}
	return nil
}

// GetBMCAttributeValues retrieves specific BMC attribute values.
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes []string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	if attr, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		mockRWMutex.Lock()
		UnitTestMockUps.BMCSettingAttr[r.options.Username] = UnitTestMockUps.BMCSettingAttr["default"]
		mockRWMutex.Unlock()
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username][name]; ok && filtered[name].AttributeName != "" {
			result[name] = attrData["value"]
		}
	}
	return result, nil
}

// GetBMCPendingAttributeValues returns pending BMC attribute values.
func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(ctx context.Context, systemUUID string) (redfish.SettingsAttributes, error) {
	pending := UnitTestMockUps.PendingBMCSetting[r.options.Username]
	if len(pending) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(pending))
	for key, data := range pending {
		result[key] = data["value"]
	}
	return result, nil
}

// getFilteredBMCRegistryAttributes returns filtered BMC registry attributes.
func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(readOnly, immutable bool) (map[string]redfish.Attribute, error) {
	if attr, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		return nil, fmt.Errorf("no BMC setting attributes found")
	}

	filtered := make(map[string]redfish.Attribute)
	for name, attrData := range UnitTestMockUps.BMCSettingAttr[r.options.Username] {
		filtered[name] = redfish.Attribute{
			AttributeName: name,
			Immutable:     immutable,
			ReadOnly:      readOnly,
			Type:          attrData["type"].(redfish.AttributeType),
			ResetRequired: attrData["reboot"].(bool),
		}
	}
	return filtered, nil
}

// CheckBMCAttributes validates BMC attributes.
func (r *RedfishLocalBMC) CheckBMCAttributes(UUID string, attrs redfish.SettingsAttributes) (bool, error) {
	if attr, ok := UnitTestMockUps.BMCSettingAttr[r.options.Username]; !ok || len(attr) == 0 {
		mockRWMutex.Lock()
		UnitTestMockUps.BMCSettingAttr[r.options.Username] = UnitTestMockUps.BMCSettingAttr["default"]
		mockRWMutex.Unlock()
	}
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return common.CheckAttribues(attrs, filtered)
}

// GetBMCVersion retrieves the BMC version.
func (r *RedfishLocalBMC) GetBMCVersion(ctx context.Context, systemUUID string) (string, error) {
	if ver, ok := UnitTestMockUps.BMCVersion[r.options.Username]; ver == "" || !ok {
		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()
		var err error
		UnitTestMockUps.BMCVersion[r.options.Username], err = r.RedfishBMC.GetBMCVersion(ctx, systemUUID)
		if err != nil {
			return "", err
		}
	}
	return UnitTestMockUps.BMCVersion[r.options.Username], nil
}

// UpgradeBMCVersion initiates a BMC upgrade.
func (r *RedfishLocalBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, params *redfish.SimpleUpdateParameters) (string, bool, error) {
	mockRWMutex.Lock()
	UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username] = 0
	UnitTestMockUps.BMCUpgradingVersion[r.options.Username] = params.ImageURI
	mockRWMutex.Unlock()

	go func() {
		mockRWMutex.Lock()
		defer mockRWMutex.Unlock()

		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username] < len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username] = UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username] + 1
		}
	}()
	return "dummyTask", false, nil
}

// GetBMCUpgradeTask retrieves the status of a BMC upgrade task.
func (r *RedfishLocalBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*redfish.Task, error) {
	mockRWMutex.Lock()
	defer mockRWMutex.Unlock()
	if _, ok := UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username]; !ok {
		UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username] = 0
	}
	index := UnitTestMockUps.BMCUpgradeTaskIndex[r.options.Username]
	if index >= len(UnitTestMockUps.BMCUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BMCUpgradeTaskStatus[index]
	if task.TaskState == redfish.CompletedTaskState {
		UnitTestMockUps.BMCVersion[r.options.Username] = UnitTestMockUps.BMCUpgradingVersion[r.options.Username]
	}
	return task, nil
}

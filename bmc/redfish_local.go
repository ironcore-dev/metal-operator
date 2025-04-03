// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stmcginnis/gofish/redfish"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ BMC = (*RedfishLocalBMC)(nil)

var defaultMockedBMCSetting = []map[string]any{
	{"name": "abc", "type": string(TypeString), "reboot": false, "value": "blah"},
	{"name": "fooreboot", "type": string(TypeInteger), "reboot": true, "value": 123},
}

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
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

	go func() {
		time.Sleep(250 * time.Millisecond)
		system, err := r.getSystemByUUID(ctx, systemUUID)
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
			log.V(1).Error(err, "failed to Patch system to power on", "systemUUID", systemUUID)
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

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemUUID string) error {

	go func() {
		time.Sleep(250 * time.Millisecond)
		system, err := r.getSystemByUUID(ctx, systemUUID)
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
			log.V(1).Error(err, "failed to Patch system to power off", "systemUUID", systemUUID)
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

// mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBMCAttributesImediately(
	ctx context.Context,
	attributes redfish.SettingsAttributes,
) (err error) {
	attrs := make(map[string]interface{}, len(attributes))
	for name, value := range attributes {
		attrs[name] = value
	}
	if len(defaultMockedBMCSetting) == 0 {
		defaultMockedBMCSetting = []map[string]any{}
	}

	for key, attr := range attributes {
		for _, eachMock := range defaultMockedBMCSetting {
			if value, ok := eachMock["name"]; ok && value == key {
				eachMock["value"] = attr
			}
		}
	}
	r.StoredBMCSettingData = defaultMockedBMCSetting
	return nil
}

// func (r *RedfishLocalBMC) getMockedBIOSSettingData() map[string]any {

// 	if len(r.StoredBIOSSettingData) > 0 {
// 		return r.StoredBIOSSettingData
// 	}
// 	return map[string]any{"abc": "blah", "fooreboot": 123}

// }

func (r *RedfishLocalBMC) getMockedBMCSettingData() []map[string]any {

	if len(r.StoredBMCSettingData) > 0 {
		return r.StoredBMCSettingData
	}
	return defaultMockedBMCSetting

}

func (r *RedfishLocalBMC) GetBMCAttributeValues(
	ctx context.Context,
	attributes []string,
) (
	result redfish.SettingsAttributes,
	err error,
) {

	if len(attributes) == 0 {
		return
	}

	mockedAttributes := r.getMockedBMCSettingData()

	filteredAttr, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return
	}
	result = make(map[string]interface{}, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			for _, eachMock := range mockedAttributes {
				if value, ok := eachMock["name"]; ok && value == name {
					result[name] = eachMock["value"]
					break
				}
			}
		}
	}
	return
}

func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	filtered map[string]RegistryEntryAttributes,
	err error,
) {
	mockedAttributes := r.getMockedBMCSettingData()
	filtered = make(map[string]RegistryEntryAttributes)
	if len(mockedAttributes) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for _, eachMock := range mockedAttributes {
		data := RegistryEntryAttributes{}
		data.AttributeName = eachMock["name"].(string)
		data.Immutable = immutable
		data.ReadOnly = readOnly
		data.Type = eachMock["type"].(string)
		data.ResetRequired = eachMock["reboot"].(bool)
		filtered[eachMock["name"].(string)] = data
	}
	return filtered, err
}

// check if the arrtibutes need to reboot when changed, and are correct type.
// supported attrType, bmc and bios
func (r *RedfishLocalBMC) CheckBMCAttributes(attrs redfish.SettingsAttributes) (reset bool, err error) {
	reset = false
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)

	if err != nil {
		return reset, err
	}

	if len(filtered) == 0 {
		return reset, err
	}
	//TODO: add more types like maps etc
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			err = errors.Join(err, fmt.Errorf("attribute %s not found or immutable/hidden. attr present %v", name, filtered))
			continue
		}
		if entryAttribute.ResetRequired {
			reset = true
		}
		switch strings.ToLower(entryAttribute.Type) {
		case string(TypeInteger):
			if _, ok := value.(int); !ok {
				err = errors.Join(
					err,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case string(TypeString):
			if _, ok := value.(string); !ok {
				err = errors.Join(
					err,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case string(TypeEnumerations):
			if _, ok := value.(string); !ok {
				err = errors.Join(
					err,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
			var invalidEnum bool
			for _, attrValue := range entryAttribute.Value {
				if attrValue.ValueName == value.(string) {
					invalidEnum = true
					break
				}
			}
			if !invalidEnum {
				err = errors.Join(err, fmt.Errorf("attribute %s value is unknown. needed %v", name, entryAttribute.Value))
			}
			continue
		default:
			err = errors.Join(
				err,
				fmt.Errorf("attribute %s value has wrong type. needed %s for %v ",
					name,
					entryAttribute.Type,
					entryAttribute,
				))
		}
	}
	return reset, err
}

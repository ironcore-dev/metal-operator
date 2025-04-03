// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

var defaultMockedBMCSetting = []map[string]any{
	{"name": "abc", "type": string(TypeString), "reboot": false, "value": "blah"},
	{"name": "fooreboot", "type": string(TypeInteger), "reboot": true, "value": 123},
}

var defaultMockedBIOSSetting = map[string]map[string]any{
	"abc":       {"type": "string", "reboot": false, "value": "bar"},
	"fooreboot": {"type": "integer", "reboot": true, "value": 123},
}

var pendingMockedBIOSSetting = map[string]map[string]any{}

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
	if len(pendingMockedBIOSSetting) > 0 {

		for key, data := range pendingMockedBIOSSetting {
			if _, ok := defaultMockedBIOSSetting[key]; ok {
				defaultMockedBIOSSetting[key] = data
			}
		}
		pendingMockedBIOSSetting = map[string]map[string]any{}
		r.StoredBIOSSettingData = defaultMockedBIOSSetting
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
	if len(pendingMockedBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(pendingMockedBIOSSetting))

	for key, data := range pendingMockedBIOSSetting {
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
	if len(defaultMockedBIOSSetting) == 0 {
		defaultMockedBIOSSetting = map[string]map[string]any{}
	}

	pendingMockedBIOSSetting = map[string]map[string]any{}
	for key, attrData := range attributes {
		if AttributesData, ok := defaultMockedBIOSSetting[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				pendingMockedBIOSSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	r.StoredBIOSSettingData = defaultMockedBIOSSetting

	return nil
}

func (r *RedfishLocalBMC) getMockedBIOSSettingData() map[string]map[string]any {

	if len(r.StoredBIOSSettingData) > 0 {
		return r.StoredBIOSSettingData
	}
	return defaultMockedBIOSSetting

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

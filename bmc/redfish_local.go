// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

var defaultMockedBIOSSetting = map[string]map[string]any{
	"abc":       {"type": "string", "reboot": false, "value": "bar"},
	"fooreboot": {"type": "integer", "reboot": true, "value": 123},
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

// mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBiosAttributesOnReset(
	ctx context.Context,
	systemUUID string,
	attributes redfish.SettingsAttributes,
) error {
	if len(defaultMockedBIOSSetting) == 0 {
		defaultMockedBIOSSetting = map[string]map[string]any{}
	}

	for key, attrData := range attributes {
		if AttributesData, ok := defaultMockedBIOSSetting[key]; ok {
			AttributesData["value"] = attrData
		}
	}
	r.StoredBMCSettingData = defaultMockedBIOSSetting
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
	return r.checkBiosAttribues(attrs, filtered)
}

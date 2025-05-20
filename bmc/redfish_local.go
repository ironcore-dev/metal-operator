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
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	patch := *system
	patch.RawData = nil
	patch.PowerState = redfish.OnPowerState
	if err := system.Patch(systemURI, patch); err != nil {
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
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	patch := *system
	patch.RawData = nil
	patch.PowerState = redfish.OffPowerState
	if err := system.Patch(systemURI, patch); err != nil {
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

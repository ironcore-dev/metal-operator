// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	gofishCommon "github.com/stmcginnis/gofish/common"

	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

const (
	DummyMockTaskForUpgrade = "dummyTask"
)

// RedfishLocalBMC implements the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(ctx context.Context, options Options) (BMC, error) {
	if UnitTestMockUps == nil {
		InitMockUp()
	}
	if UnitTestMockUps.SimulateUnvailableBMC {
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

// SetBMCAttributesImmediately sets BMC attributes, applying them immediately or on reset.
func (r *RedfishLocalBMC) SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes redfish.SettingsAttributes) error {
	for key, value := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
			if reboot, ok := attrData["reboot"].(bool); ok && !reboot {
				attrData["value"] = value
			} else {
				UnitTestMockUps.PendingBMCSetting[key] = map[string]interface{}{
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
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	result := make(redfish.SettingsAttributes, len(attributes))
	for key := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok && filtered[key].AttributeName != "" {
			result[key] = attrData["value"]
		}
	}
	return result, nil
}

// GetBMCPendingAttributeValues returns pending BMC attribute values.
func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(ctx context.Context, systemUUID string) (redfish.SettingsAttributes, error) {
	pending := UnitTestMockUps.PendingBMCSetting
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
	if len(UnitTestMockUps.BMCSettingAttr) == 0 {
		return nil, fmt.Errorf("no BMC setting attributes found")
	}

	filtered := make(map[string]redfish.Attribute)
	for name, attrData := range UnitTestMockUps.BMCSettingAttr {
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
func (r *RedfishLocalBMC) CheckBMCAttributes(ctx context.Context, UUID string, attrs redfish.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return common.CheckAttribues(attrs, filtered)
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	gofishCommon "github.com/stmcginnis/gofish/common"

	"github.com/stmcginnis/gofish/redfish"
	ctrl "sigs.k8s.io/controller-runtime"
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
		return nil, err
	}
	if acc, ok := UnitTestMockUps.Accounts[options.Username]; ok {
		if acc.Password != options.Password {
			return nil, &gofishCommon.Error{
				HTTPReturnedStatusCode: 401,
			}
		}
	} else {
		return nil, &gofishCommon.Error{
			HTTPReturnedStatusCode: 401,
		}
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

// GetAccounts retrieves all user accounts from the BMC.
func (r *RedfishLocalBMC) GetAccounts(ctx context.Context) ([]*redfish.ManagerAccount, error) {
	accounts := make([]*redfish.ManagerAccount, 0, len(UnitTestMockUps.Accounts))
	for _, a := range UnitTestMockUps.Accounts {
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// CreateOrUpdateAccount creates or updates a user account on the BMC.
func (r *RedfishLocalBMC) CreateOrUpdateAccount(
	ctx context.Context, userName, role, password string, enabled bool,
) error {
	for _, a := range UnitTestMockUps.Accounts {
		if a.UserName == userName {
			a.RoleID = role
			a.UserName = userName
			a.Enabled = enabled
			a.Password = password
			return nil
		}
	}
	newAccount := redfish.ManagerAccount{
		Entity: gofishCommon.Entity{
			ID: fmt.Sprintf("%d", len(UnitTestMockUps.Accounts)+1),
		},
		UserName: userName,
		RoleID:   role,
		Enabled:  enabled,
		Password: password,
	}
	UnitTestMockUps.Accounts[userName] = &newAccount
	return nil
}

func (r *RedfishLocalBMC) DeleteAccount(ctx context.Context, userName, id string) error {
	if _, ok := UnitTestMockUps.Accounts[userName]; ok {
		delete(UnitTestMockUps.Accounts, userName)
		return nil
	}
	return fmt.Errorf("account %s not found", userName)
}

// GetBiosVersion retrieves the BIOS version.
func (r *RedfishLocalBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BIOSVersion == "" {
		var err error
		UnitTestMockUps.BIOSVersion, err = r.RedfishBMC.GetBiosVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BIOS version: %w", err)
		}
	}
	return UnitTestMockUps.BIOSVersion, nil
}

// UpgradeBiosVersion initiates a BIOS upgrade.
func (r *RedfishLocalBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, params *redfish.SimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BIOSUpgradeTaskIndex = 0
	UnitTestMockUps.BIOSUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BIOSUpgradeTaskIndex < len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BIOSUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBiosUpgradeTask retrieves the status of a BIOS upgrade task.
func (r *RedfishLocalBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*redfish.Task, error) {
	index := UnitTestMockUps.BIOSUpgradeTaskIndex
	if index >= len(UnitTestMockUps.BIOSUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BIOSUpgradeTaskStatus[index]
	if task.TaskState == redfish.CompletedTaskState {
		UnitTestMockUps.BIOSVersion = UnitTestMockUps.BIOSUpgradingVersion
	}
	return task, nil
}

// ResetManager resets the BMC with a delay for pending settings.
func (r *RedfishLocalBMC) ResetManager(ctx context.Context, UUID string, resetType redfish.ResetType) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Simulating BMC reset", "UUID", UUID, "ResetType", resetType)
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
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes []string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[name]; ok && filtered[name].AttributeName != "" {
			result[name] = attrData["value"]
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
func (r *RedfishLocalBMC) CheckBMCAttributes(UUID string, attrs redfish.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return common.CheckAttribues(attrs, filtered)
}

// GetBMCVersion retrieves the BMC version.
func (r *RedfishLocalBMC) GetBMCVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BMCVersion == "" {
		var err error
		UnitTestMockUps.BMCVersion, err = r.RedfishBMC.GetBMCVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BMC version: %w", err)
		}
	}
	return UnitTestMockUps.BMCVersion, nil
}

// UpgradeBMCVersion initiates a BMC upgrade.
func (r *RedfishLocalBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, params *redfish.SimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BMCUpgradeTaskIndex = 0
	UnitTestMockUps.BMCUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BMCUpgradeTaskIndex < len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BMCUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBMCUpgradeTask retrieves the status of a BMC upgrade task.
func (r *RedfishLocalBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*redfish.Task, error) {
	index := UnitTestMockUps.BMCUpgradeTaskIndex
	if index >= len(UnitTestMockUps.BMCUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BMCUpgradeTaskStatus[index]
	if task.TaskState == redfish.CompletedTaskState {
		UnitTestMockUps.BMCVersion = UnitTestMockUps.BMCUpgradingVersion
	}
	return task, nil
}

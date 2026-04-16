// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stmcginnis/gofish/schemas"
)

var _ BMC = (*RedfishLocalBMC)(nil)

const (
	DummyMockTaskForUpgrade = "dummyTask"
)

// RedfishLocalBMC implements the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBaseBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(ctx context.Context, options Options) (BMC, error) {
	if UnitTestMockUps == nil {
		InitMockUp()
	}
	if UnitTestMockUps.SimulateUnvailableBMC {
		err := &schemas.Error{
			HTTPReturnedStatusCode: 503,
		}
		return nil, err
	}
	bmc, err := newRedfishBaseBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	if acc, ok := UnitTestMockUps.Accounts[options.Username]; ok {
		if acc.Password == options.Password {
			// authenticated
			return &RedfishLocalBMC{RedfishBaseBMC: bmc}, nil
		}
	}
	return nil, &schemas.Error{
		HTTPReturnedStatusCode: 401,
	}
}

// GetAccounts retrieves all user accounts from the BMC.
func (r *RedfishLocalBMC) GetAccounts() ([]*schemas.ManagerAccount, error) {
	accounts := make([]*schemas.ManagerAccount, 0, len(UnitTestMockUps.Accounts))
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
	newAccount := schemas.ManagerAccount{
		Entity: schemas.Entity{
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
		UnitTestMockUps.BIOSVersion, err = r.RedfishBaseBMC.GetBiosVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BIOS version: %w", err)
		}
	}
	return UnitTestMockUps.BIOSVersion, nil
}

// UpgradeBiosVersion initiates a BIOS upgrade.
func (r *RedfishLocalBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BIOSUpgradeTaskIndex = 0
	UnitTestMockUps.BIOSUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		lenTask := len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
		if strings.Contains(params.ImageURI, "fail") {
			lenTask = len(UnitTestMockUps.BIOSUpgradeTaskFailedStatus) - 1
		}
		for UnitTestMockUps.BIOSUpgradeTaskIndex < lenTask {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BIOSUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBiosUpgradeTask retrieves the status of a BIOS upgrade task.
func (r *RedfishLocalBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	index := UnitTestMockUps.BIOSUpgradeTaskIndex
	taskStatus := UnitTestMockUps.BIOSUpgradeTaskStatus
	if strings.Contains(UnitTestMockUps.BIOSUpgradingVersion, "fail") {
		taskStatus = UnitTestMockUps.BIOSUpgradeTaskFailedStatus
	}

	if index >= len(taskStatus) {
		index = len(taskStatus) - 1
	}
	task := &taskStatus[index]
	if task.TaskState == schemas.CompletedTaskState {
		UnitTestMockUps.BIOSVersion = UnitTestMockUps.BIOSUpgradingVersion
	}
	return task, nil
}

// SetBMCAttributesImmediately sets BMC attributes via HTTP PATCH to the BMC Settings endpoint.
// Navigates from the manager's @Redfish.Settings.SettingsObject link, mirroring the Dell pattern.
func (r *RedfishLocalBMC) SetBMCAttributesImmediately(ctx context.Context, bmcUUID string, attributes schemas.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}
	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return fmt.Errorf("failed to get manager: %w", err)
	}
	var managerData struct {
		Settings schemas.Settings `json:"@Redfish.Settings"`
	}
	if err := json.Unmarshal(manager.RawData, &managerData); err != nil {
		return fmt.Errorf("failed to parse manager data: %w", err)
	}
	data := map[string]any{
		"Attributes":                 attributes,
		"@Redfish.SettingsApplyTime": map[string]string{"ApplyTime": string(schemas.ImmediateSettingsApplyTime)},
	}
	resp, err := manager.GetClient().Patch(managerData.Settings.SettingsObject, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("PATCH %s returned status %d", managerData.Settings.SettingsObject, resp.StatusCode)
	}
	return nil
}

// GetBMCAttributeValues retrieves specific BMC attribute values via HTTP from the BMC manager.
// Integer-typed attributes are converted from float64 (JSON default) to int to match controller expectations.
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(ctx, false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	manager, err := r.GetManager(UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get BMC manager: %w", err)
	}

	var raw struct {
		Attributes schemas.SettingsAttributes `json:"Attributes"`
	}
	if err := json.Unmarshal(manager.RawData, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse manager attributes: %w", err)
	}

	result := make(schemas.SettingsAttributes, len(attributes))
	for key := range attributes {
		entry, ok := filtered[key]
		if !ok {
			continue
		}
		val := raw.Attributes[key]
		// JSON numbers are float64; convert to int for integer-typed attributes so
		// the controller's type switch produces int-typed diff values for checkAttributes.
		if strings.EqualFold(string(entry.Type), "integer") {
			if f, ok := val.(float64); ok {
				val = int(f)
			}
		}
		result[key] = val
	}
	return result, nil
}

// GetBMCPendingAttributeValues returns pending BMC attribute values by navigating the manager's
// @Redfish.Settings.SettingsObject link, mirroring the Dell pattern.
func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(ctx context.Context, uuid string) (schemas.SettingsAttributes, error) {
	manager, err := r.GetManager(uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get manager: %w", err)
	}
	var managerData struct {
		Settings schemas.Settings `json:"@Redfish.Settings"`
	}
	if err := json.Unmarshal(manager.RawData, &managerData); err != nil {
		return nil, fmt.Errorf("failed to parse manager data: %w", err)
	}
	var pending struct {
		Attributes schemas.SettingsAttributes `json:"Attributes"`
	}
	if err := r.GetEntityFromUri(ctx, managerData.Settings.SettingsObject, manager.GetClient(), &pending); err != nil {
		return nil, fmt.Errorf("failed to get pending BMC attributes: %w", err)
	}
	return pending.Attributes, nil
}

// getFilteredBMCRegistryAttributes fetches the BMC attribute registry from the server and
// filters by readOnly / immutable flags, returning a map keyed by attribute name.
func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(ctx context.Context, readOnly, immutable bool) (map[string]schemas.Attributes, error) {
	var bmcRegistry schemas.AttributeRegistry
	if err := r.GetEntityFromUri(ctx, "/redfish/v1/Registries/BMCAttributeRegistry", r.client.GetService().GetClient(), &bmcRegistry); err != nil {
		return nil, fmt.Errorf("failed to fetch BMC attribute registry: %w", err)
	}

	filtered := make(map[string]schemas.Attributes)
	for _, entry := range bmcRegistry.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filtered[entry.AttributeName] = entry
		}
	}
	return filtered, nil
}

// CheckBMCAttributes validates BMC attributes against the server-side registry.
func (r *RedfishLocalBMC) CheckBMCAttributes(ctx context.Context, UUID string, attrs schemas.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBMCRegistryAttributes(ctx, false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return checkAttributes(attrs, filtered)
}

// GetBMCVersion retrieves the BMC version.
func (r *RedfishLocalBMC) GetBMCVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BMCVersion == "" {
		var err error
		UnitTestMockUps.BMCVersion, err = r.RedfishBaseBMC.GetBMCVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BMC version: %w", err)
		}
	}
	return UnitTestMockUps.BMCVersion, nil
}

// UpgradeBMCVersion initiates a BMC upgrade.
func (r *RedfishLocalBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BMCUpgradeTaskIndex = 0
	UnitTestMockUps.BMCUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		lenTask := len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
		if strings.Contains(params.ImageURI, "fail") {
			lenTask = len(UnitTestMockUps.BMCUpgradeTaskFailedStatus) - 1
		}
		for UnitTestMockUps.BMCUpgradeTaskIndex < lenTask {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BMCUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBMCUpgradeTask retrieves the status of a BMC upgrade task.
func (r *RedfishLocalBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	index := UnitTestMockUps.BMCUpgradeTaskIndex

	taskStatus := UnitTestMockUps.BMCUpgradeTaskStatus
	if strings.Contains(UnitTestMockUps.BMCUpgradingVersion, "fail") {
		taskStatus = UnitTestMockUps.BMCUpgradeTaskFailedStatus
	}

	if index >= len(taskStatus) {
		index = len(taskStatus) - 1
	}
	task := &taskStatus[index]
	if task.TaskState == schemas.CompletedTaskState {
		UnitTestMockUps.BMCVersion = UnitTestMockUps.BMCUpgradingVersion
	}
	return task, nil
}

// CheckBMCPendingComponentUpgrade returns false for local provider.
// This is the expected behavior for non-real hardware environments; vendor implementations
// (Dell, HPE, Lenovo) override this to check actual firmware inventory.
func (r *RedfishLocalBMC) CheckBMCPendingComponentUpgrade(_ context.Context, _ ComponentType) (bool, error) {
	return false, nil
}

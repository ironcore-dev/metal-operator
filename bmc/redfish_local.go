// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ BMC = (*RedfishLocalBMC)(nil)

const (
	DummyMockTaskForUpgrade = "/redfish/v1/TaskService/Tasks/upgrade"
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

// UpgradeBiosVersion initiates a BIOS upgrade via HTTP to the mock Redfish server.
func (r *RedfishLocalBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	p := *params
	if !strings.HasPrefix(params.ImageURI, "bios://") {
		systems, err := r.GetSystems(ctx)
		if err != nil || len(systems) == 0 {
			return "", true, fmt.Errorf("failed to resolve system URI for BIOS upgrade: %w", err)
		}
		p.ImageURI = "bios://" + systems[0].URI + "/" + url.PathEscape(params.ImageURI)
	}
	return upgradeVersion(ctx, r.RedfishBaseBMC, &p, localBuildRequestBody, localExtractTaskURI)
}

// GetBiosUpgradeTask retrieves the status of a BIOS upgrade task via HTTP.
func (r *RedfishLocalBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, localParseTask)
}

// ResetManager resets the BMC with a delay for pending settings.
func (r *RedfishLocalBMC) ResetManager(ctx context.Context, UUID string, resetType schemas.ResetType) error {
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
func (r *RedfishLocalBMC) SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes schemas.SettingsAttributes) error {
	for key, value := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
			if reboot, ok := attrData["reboot"].(bool); ok && !reboot {
				attrData["value"] = value
			} else {
				UnitTestMockUps.PendingBMCSetting[key] = map[string]any{
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
func (r *RedfishLocalBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	result := make(schemas.SettingsAttributes, len(attributes))
	for key := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok && filtered[key].AttributeName != "" {
			result[key] = attrData["value"]
		}
	}
	return result, nil
}

// GetBMCPendingAttributeValues returns pending BMC attribute values.
func (r *RedfishLocalBMC) GetBMCPendingAttributeValues(ctx context.Context, systemUUID string) (schemas.SettingsAttributes, error) {
	pending := UnitTestMockUps.PendingBMCSetting
	if len(pending) == 0 {
		return schemas.SettingsAttributes{}, nil
	}

	result := make(schemas.SettingsAttributes, len(pending))
	for key, data := range pending {
		result[key] = data["value"]
	}
	return result, nil
}

// getFilteredBMCRegistryAttributes returns filtered BMC registry attributes.
func (r *RedfishLocalBMC) getFilteredBMCRegistryAttributes(readOnly, immutable bool) (map[string]schemas.Attributes, error) {
	if len(UnitTestMockUps.BMCSettingAttr) == 0 {
		return nil, fmt.Errorf("no BMC setting attributes found")
	}

	filtered := make(map[string]schemas.Attributes)
	for name, attrData := range UnitTestMockUps.BMCSettingAttr {
		filtered[name] = schemas.Attributes{
			AttributeName: name,
			Immutable:     immutable,
			ReadOnly:      readOnly,
			Type:          attrData["type"].(schemas.AttributeType),
			ResetRequired: attrData["reboot"].(bool),
		}
	}
	return filtered, nil
}

// CheckBMCAttributes validates BMC attributes.
func (r *RedfishLocalBMC) CheckBMCAttributes(ctx context.Context, UUID string, attrs schemas.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return checkAttributes(attrs, filtered)
}

// UpgradeBMCVersion initiates a BMC upgrade via HTTP to the mock Redfish server.
func (r *RedfishLocalBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	p := *params
	if !strings.HasPrefix(params.ImageURI, "bmc://") {
		p.ImageURI = "bmc://" + params.ImageURI
	}
	return upgradeVersion(ctx, r.RedfishBaseBMC, &p, localBuildRequestBody, localExtractTaskURI)
}

// GetBMCUpgradeTask retrieves the status of a BMC upgrade task via HTTP.
func (r *RedfishLocalBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, localParseTask)
}

func localBuildRequestBody(params *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody {
	return &SimpleUpdateRequestBody{
		UpdateServiceSimpleUpdateParameters: *params,
	}
}

func localExtractTaskURI(resp *http.Response) (string, error) {
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	var taskResp struct {
		OdataID string `json:"@odata.id"`
	}
	if err := json.Unmarshal(rawBody, &taskResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal task URI: %w", err)
	}
	return taskResp.OdataID, nil
}

func localParseTask(_ context.Context, resp *http.Response) (*schemas.Task, error) {
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	task := &schemas.Task{}
	if err := json.Unmarshal(rawBody, task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %w", err)
	}
	return task, nil
}

// CheckBMCPendingComponentUpgrade returns false for local provider.
// This is the expected behavior for non-real hardware environments; vendor implementations
// (Dell, HPE, Lenovo) override this to check actual firmware inventory.
func (r *RedfishLocalBMC) CheckBMCPendingComponentUpgrade(_ context.Context, _ ComponentType) (bool, error) {
	return false, nil
}

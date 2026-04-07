// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

// LenovoRedfishBMC is the Lenovo-specific implementation of the BMC interface.
type LenovoRedfishBMC struct {
	*RedfishBaseBMC
}

// --- BMC interface method overrides ---

func (r *LenovoRedfishBMC) GetBMCAttributeValues(ctx context.Context, bmcUUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	result, err := httpBasedGetBMCSettingAttribute(r.client.GetService().GetClient(), attributes)
	if err != nil {
		return result, err
	}
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Fetched data from BMC Settings ", "Result", result)
	return result, nil
}

func (r *LenovoRedfishBMC) GetBMCPendingAttributeValues(_ context.Context, _ string) (schemas.SettingsAttributes, error) {
	return schemas.SettingsAttributes{}, nil
}

func (r *LenovoRedfishBMC) SetBMCAttributesImmediately(ctx context.Context, _ string, attributes schemas.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}
	return httpBasedUpdateBMCAttributes(r.client.GetService().GetClient(), attributes, schemas.ImmediateSettingsApplyTime)
}

func (r *LenovoRedfishBMC) CheckBMCAttributes(_ context.Context, _ string, _ schemas.SettingsAttributes) (bool, error) {
	return false, nil
}

// --- Firmware upgrade overrides ---

func (r *LenovoRedfishBMC) lenovoBuildRequestBody(parameters *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Password = parameters.Password
	body.Username = parameters.Username
	body.Targets = parameters.Targets
	body.TransferProtocol = parameters.TransferProtocol
	return body
}

func (r *LenovoRedfishBMC) lenovoExtractTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	var tResp struct {
		TaskMonitor string `json:"@odata.id,omitempty"`
	}
	if err = json.Unmarshal(rawBody, &tResp); err != nil {
		return "", fmt.Errorf("failed to Unmarshal taskMonitor URI %v", err)
	}

	if tResp.TaskMonitor == "" {
		return "", fmt.Errorf("lenovoExtractTaskMonitorURI: missing @odata.id in response")
	}

	return tResp.TaskMonitor, nil
}

func (r *LenovoRedfishBMC) lenovoParseTaskDetails(ctx context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
	rawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(rawBody, &task); err != nil {
		return nil, err
	}

	if len(task.Messages) > 0 && task.TaskState == schemas.CompletedTaskState && task.TaskStatus == schemas.OKHealth {
		for _, msg := range task.Messages {
			if strings.Contains(msg.MessageID, "OperationTransitionedToJob") && len(msg.MessageArgs) > 0 {
				respJob, err := r.client.GetService().GetClient().Get(msg.MessageArgs[0])
				if err != nil {
					return nil, err
				}
				defer respJob.Body.Close() // nolint: errcheck

				if respJob.StatusCode != http.StatusAccepted && respJob.StatusCode != http.StatusOK {
					respJobRawBody, err := io.ReadAll(respJob.Body)
					if err != nil {
						return nil,
							fmt.Errorf("failed to get the upgrade Task details. and read the response body %v, statusCode %v",
								err, respJob.StatusCode)
					}
					return nil,
						fmt.Errorf("failed to get the upgrade Task details. %v, statusCode %v",
							string(respJobRawBody), respJob.StatusCode)
				}

				respJobRawBody, err := io.ReadAll(respJob.Body)
				if err != nil {
					return nil,
						fmt.Errorf("failed to get the upgrade Task details. and read the response body %v, statusCode %v",
							err, respJob.StatusCode)
				}

				job := &schemas.Job{}
				if err = json.Unmarshal(respJobRawBody, &job); err != nil {
					return nil, err
				}
				task = &schemas.Task{}
				task.ID = job.ID
				task.ODataID = job.ODataID
				task.Description = job.Description
				task.StartTime = job.StartTime
				task.EndTime = job.EndTime
				task.PercentComplete = job.PercentComplete
				task.TaskState = schemas.TaskState(job.JobState)
				task.TaskStatus = job.JobStatus
				task.Messages = job.Messages
				break
			}
		}
	}

	return task, nil
}

func (r *LenovoRedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.lenovoBuildRequestBody, r.lenovoExtractTaskMonitorURI)
}

func (r *LenovoRedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.lenovoParseTaskDetails)
}

func (r *LenovoRedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.lenovoBuildRequestBody, r.lenovoExtractTaskMonitorURI)
}

func (r *LenovoRedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.lenovoParseTaskDetails)
}

// CheckBMCPendingComponentUpgrade checks for pending component upgrades (Lenovo: "-Pending" suffix).
func (r *LenovoRedfishBMC) CheckBMCPendingComponentUpgrade(ctx context.Context, componentType ComponentType) (bool, error) {
	if componentType != ComponentTypeBMC && componentType != ComponentTypeBIOS {
		return false, fmt.Errorf("unsupported component type: %q", componentType)
	}
	return checkPendingComponentUpgrade(ctx, r.RedfishBaseBMC, componentType, r.lenovoGetComponentFilters, r.lenovoMatchesComponentFilter, r.lenovoCheckPending)
}

func (r *LenovoRedfishBMC) lenovoGetComponentFilters(componentType ComponentType) []string {
	switch componentType {
	case ComponentTypeBMC:
		return []string{"Firmware:BMC"}
	case ComponentTypeBIOS:
		return []string{"Firmware:UEFI", "BIOS"}
	default:
		return []string{}
	}
}

func (r *LenovoRedfishBMC) lenovoMatchesComponentFilter(fw *schemas.SoftwareInventory, filters []string) bool {
	idUpper := strings.ToUpper(fw.ID)
	for _, filter := range filters {
		if strings.Contains(idUpper, strings.ToUpper(filter)) {
			return true
		}
	}
	return false
}

func (r *LenovoRedfishBMC) lenovoCheckPending(fw *schemas.SoftwareInventory) bool {
	return strings.Contains(strings.ToUpper(fw.ID), "-PENDING")
}

// --- VirtualMedia methods ---

// getManagerForSystem retrieves the manager responsible for the specified system.
// For most single-BMC deployments, there is only one manager.
// For multi-system chassis, this returns the first manager that manages the specified system.
func (r *LenovoRedfishBMC) getManagerForSystem(systemURI string) (*schemas.Manager, error) {
	// Verify the system exists
	_, err := schemas.GetObject[schemas.ComputerSystem](r.client, systemURI)
	if err != nil {
		return nil, fmt.Errorf("failed to get system: %w", err)
	}

	// Get all managers
	managers, err := r.client.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("failed to get managers: %w", err)
	}
	if len(managers) == 0 {
		return nil, fmt.Errorf("no managers found")
	}

	// For single-manager BMCs (most common), return the only manager
	if len(managers) == 1 {
		return managers[0], nil
	}

	// For multi-manager systems, find the manager that manages this system
	// In Redfish, managers expose which systems they manage via ManagerForServers links
	for _, manager := range managers {
		// Check if this manager manages the specified system
		systems, err := manager.ManagerForServers()
		if err != nil {
			continue
		}
		for _, sys := range systems {
			if sys.ODataID == systemURI {
				return manager, nil
			}
		}
	}

	// Fallback: return first manager with a warning logged
	// This handles BMCs where the ManagerForServers link might not be populated
	return managers[0], nil
}

// MountVirtualMedia mounts a virtual media image to the specified slot.
// Lenovo uses Manager endpoints and PATCH requests with EXT-prefixed slot IDs.
func (r *LenovoRedfishBMC) MountVirtualMedia(ctx context.Context, systemURI string, mediaURL string, slotID string) error {
	manager, err := r.getManagerForSystem(systemURI)
	if err != nil {
		return fmt.Errorf("failed to get manager for system: %w", err)
	}

	vmURI := fmt.Sprintf("%s/VirtualMedia/EXT%s", manager.ODataID, slotID)

	payload := map[string]any{
		"Image":          mediaURL,
		"Inserted":       true,
		"WriteProtected": true,
	}

	resp, err := r.client.Service.GetClient().Patch(vmURI, payload)
	if err != nil {
		return fmt.Errorf("failed to mount virtual media: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", cerr)
		}
	}()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to mount virtual media, status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// EjectVirtualMedia ejects virtual media from the specified slot.
// Lenovo uses Manager endpoints and PATCH requests with EXT-prefixed slot IDs.
func (r *LenovoRedfishBMC) EjectVirtualMedia(ctx context.Context, systemURI string, slotID string) error {
	manager, err := r.getManagerForSystem(systemURI)
	if err != nil {
		return fmt.Errorf("failed to get manager for system: %w", err)
	}

	vmURI := fmt.Sprintf("%s/VirtualMedia/EXT%s", manager.ODataID, slotID)

	payload := map[string]any{
		"Image":    "",
		"Inserted": false,
	}

	resp, err := r.client.Service.GetClient().Patch(vmURI, payload)
	if err != nil {
		return fmt.Errorf("failed to eject virtual media: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close response body: %w", cerr)
		}
	}()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to eject virtual media, status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetVirtualMediaStatus retrieves the status of all virtual media slots.
// Lenovo uses Manager endpoints for VirtualMedia.
func (r *LenovoRedfishBMC) GetVirtualMediaStatus(ctx context.Context, systemURI string) ([]*schemas.VirtualMedia, error) {
	manager, err := r.getManagerForSystem(systemURI)
	if err != nil {
		return nil, fmt.Errorf("failed to get manager for system: %w", err)
	}

	return manager.VirtualMedia()
}

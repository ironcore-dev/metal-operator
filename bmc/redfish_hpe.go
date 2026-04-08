// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
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

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// HPERedfishBMC is the HPE-specific implementation of the BMC interface.
type HPERedfishBMC struct {
	*RedfishBaseBMC
}

// --- BMC interface method overrides ---

func (r *HPERedfishBMC) GetBMCAttributeValues(ctx context.Context, bmcUUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	return httpBasedGetBMCSettingAttribute(r.client.GetService().GetClient(), attributes)
}

func (r *HPERedfishBMC) GetBMCPendingAttributeValues(_ context.Context, _ string) (schemas.SettingsAttributes, error) {
	return schemas.SettingsAttributes{}, nil
}

func (r *HPERedfishBMC) SetBMCAttributesImmediately(ctx context.Context, _ string, attributes schemas.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}
	return httpBasedUpdateBMCAttributes(r.client.GetService().GetClient(), attributes, schemas.ImmediateSettingsApplyTime)
}

func (r *HPERedfishBMC) CheckBMCAttributes(_ context.Context, _ string, _ schemas.SettingsAttributes) (bool, error) {
	return false, nil
}

// CreateEventSubscription overrides the base implementation to omit DeliveryRetryPolicy.
// HPE iLO firmware does not support the DeliveryRetryPolicy property in EventDestination
// POST requests and returns: "PropertyNotWritableOrUnknown: DeliveryRetryPolicy is not writable or unknown"
// Even when the EventService advertises retry capabilities, iLO rejects this field.
func (r *HPERedfishBMC) CreateEventSubscription(
	ctx context.Context,
	destination string,
	eventFormatType schemas.EventFormatType,
	retry schemas.DeliveryRetryPolicy,
) (string, error) {
	service := r.client.GetService()
	ev, err := service.EventService()
	if err != nil {
		return "", fmt.Errorf("failed to get event service: %w", err)
	}
	if !ev.ServiceEnabled {
		return "", fmt.Errorf("event service is not enabled")
	}

	payload := &subscriptionPayload{
		Destination:     destination,
		EventFormatType: eventFormatType,
		Protocol:        schemas.RedfishEventDestinationProtocol,
		Context:         "metal-operator",
		// NOTE: DeliveryRetryPolicy is intentionally omitted for HPE iLO compatibility
	}

	client := ev.GetClient()
	// some implementations (like Dell) do not support ResourceTypes and RegistryPrefixes
	if len(ev.ResourceTypes) == 0 {
		payload.EventTypes = []schemas.EventType{}
	}
	// Omit RegistryPrefixes and ResourceTypes to allow all events.
	// Sending empty strings ("") causes 400 errors on BMCs that validate enum values.

	resp, err := client.Post(ev.SubscriptionsLink, payload)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read error response body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to create event subscription status code: %d", resp.StatusCode)
		}

		// Parse Redfish error response
		var redfishError struct {
			Error struct {
				MessageExtendedInfo []struct {
					MessageID string `json:"MessageId"`
					Message   string `json:"Message"`
				} `json:"@Message.ExtendedInfo"`
			} `json:"error"`
		}

		if err := json.Unmarshal(bodyBytes, &redfishError); err == nil {
			// Check if it's a "resource already exists" error
			for _, info := range redfishError.Error.MessageExtendedInfo {
				if strings.Contains(info.MessageID, "ResourceAlreadyExists") ||
					strings.Contains(info.MessageID, "PropertyValueModified") {
					// Handle duplicate subscription - try to find existing one
					if existingLink, findErr := r.findExistingSubscription(destination, eventFormatType); findErr == nil {
						// Successfully found existing subscription
						return existingLink, nil
					}
					// Failed to find existing subscription - fall through to return original error
					// This preserves the detailed Redfish error message for troubleshooting
					break
				}
			}
		}

		// Not a duplicate error - return original error with details
		return "", fmt.Errorf("failed to create event subscription status code: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// return subscription link from returned location
	subscriptionLink := resp.Header.Get("Location")
	if subscriptionLink == "" {
		return "", fmt.Errorf("failed to get subscription link from response header")
	}
	urlParser, err := url.ParseRequestURI(subscriptionLink)
	if err == nil {
		subscriptionLink = urlParser.RequestURI()
	}
	return subscriptionLink, nil
}

// --- Firmware upgrade overrides ---

func (r *HPERedfishBMC) hpeBuildRequestBody(parameters *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Password = parameters.Password
	body.Username = parameters.Username
	body.Targets = parameters.Targets
	body.TransferProtocol = parameters.TransferProtocol
	return body
}

func (r *HPERedfishBMC) hpeExtractTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %w %v", err, rawBody)
	}

	var tResp struct {
		TaskMonitor string
	}
	if err = json.Unmarshal(rawBody, &tResp); err != nil {
		return "", fmt.Errorf("failed to Unmarshal taskMonitor URI %w", err)
	}

	if tResp.TaskMonitor != "" {
		return tResp.TaskMonitor, nil
	}

	if loc := response.Header.Get("Location"); loc != "" {
		return loc, nil
	}

	return "", fmt.Errorf("task monitor URI not found in response body or Location header")
}

func (r *HPERedfishBMC) hpeParseTaskDetails(_ context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
	rawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(rawBody, &task); err != nil {
		return nil, err
	}

	if task.TaskState == "" && task.ODataID == "" {
		type errTask struct {
			Code         string              `json:"code"`
			Message      string              `json:"message"`
			ExtendedInfo []map[string]string `json:"@Message.ExtendedInfo"`
		}
		var tTask struct {
			Error errTask `json:"error"`
		}
		if err = json.Unmarshal(rawBody, &tTask); err != nil {
			return task,
				fmt.Errorf("unable to extract the completed task details %v. \nResponse body %v",
					err, string(rawBody))
		}
		if len(tTask.Error.ExtendedInfo) > 0 {
			if msgID, ok := tTask.Error.ExtendedInfo[0]["MessageId"]; ok && strings.Contains(msgID, "Success") {
				task.TaskState = schemas.CompletedTaskState
				task.PercentComplete = gofish.ToRef(uint(100))
				task.TaskStatus = schemas.OKHealth
				return task, nil
			}
		}
		return task, fmt.Errorf("unable to find the state of the Task %v", string(rawBody))
	}

	return task, nil
}

func (r *HPERedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.hpeBuildRequestBody, r.hpeExtractTaskMonitorURI)
}

func (r *HPERedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.hpeParseTaskDetails)
}

func (r *HPERedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.hpeBuildRequestBody, r.hpeExtractTaskMonitorURI)
}

func (r *HPERedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.hpeParseTaskDetails)
}

// CheckBMCPendingComponentUpgrade checks for staged component upgrades (HPE: Staged=true).
// NOTE: HPE firmware entries use numeric IDs, so matching is done via fw.Name.
func (r *HPERedfishBMC) CheckBMCPendingComponentUpgrade(ctx context.Context, componentType ComponentType) (bool, error) {
	if componentType != ComponentTypeBMC && componentType != ComponentTypeBIOS {
		return false, fmt.Errorf("unsupported component type: %q", componentType)
	}
	return checkPendingComponentUpgrade(ctx, r.RedfishBaseBMC, componentType, r.hpeGetComponentFilters, r.hpeMatchesComponentFilter, r.hpeCheckPending)
}

func (r *HPERedfishBMC) hpeGetComponentFilters(componentType ComponentType) []string {
	switch componentType {
	case ComponentTypeBMC:
		return []string{"iLO"}
	case ComponentTypeBIOS:
		return []string{"System ROM"}
	default:
		return []string{}
	}
}

func (r *HPERedfishBMC) hpeMatchesComponentFilter(fw *schemas.SoftwareInventory, filters []string) bool {
	nameUpper := strings.ToUpper(fw.Name)
	for _, filter := range filters {
		if strings.Contains(nameUpper, strings.ToUpper(filter)) {
			return true
		}
	}
	return false
}

func (r *HPERedfishBMC) hpeCheckPending(fw *schemas.SoftwareInventory) bool {
	return fw.Staged
}

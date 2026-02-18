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

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// HPERedfishBMC is the HPE-specific implementation of the BMC interface.
type HPERedfishBMC struct {
	*RedfishBaseBMC
}

// --- BMC interface method overrides ---

func (r *HPERedfishBMC) GetBMCAttributeValues(ctx context.Context, bmcUUID string, attributes map[string]string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	return httpBasedGetBMCSettingAttribute(r.client.GetService().GetClient(), attributes)
}

func (r *HPERedfishBMC) GetBMCPendingAttributeValues(_ context.Context, _ string) (redfish.SettingsAttributes, error) {
	return redfish.SettingsAttributes{}, nil
}

func (r *HPERedfishBMC) SetBMCAttributesImmediately(ctx context.Context, _ string, attributes redfish.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}
	return httpBasedUpdateBMCAttributes(r.client.GetService().GetClient(), attributes, common.ImmediateApplyTime)
}

func (r *HPERedfishBMC) CheckBMCAttributes(_ context.Context, _ string, _ redfish.SettingsAttributes) (bool, error) {
	return false, nil
}

// --- Firmware upgrade overrides ---

func (r *HPERedfishBMC) hpeBuildRequestBody(parameters *redfish.SimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Passord = parameters.Passord
	body.Username = parameters.Username
	body.Targets = parameters.Targets
	body.TransferProtocol = parameters.TransferProtocol
	return body
}

func (r *HPERedfishBMC) hpeExtractTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	var tResp struct {
		TaskMonitor string
	}
	if err = json.Unmarshal(rawBody, &tResp); err != nil {
		return tResp.TaskMonitor, fmt.Errorf("failed to Unmarshal taskMonitor URI %v", err)
	}

	return tResp.TaskMonitor, nil
}

func (r *HPERedfishBMC) hpeParseTaskDetails(_ context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
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
		if strings.Contains(tTask.Error.ExtendedInfo[0]["MessageId"], "Success") {
			task.TaskState = redfish.CompletedTaskState
			task.PercentComplete = 100
			task.TaskStatus = common.OKHealth
			return task, nil
		}
		return task, fmt.Errorf("unable to find the state of the Task %v", string(rawBody))
	}

	return task, nil
}

func (r *HPERedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.hpeBuildRequestBody, r.hpeExtractTaskMonitorURI)
}

func (r *HPERedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*redfish.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.hpeParseTaskDetails)
}

func (r *HPERedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.hpeBuildRequestBody, r.hpeExtractTaskMonitorURI)
}

func (r *HPERedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*redfish.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.hpeParseTaskDetails)
}

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
	"k8s.io/utils/ptr"
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
				task.PercentComplete = ptr.To(uint(100))
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

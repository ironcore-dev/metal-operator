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
		return tResp.TaskMonitor, fmt.Errorf("failed to Unmarshal taskMonitor URI %v", err)
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

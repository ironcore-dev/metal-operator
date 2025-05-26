// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

type Lenovo struct {
	Service *gofish.Service
}

func (r *Lenovo) GetUpdateRequestBody(
	parameters *redfish.SimpleUpdateParameters,
) *SimpleUpdateRequestBody {
	RequestBody := &SimpleUpdateRequestBody{}
	RequestBody.ForceUpdate = parameters.ForceUpdate
	RequestBody.ImageURI = parameters.ImageURI
	RequestBody.Passord = parameters.Passord
	RequestBody.Username = parameters.Username
	RequestBody.Targets = parameters.Targets
	RequestBody.TransferProtocol = parameters.TransferProtocol
	return RequestBody
}

func (r *Lenovo) GetUpdateTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	// extract tasks ID to monitor it
	var tResp struct {
		TaskMonitor string `json:"@odata.id,omitempty"`
	}
	err = json.Unmarshal(rawBody, &tResp)
	if err != nil {
		return tResp.TaskMonitor, fmt.Errorf("failed to Unmarshal taskMonitor URI %v", err)
	}

	return tResp.TaskMonitor, nil
}

func (r *Lenovo) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
	respTaskRawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respTaskRawBody, &task)
	if err != nil {
		return nil, err
	}

	if len(task.Messages) > 0 && task.TaskState == redfish.CompletedTaskState && task.TaskStatus == common.OKHealth {
		for _, msg := range task.Messages {
			if strings.Contains(msg.MessageID, "OperationTransitionedToJob") &&
				len(msg.MessageArgs) > 0 {

				respJob, err := r.Service.GetClient().Get(msg.MessageArgs[0])
				if err != nil {
					return nil, err
				}
				defer respJob.Body.Close() // nolint: errcheck

				if respJob.StatusCode != http.StatusAccepted && respJob.StatusCode != http.StatusOK {
					respJobRawBody, err := io.ReadAll(respJob.Body)
					if err != nil {
						return nil,
							fmt.Errorf(
								"failed to get the upgrade Task details. and read the response body %v, statusCode %v",
								err,
								respJob.StatusCode,
							)
					}
					return nil,
						fmt.Errorf("failed to get the upgrade Task details. %v, statusCode %v",
							string(respJobRawBody),
							respJob.StatusCode)
				}

				respJobRawBody, err := io.ReadAll(respJob.Body)
				if err != nil {
					return nil,
						fmt.Errorf(
							"failed to get the upgrade Task details. and read the response body %v, statusCode %v",
							err,
							respJob.StatusCode,
						)
				}

				job := &redfish.Job{}
				err = json.Unmarshal(respJobRawBody, &job)
				if err != nil {
					return nil, err
				}
				task = &redfish.Task{}
				task.ID = job.ID
				task.ODataID = job.ODataID
				task.Description = job.Description
				task.StartTime = job.StartTime
				task.EndTime = job.EndTime
				task.PercentComplete = job.PercentComplete
				task.TaskState = redfish.TaskState(job.JobState)
				task.TaskStatus = job.JobStatus
				task.Messages = job.Messages
				break
			}
		}
	}

	return task, nil
}

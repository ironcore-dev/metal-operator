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

type HPE struct {
	Service *gofish.Service
}

func (r *HPE) GetUpdateRequestBody(
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

func (r *HPE) GetUpdateTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	// extract tasks ID to monitor it
	var tResp struct {
		TaskMonitor string
	}
	err = json.Unmarshal(rawBody, &tResp)
	if err != nil {
		return tResp.TaskMonitor, fmt.Errorf("failed to Unmarshal taskMonitor URI %v", err)
	}

	return tResp.TaskMonitor, nil
}

func (r *HPE) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
	respTaskRawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respTaskRawBody, &task)
	if err != nil {
		return nil, err
	}

	if task.TaskState == "" && task.ODataID == "" {
		// hpe gives error after completion of data, exptract it to verify if success
		type errTask struct {
			Code         string              `json:"code"`
			Message      string              `json:"message"`
			ExtendedInfo []map[string]string `json:"@Message.ExtendedInfo"`
		}
		var tTask struct {
			Error errTask `json:"error"`
		}
		err = json.Unmarshal(respTaskRawBody, &tTask)
		if err != nil {
			return task,
				fmt.Errorf(
					"unable to extract the completed task details %v. \nResponse body %v",
					err,
					string(respTaskRawBody))
		}
		if strings.Contains(tTask.Error.ExtendedInfo[0]["MessageId"], "Success") {
			task.TaskState = redfish.CompletedTaskState
			task.PercentComplete = 100
			task.TaskStatus = common.OKHealth
			return task, nil
		}
		return task, fmt.Errorf("unable to find the state of the Task %v", string(respTaskRawBody))
	}

	return task, nil
}

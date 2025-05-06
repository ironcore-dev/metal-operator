// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

type Dell struct {
	Service *gofish.Service
}

func (r *Dell) GetUpdateBIOSRequestBody(
	parameters *redfish.SimpleUpdateParameters,
) *SimpleUpdateRequestBody {
	RequestBody := &SimpleUpdateRequestBody{}
	RequestBody.RedfishOperationApplyTime = redfish.ImmediateOperationApplyTime
	RequestBody.ForceUpdate = parameters.ForceUpdate
	RequestBody.ImageURI = parameters.ImageURI
	RequestBody.Passord = parameters.Passord
	RequestBody.Username = parameters.Username
	RequestBody.Targets = parameters.Targets
	RequestBody.TransferProtocol = parameters.TransferProtocol

	return RequestBody
}

func (r *Dell) GetUpdateBIOSTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %v %v", err, rawBody)
	}

	if taskMonitor, ok := response.Header["Location"]; ok && len(rawBody) == 0 {
		return taskMonitor[0], nil
	}

	return "", fmt.Errorf("unexpected response body %v %v", err, rawBody)
}

func (r *Dell) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
	respTaskRawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respTaskRawBody, &task)
	if err != nil {
		return nil, err
	}

	return task, nil
}

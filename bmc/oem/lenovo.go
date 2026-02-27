// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Lenovo struct {
	Service *gofish.Service
}

func (r *Lenovo) GetUpdateRequestBody(
	parameters *schemas.UpdateServiceSimpleUpdateParameters,
) *SimpleUpdateRequestBody {
	RequestBody := &SimpleUpdateRequestBody{}
	RequestBody.ForceUpdate = parameters.ForceUpdate
	RequestBody.ImageURI = parameters.ImageURI
	RequestBody.Password = parameters.Password
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

func (r *Lenovo) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
	respTaskRawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respTaskRawBody, &task)
	if err != nil {
		return nil, err
	}

	if len(task.Messages) > 0 && task.TaskState == schemas.CompletedTaskState && task.TaskStatus == schemas.OKHealth {
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

				job := &schemas.Job{}
				err = json.Unmarshal(respJobRawBody, &job)
				if err != nil {
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

type LenovoXCCManager struct {
	BMC     *schemas.Manager
	Service *gofish.Service
}

func (l *LenovoXCCManager) GetObjFromUri(
	ctx context.Context,
	uri string,
	respObj any,
) (string, error) {
	resp, err := l.BMC.GetClient().Get(uri)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(rawBody, &respObj)
	if err != nil {
		return "", err
	}
	return resp.Header.Get("ETag"), nil
}

func (l *LenovoXCCManager) GetOEMBMCSettingAttribute(
	ctx context.Context,
	attributes map[string]string,
) (schemas.SettingsAttributes, error) {
	log := ctrl.LoggerFrom(ctx)
	c := l.Service.GetClient()
	if c == nil {
		return nil, fmt.Errorf("failed to get client from gofish service")
	}
	result := schemas.SettingsAttributes{}
	errs := []error{}
	for key, data := range attributes {
		parts := strings.Fields(key)
		if len(parts) != 2 {
			errs = append(errs, fmt.Errorf("invalid attribute format: %s\n expected '<HTTP METHOD> <URI>'", key))
		}
		resp, err := c.Get(parts[1])
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to GET attribute %s to URL %s: %v", key, parts[1], err))
			continue
		}
		okCodes := []int{http.StatusOK, http.StatusNoContent}
		if !slices.Contains(okCodes, resp.StatusCode) {
			errs = append(errs, fmt.Errorf("failed to GET attribute %s: received status code %d", parts[1], resp.StatusCode))
			continue
		}
		respRawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read response body for url %s: %v\n %v", parts[1], err, resp))
			continue
		}
		defer resp.Body.Close() // nolint: errcheck
		var respData map[string]any
		err = json.Unmarshal(respRawBody, &respData)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to unmarshal response body for GET url %s: %v\nbody: %v", parts[1], err, string(respRawBody)))
			continue
		}
		var dataMap map[string]any
		err = json.Unmarshal([]byte(data), &dataMap)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to unmarshal spec data for url %s: %v\nbody: %v", parts[1], err, data))
			continue
		}
		// compare the returned JSON with the expected JSON
		subJson := IsSubMap(respData, dataMap)
		if subJson {
			// matched
			result[key] = data
		} else {
			// not matched
			result[key] = string(respRawBody)
		}
	}
	log.V(1).Info("Fetched data from BMC Settings ", "Result", result)
	return result, errors.Join(errs...)
}

func (l *LenovoXCCManager) CheckBMCAttributes(
	ctx context.Context,
	attributes schemas.SettingsAttributes,
) (bool, error) {
	// We do not have any option to check attributes for HPE iLO
	return false, nil
}

func (l *LenovoXCCManager) UpdateBMCAttributesApplyAt(
	ctx context.Context,
	attrs schemas.SettingsAttributes,
	applyTime schemas.SettingsApplyTime,
) error {
	// apply the attributes through PATCH or POST call
	// we can not paralaize the calls to HPE here due to limitation from server
	if applyTime != schemas.ImmediateSettingsApplyTime {
		return fmt.Errorf("does not support scheduled apply time for BMC attributes")
	}
	c := l.Service.GetClient()
	if c == nil {
		return fmt.Errorf("failed to get client from gofish service")
	}
	okCodes := []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusCreated}
	errs := []error{}
	for attr, value := range attrs {
		parts := strings.Fields(attr)
		if len(parts) != 2 {
			errs = append(errs, fmt.Errorf("invalid attribute format: %s\n expected '<HTTP METHOD> <URI>'", attr))
		}
		url := parts[1]
		valueMap := map[string]any{}
		err := json.Unmarshal([]byte(value.(string)), &valueMap)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to unmarshal spec data for url %s: %v\nbody: %v", parts[1], err, value))
			continue
		}
		switch parts[0] {
		case http.MethodPost:
			resp, err := c.Post(url, valueMap)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to POST attribute %s to URL %s: %v", attr, url, err))
				continue
			}
			if !slices.Contains(okCodes, resp.StatusCode) {
				errs = append(errs, fmt.Errorf("failed to POST attribute %s: received status code %d", attr, resp.StatusCode))
				continue
			}
		case http.MethodPatch:
			resp, err := c.Patch(url, valueMap)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to PATCH attribute %s to URL %s: %v", attr, url, err))
				continue
			}
			if !slices.Contains(okCodes, resp.StatusCode) {
				errs = append(errs, fmt.Errorf("failed to POST attribute %s: received status code %d", attr, resp.StatusCode))
				continue
			}
		default:
			errs = append(errs, fmt.Errorf("unsupported HTTP method %s for attribute %s", parts[0], attr))
		}
	}
	return errors.Join(errs...)
}

func (l *LenovoXCCManager) GetBMCPendingAttributeValues(ctx context.Context) (schemas.SettingsAttributes, error) {
	// We do not have any option to get pending attributes for Dell iDRAC
	return schemas.SettingsAttributes{}, nil
}

func (r *Lenovo) MountVirtualMedia(ctx context.Context, systemURI string, mediaURL string, slotID string) error {
	managers, err := r.Service.Managers()
	if err != nil {
		return fmt.Errorf("failed to get managers: %w", err)
	}
	if len(managers) == 0 {
		return fmt.Errorf("no managers found")
	}

	manager := managers[0]
	vmURI := fmt.Sprintf("%s/VirtualMedia/EXT%s", manager.ODataID, slotID)

	payload := map[string]any{
		"Image":          mediaURL,
		"Inserted":       true,
		"WriteProtected": true,
	}

	resp, err := r.Service.GetClient().Patch(vmURI, payload)
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

func (r *Lenovo) EjectVirtualMedia(ctx context.Context, systemURI string, slotID string) error {
	managers, err := r.Service.Managers()
	if err != nil {
		return fmt.Errorf("failed to get managers: %w", err)
	}
	if len(managers) == 0 {
		return fmt.Errorf("no managers found")
	}

	manager := managers[0]
	vmURI := fmt.Sprintf("%s/VirtualMedia/EXT%s", manager.ODataID, slotID)

	payload := map[string]any{
		"Image":    "",
		"Inserted": false,
	}

	resp, err := r.Service.GetClient().Patch(vmURI, payload)
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

func (r *Lenovo) GetVirtualMediaStatus(ctx context.Context, systemURI string) ([]*schemas.VirtualMedia, error) {
	managers, err := r.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("failed to get managers: %w", err)
	}
	if len(managers) == 0 {
		return nil, fmt.Errorf("no managers found")
	}

	manager := managers[0]
	return manager.VirtualMedia()
}

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SimpleUpdateRequestBody extends SimpleUpdateParameters with an OEM apply-time field.
type SimpleUpdateRequestBody struct {
	redfish.SimpleUpdateParameters
	RedfishOperationApplyTime redfish.OperationApplyTime `json:"@Redfish.OperationApplyTime,omitempty"`
}

// upgradeRequestBodyFn builds a vendor-specific request body from update parameters.
type upgradeRequestBodyFn func(parameters *redfish.SimpleUpdateParameters) *SimpleUpdateRequestBody

// upgradeTaskMonitorURIFn extracts the task monitor URI from the update response.
type upgradeTaskMonitorURIFn func(response *http.Response) (string, error)

// taskMonitorDetailsFn parses vendor-specific task monitor details.
type taskMonitorDetailsFn func(ctx context.Context, response *http.Response) (*redfish.Task, error)

// upgradeVersion is the common firmware upgrade flow shared by all vendors.
// Vendor-specific parts are injected via callbacks.
func upgradeVersion(ctx context.Context, base *RedfishBaseBMC, params *redfish.SimpleUpdateParameters, requestBodyFn upgradeRequestBodyFn, taskMonitorURIFn upgradeTaskMonitorURIFn) (string, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	service := base.client.GetService()

	updateService, err := service.UpdateService()
	if err != nil {
		return "", false, err
	}

	type tActions struct {
		SimpleUpdate struct {
			AllowableValues []string `json:"TransferProtocol@Redfish.AllowableValues"`
			Target          string
		} `json:"#UpdateService.SimpleUpdate"`
		StartUpdate common.ActionTarget `json:"#UpdateService.StartUpdate"`
	}

	var tUS struct {
		Actions tActions
	}

	if err = json.Unmarshal(updateService.RawData, &tUS); err != nil {
		return "", false, err
	}

	requestBody := requestBodyFn(params)

	resp, err := updateService.PostWithResponse(tUS.Actions.SimpleUpdate.Target, &requestBody)
	if err != nil {
		return "", false, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Error(err, "failed to close response body")
		}
	}(resp.Body)

	// any error post this point is fatal, as we can not issue multiple upgrade requests.
	// expectation is to move to failed state, and manually check the status before retrying
	log.V(1).Info("Update has been issued", "ResponseCode", resp.StatusCode)
	if resp.StatusCode != http.StatusAccepted {
		rawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", true,
				fmt.Errorf("failed to accept the upgrade request. and read the response body %v, statusCode %v",
					err, resp.StatusCode)
		}
		return "", true,
			fmt.Errorf("failed to accept the upgrade request %v, statusCode %v",
				string(rawBody), resp.StatusCode)
	}

	taskMonitorURI, err := taskMonitorURIFn(resp)
	if err != nil {
		return "", true, fmt.Errorf("failed to read task monitor URI. %v", err)
	}

	log.V(1).Info("update has been accepted.", "Response", taskMonitorURI)
	return taskMonitorURI, false, nil
}

// getUpgradeTask is the common task polling flow shared by all vendors.
func getUpgradeTask(ctx context.Context, base *RedfishBaseBMC, taskURI string, parseTaskDetails taskMonitorDetailsFn) (*redfish.Task, error) {
	log := ctrl.LoggerFrom(ctx)

	respTask, err := base.client.GetService().GetClient().Get(taskURI)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Error(err, "failed to close response body")
		}
	}(respTask.Body)

	if respTask.StatusCode != http.StatusAccepted && respTask.StatusCode != http.StatusOK {
		respTaskRawBody, err := io.ReadAll(respTask.Body)
		if err != nil {
			return nil,
				fmt.Errorf("failed to get the upgrade Task details. and read the response body %v, statusCode %v",
					err, respTask.StatusCode)
		}
		return nil,
			fmt.Errorf("failed to get the upgrade Task details. %v, statusCode %v",
				string(respTaskRawBody), respTask.StatusCode)
	}

	return parseTaskDetails(ctx, respTask)
}

// httpBasedGetBMCSettingAttribute retrieves BMC settings via HTTP GET.
// Shared by HPE and Lenovo which use "GET <URI>" format attributes.
func httpBasedGetBMCSettingAttribute(c common.Client, attributes map[string]string) (redfish.SettingsAttributes, error) {
	if c == nil {
		return nil, fmt.Errorf("failed to get client from gofish service")
	}
	result := redfish.SettingsAttributes{}
	var errs []error
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
		if isSubMap(respData, dataMap) {
			result[key] = data
		} else {
			result[key] = string(respRawBody)
		}
	}
	return result, errors.Join(errs...)
}

// httpBasedUpdateBMCAttributes applies BMC attributes via HTTP POST/PATCH.
// Shared by HPE and Lenovo which use "POST <URI>" or "PATCH <URI>" format attributes.
func httpBasedUpdateBMCAttributes(c common.Client, attrs redfish.SettingsAttributes, applyTime common.ApplyTime) error {
	if applyTime != common.ImmediateApplyTime {
		return fmt.Errorf("does not support scheduled apply time for BMC attributes")
	}
	if c == nil {
		return fmt.Errorf("failed to get client from gofish service")
	}
	okCodes := []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusCreated}
	var errs []error
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
				errs = append(errs, fmt.Errorf("failed to PATCH attribute %s: received status code %d", attr, resp.StatusCode))
				continue
			}
		default:
			errs = append(errs, fmt.Errorf("unsupported HTTP method %s for attribute %s", parts[0], attr))
		}
	}
	return errors.Join(errs...)
}

// isSubMap checks if sub is a subset of main (recursively for nested maps).
func isSubMap(main, sub map[string]any) bool {
	for k, vSub := range sub {
		vMain, ok := main[k]
		if !ok {
			return false
		}
		switch vSubTyped := vSub.(type) {
		case map[string]any:
			vMainTyped, ok := vMain.(map[string]any)
			if !ok || !isSubMap(vMainTyped, vSubTyped) {
				return false
			}
		default:
			if !reflect.DeepEqual(vMain, vSub) {
				return false
			}
		}
	}
	return true
}

// checkAttributes validates attributes against a filtered registry, returning
// whether a reset is required and any validation errors.
func checkAttributes(attrs redfish.SettingsAttributes, filtered map[string]redfish.Attribute) (reset bool, err error) {
	reset = false
	var errs []error
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			errs = append(errs, fmt.Errorf("attribute %s not found or immutable/hidden", name))
			continue
		}
		if entryAttribute.ResetRequired {
			reset = true
		}
		switch entryAttribute.Type {
		case redfish.IntegerAttributeType:
			if _, ok := value.(int); !ok {
				errs = append(errs,
					fmt.Errorf("attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name, value, entryAttribute.Type, entryAttribute))
			}
		case redfish.StringAttributeType:
			if _, ok := value.(string); !ok {
				errs = append(errs,
					fmt.Errorf("attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name, value, entryAttribute.Type, entryAttribute))
			}
		case redfish.EnumerationAttributeType:
			if _, ok := value.(string); !ok {
				errs = append(errs,
					fmt.Errorf("attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name, value, entryAttribute.Type, entryAttribute))
				break
			}
			var validEnum bool
			for _, attrValue := range entryAttribute.Value {
				if attrValue.ValueName == value.(string) {
					validEnum = true
					break
				}
			}
			if !validEnum {
				errs = append(errs, fmt.Errorf("attribute %s value is unknown. needed %v", name, entryAttribute.Value))
			}
		default:
			errs = append(errs,
				fmt.Errorf("attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
					name, value, entryAttribute.Type, entryAttribute))
		}
	}
	return reset, errors.Join(errs...)
}

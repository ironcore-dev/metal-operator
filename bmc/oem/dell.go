// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	helpers "github.com/ironcore-dev/metal-operator/bmc/common"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type Dell struct {
	Service *gofish.Service
}

func (r *Dell) GetUpdateRequestBody(
	parameters *schemas.UpdateServiceSimpleUpdateParameters,
) *SimpleUpdateRequestBody {
	RequestBody := &SimpleUpdateRequestBody{}
	RequestBody.RedfishOperationApplyTime = schemas.ImmediateOperationApplyTime
	RequestBody.ForceUpdate = parameters.ForceUpdate
	RequestBody.ImageURI = parameters.ImageURI
	RequestBody.Password = parameters.Password
	RequestBody.Username = parameters.Username
	RequestBody.Targets = parameters.Targets
	RequestBody.TransferProtocol = parameters.TransferProtocol

	return RequestBody
}

func (r *Dell) GetUpdateTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body: %w", err)
	}

	// Dell iDRAC returns task monitor URI in Location header for async operations
	if taskMonitor, ok := response.Header["Location"]; ok && len(taskMonitor) > 0 {
		return taskMonitor[0], nil
	}

	// Some Dell iDRAC versions return task info in response body
	var taskResp struct {
		TaskMonitor string `json:"@odata.id,omitempty"`
		Task        struct {
			OdataID string `json:"@odata.id,omitempty"`
		} `json:"Task,omitempty"`
	}

	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &taskResp); err != nil {
			return "", fmt.Errorf("failed to unmarshal task monitor response: %w", err)
		}

		if taskResp.TaskMonitor != "" {
			return taskResp.TaskMonitor, nil
		}
		if taskResp.Task.OdataID != "" {
			return taskResp.Task.OdataID, nil
		}
	}

	return "", fmt.Errorf("unable to extract task monitor URI from Dell iDRAC response")
}

func (r *Dell) GetTaskMonitorDetails(ctx context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
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

type DellIdracManager struct {
	BMC     *schemas.Manager
	Service *gofish.Service
}

type DellAttributes struct {
	Id         string
	Attributes schemas.SettingsAttributes
	Settings   schemas.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type DellManagerLinksOEM struct {
	DellLinkAttributes  schemas.Links `json:"DellAttributes"`
	DellAttributesCount int           `json:"DellAttributes@odata.count"`
}

func (d *DellIdracManager) GetObjFromUri(
	ctx context.Context,
	uri string,
	respObj any,
) (string, error) {
	resp, err := d.BMC.GetClient().Get(uri)
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

func (d *DellIdracManager) getCurrentBMCSettingAttribute(ctx context.Context) ([]DellAttributes, error) {
	type temp struct {
		Links struct {
			Oem struct {
				Dell DellManagerLinksOEM
			}
		}
	}

	tempData := &temp{}
	err := json.Unmarshal(d.BMC.RawData, tempData)
	if err != nil {
		return nil, err
	}

	// get all current attributes values for dell manager
	BMCDellAttributes := []DellAttributes{}
	var errs []error
	for _, data := range tempData.Links.Oem.Dell.DellLinkAttributes {
		BMCDellAttribute := &DellAttributes{}
		eTag, err := d.GetObjFromUri(ctx, data.String(), BMCDellAttribute)
		if err != nil {
			errs = append(errs, err)
		}
		BMCDellAttribute.Etag = eTag
		BMCDellAttributes = append(BMCDellAttributes, *BMCDellAttribute)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return BMCDellAttributes, nil

}

func (d *DellIdracManager) getFilteredBMCRegistryAttributes(
	ctx context.Context,
	readOnly bool,
	immutable bool,
) (
	filtered map[string]schemas.Attributes,
	err error,
) {
	// from the registriesAttribure, get the attributes which can be changed.
	registries, err := d.Service.Registries()
	if err != nil {
		return nil, err
	}
	bmcRegistryAttribute := &schemas.AttributeRegistry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "ManagerAttributeRegistry") {
			_, err = d.GetObjFromUri(ctx, registry.Location[0].URI, bmcRegistryAttribute)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	// filter out immutable, readonly and hidden attributes
	filteredAttr := make(map[string]schemas.Attributes)
	for _, entry := range bmcRegistryAttribute.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filteredAttr[entry.AttributeName] = entry
		}
	}

	return filteredAttr, nil
}

func (d *DellIdracManager) GetOEMBMCSettingAttribute(
	ctx context.Context,
	attributes map[string]string,
) (schemas.SettingsAttributes, error) {

	BMCDellAttributes, err := d.getCurrentBMCSettingAttribute(ctx)
	if err != nil {
		return nil, err
	}

	// merge al the current attributes to single map, to help fetch it later
	var mergedBMCAttributes = make(schemas.SettingsAttributes)
	for _, BMCattributeValue := range BMCDellAttributes {
		for k, v := range BMCattributeValue.Attributes {
			if _, ok := mergedBMCAttributes[k]; !ok {
				mergedBMCAttributes[k] = v
			} else {
				return nil,
					fmt.Errorf("duplicate attributes in BMC settings are not supported duplicate key %v. in attribute %v",
						k,
						BMCDellAttributes,
					)
			}
		}
	}

	filteredAttr, err := d.getFilteredBMCRegistryAttributes(ctx, false, false)
	if err != nil {
		return nil, err
	}

	if len(filteredAttr) == 0 {
		return nil, fmt.Errorf("'ManagerAttributeRegistry' not found")
	}

	// from the given attributes to change, find the ones which can be changed and get current value for them
	result := make(schemas.SettingsAttributes, len(attributes))
	var errs []error
	for name := range attributes {
		var entry schemas.Attributes
		var ok bool

		// First check registry attributes, then fall back to Dell common attributes
		if entry, ok = filteredAttr[name]; !ok {
			if entry, ok = dellCommonBMCAttributes[name]; !ok {
				// possible error in settings key
				errs = append(errs, fmt.Errorf("setting key '%v' not found in possible settings", name))
				continue
			}
		}

		// enumerations current setting contains display name.
		// need to be checked with the actual value rather than the display value
		// as the settings provided will have actual values.
		// replace display values with actual values
		if strings.ToLower(string(entry.Type)) == string(schemas.EnumerationAttributeType) {
			for _, attrValue := range entry.Value {
				if attrValue.ValueDisplayName == mergedBMCAttributes[name] {
					result[name] = attrValue.ValueName
					break
				}
			}
			if _, ok := result[name]; !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"current setting '%v' for key '%v' not found in possible values for it (%v)",
						mergedBMCAttributes[name],
						name,
						entry.Value,
					))
			}
		} else {
			result[name] = mergedBMCAttributes[name]
		}
	}
	if len(errs) > 0 {
		return result, fmt.Errorf(
			"some errors found in the settings '%v'.\nPossible settings %v",
			errs,
			maps.Keys(filteredAttr),
		)
	}

	return result, nil
}

func (d *DellIdracManager) UpdateBMCAttributesApplyAt(
	ctx context.Context,
	attrs schemas.SettingsAttributes,
	applyTime schemas.SettingsApplyTime,
) error {

	BMCattributeValues, err := d.getCurrentBMCSettingAttribute(ctx)
	if err != nil {
		return err
	}

	payloads := make(map[string]schemas.SettingsAttributes, len(BMCattributeValues))
	for key, value := range attrs {
		for _, eachAttr := range BMCattributeValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				if data, ok := payloads[eachAttr.Settings.SettingsObject]; ok {
					data[key] = value
				} else {
					payloads[eachAttr.Settings.SettingsObject] = make(schemas.SettingsAttributes)
					payloads[eachAttr.Settings.SettingsObject][key] = value
				}
				// keys cant be duplicate. Hence, break once its already found in one of idrac settings sub types
				break
			}
		}
	}

	// If there are any allowed updates, try to send updates to the system and
	// return the result.
	if len(payloads) > 0 {
		var errs []error
		// for each sub type, apply the settings
		for settingPath, payload := range payloads {
			// fetch the etag required for settingPath
			etag, err := func() (string, error) {
				resp, err := d.BMC.GetClient().Get(settingPath)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close() // nolint: errcheck
				return resp.Header.Get("ETag"), nil
			}()

			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get Etag for %v. error %v", settingPath, err))
				continue
			}

			data := map[string]any{"Attributes": payload}
			if applyTime != "" {
				data["@Redfish.SettingsApplyTime"] = map[string]string{"ApplyTime": string(applyTime)}
			}
			var header = make(map[string]string)
			if etag != "" {
				header["If-Match"] = etag
			}

			err = func() error {
				resp, err := d.BMC.GetClient().PatchWithHeaders(settingPath, data, header)
				if err != nil {
					return err
				}
				defer resp.Body.Close() // nolint: errcheck
				return nil
			}()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to patch settings at %v. error %v", settingPath, err))
				continue
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("some settings failed to apply %v", errs)
		}
	}
	return nil
}

func (d *DellIdracManager) GetBMCPendingAttributeValues(ctx context.Context) (schemas.SettingsAttributes, error) {

	BMCattributeValues, err := d.getCurrentBMCSettingAttribute(ctx)
	if err != nil {
		return nil, err
	}

	var mergedPendingBMCAttributes = make(schemas.SettingsAttributes)
	var tBMCSetting struct {
		Attributes schemas.SettingsAttributes `json:"Attributes"`
	}

	for _, BMCattributeValue := range BMCattributeValues {
		_, err := d.GetObjFromUri(ctx, BMCattributeValue.Settings.SettingsObject, &tBMCSetting)
		if err != nil {
			return nil, err
		}
		for k, v := range tBMCSetting.Attributes {
			if _, ok := mergedPendingBMCAttributes[k]; !ok {
				mergedPendingBMCAttributes[k] = v
			} else {
				return nil, fmt.Errorf("duplicate pending attributes in Idrac settings are not supported %v", k)
			}
		}
	}

	return mergedPendingBMCAttributes, nil
}

// dellCommonBMCAttributes defines commonly configured Dell iDRAC attributes
// that may not be in the standard registry but are supported by Dell iDRAC
var dellCommonBMCAttributes = map[string]schemas.Attributes{
	"SysLog.1.SysLogEnable": {
		Type:          schemas.BooleanAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
	"SysLog.1.SysLogServer1": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: false,
	},
	"SysLog.1.SysLogServer2": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: false,
	},
	"NTPConfigGroup.1.NTPEnable": {
		Type:          schemas.BooleanAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP1": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP2": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
	"EmailAlert.1.Enable": {
		Type:          schemas.BooleanAttributeType,
		ReadOnly:      false,
		ResetRequired: false,
	},
	"EmailAlert.1.Address": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: false,
	},
	"SNMP.1.AgentEnable": {
		Type:          schemas.BooleanAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
	"SNMP.1.AgentCommunity": {
		Type:          schemas.StringAttributeType,
		ReadOnly:      false,
		ResetRequired: true,
	},
}

func (d *DellIdracManager) CheckBMCAttributes(
	ctx context.Context,
	attributes schemas.SettingsAttributes,
) (bool, error) {
	filteredAttr, err := d.getFilteredBMCRegistryAttributes(ctx, false, false)
	if err != nil {
		return false, err
	}

	// Merge Dell-specific common attributes with registry attributes
	for name, attr := range dellCommonBMCAttributes {
		if _, exists := filteredAttr[name]; !exists {
			filteredAttr[name] = attr
		}
	}

	if len(filteredAttr) == 0 {
		return false, nil
	}
	return helpers.CheckAttributes(attributes, filteredAttr)
}

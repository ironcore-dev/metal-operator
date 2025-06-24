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
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

type Dell struct {
	Service *gofish.Service
}

func (r *Dell) GetUpdateRequestBody(
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

func (r *Dell) GetUpdateTaskMonitorURI(response *http.Response) (string, error) {
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

type DellIdracManager struct {
	BMC     *redfish.Manager
	Service *gofish.Service
}

type DellAttributes struct {
	Id         string
	Attributes redfish.SettingsAttributes
	Settings   common.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type DellManagerLinksOEM struct {
	DellLinkAttributes  common.Links `json:"DellAttributes"`
	DellAttributesCount int          `json:"DellAttributes@odata.count"`
}

func (d *DellIdracManager) GetObjFromUri(
	uri string,
	respObj any,
) ([]string, error) {
	resp, err := d.BMC.GetClient().Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(rawBody, &respObj)
	if err != nil {
		return nil, err
	}
	return resp.Header["Etag"], nil
}

func (d *DellIdracManager) getCurrentBMCSettingAttribute() ([]DellAttributes, error) {

	type temp struct {
		DellOEMData DellManagerLinksOEM `json:"Dell"`
	}

	tempData := &temp{}
	err := json.Unmarshal(d.BMC.OemLinks, tempData)
	if err != nil {
		return nil, err
	}

	// get all current attributes values for dell manager
	BMCDellAttributes := []DellAttributes{}
	var errs []error
	for _, data := range tempData.DellOEMData.DellLinkAttributes {
		BMCDellAttribute := &DellAttributes{}
		eTag, err := d.GetObjFromUri(data.String(), BMCDellAttribute)
		if err != nil {
			errs = append(errs, err)
		}
		if eTag != nil {
			BMCDellAttribute.Etag = eTag[0]
		}
		BMCDellAttributes = append(BMCDellAttributes, *BMCDellAttribute)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return BMCDellAttributes, nil

}

func (d *DellIdracManager) getFilteredBMCRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	filtered map[string]redfish.Attribute,
	err error,
) {
	// from the registriesAttribure, get the attributes which can be changed.
	registries, err := d.Service.Registries()
	if err != nil {
		return nil, err
	}
	bmcRegistryAttribute := &redfish.AttributeRegistry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "ManagerAttributeRegistry") {
			_, err = d.GetObjFromUri(registry.Location[0].URI, bmcRegistryAttribute)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	// filter out immutable, readonly and hidden attributes
	filteredAttr := make(map[string]redfish.Attribute)
	for _, entry := range bmcRegistryAttribute.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filteredAttr[entry.AttributeName] = entry
		}
	}

	return filteredAttr, nil
}

func (d *DellIdracManager) GetOEMBMCSettingAttribute(
	attributes []string,
) (redfish.SettingsAttributes, error) {

	BMCDellAttributes, err := d.getCurrentBMCSettingAttribute()
	if err != nil {
		return nil, err
	}

	// merge al the current attributes to single map, to help fetch it later
	var mergedBMCAttributes = make(redfish.SettingsAttributes)
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

	filteredAttr, err := d.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}

	if len(filteredAttr) == 0 {
		return nil, fmt.Errorf("'ManagerAttributeRegistry' not found")
	}

	// from the gives attributes to change, find the ones which can be changed and get current value for them
	result := make(redfish.SettingsAttributes, len(attributes))
	var errs []error
	for _, name := range attributes {
		if entry, ok := filteredAttr[name]; ok {
			// enumerations current setting comtains display name.
			// need to be checked with the actual value rather than the display value
			// as the settings provided will have actual values.
			// replace display values with actual values
			if strings.ToLower(string(entry.Type)) == string(redfish.EnumerationAttributeType) {
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
		} else {
			// possible error in settings key
			errs = append(errs, fmt.Errorf("setting key '%v' not found in possible settings", name))
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
	attrs redfish.SettingsAttributes,
	applyTime common.ApplyTime,
) error {

	BMCattributeValues, err := d.getCurrentBMCSettingAttribute()
	if err != nil {
		return err
	}

	payloads := make(map[string]redfish.SettingsAttributes, len(BMCattributeValues))
	for key, value := range attrs {
		for _, eachAttr := range BMCattributeValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				if data, ok := payloads[eachAttr.Settings.SettingsObject.String()]; ok {
					data[key] = value
				} else {
					payloads[eachAttr.Settings.SettingsObject.String()] = make(redfish.SettingsAttributes)
					payloads[eachAttr.Settings.SettingsObject.String()][key] = value
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
			etag, err := func() ([]string, error) {
				resp, err := d.BMC.GetClient().Get(settingPath)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close() // nolint: errcheck
				return resp.Header["Etag"], nil
			}()

			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get Etag for %v. error %v", settingPath, err))
				continue
			}

			data := map[string]interface{}{"Attributes": payload}
			if applyTime != "" {
				data["@Redfish.SettingsApplyTime"] = map[string]string{"ApplyTime": string(applyTime)}
			}
			var header = make(map[string]string)
			if etag != nil {
				header["If-Match"] = etag[0]
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

func (d *DellIdracManager) GetBMCPendingAttributeValues() (redfish.SettingsAttributes, error) {

	BMCattributeValues, err := d.getCurrentBMCSettingAttribute()
	if err != nil {
		return nil, err
	}

	var mergedPendingBMCAttributes = make(redfish.SettingsAttributes)
	var tBMCSetting struct {
		Attributes redfish.SettingsAttributes `json:"Attributes"`
	}

	for _, BMCattributeValue := range BMCattributeValues {
		_, err := d.GetObjFromUri(BMCattributeValue.Settings.SettingsObject.String(), &tBMCSetting)
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

func (d *DellIdracManager) CheckBMCAttributes(attributes redfish.SettingsAttributes) (bool, error) {
	filteredAttr, err := d.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return false, err
	}
	if len(filteredAttr) == 0 {
		return false, nil
	}
	return helpers.CheckAttribues(attributes, filteredAttr)
}

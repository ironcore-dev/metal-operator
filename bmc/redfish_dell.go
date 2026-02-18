// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// DellRedfishBMC is the Dell-specific implementation of the BMC interface.
type DellRedfishBMC struct {
	*RedfishBaseBMC
}

// --- Dell iDRAC manager types ---

type dellAttributes struct {
	Id         string
	Attributes redfish.SettingsAttributes
	Settings   common.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type dellManagerLinksOEM struct {
	DellLinkAttributes  common.Links `json:"DellAttributes"`
	DellAttributesCount int          `json:"DellAttributes@odata.count"`
}

// dellCommonBMCAttributes defines commonly configured Dell iDRAC attributes
// that may not be in the standard registry but are supported by Dell iDRAC.
var dellCommonBMCAttributes = map[string]redfish.Attribute{
	"SysLog.1.SysLogEnable": {
		Type: redfish.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"SysLog.1.SysLogServer1": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"SysLog.1.SysLogServer2": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"NTPConfigGroup.1.NTPEnable": {
		Type: redfish.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP1": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP2": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"EmailAlert.1.Enable": {
		Type: redfish.BooleanAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"EmailAlert.1.Address": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"SNMP.1.AgentEnable": {
		Type: redfish.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"SNMP.1.AgentCommunity": {
		Type: redfish.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
}

// --- Dell helper methods ---

func (r *DellRedfishBMC) getObjFromUri(uri string, respObj any) (string, error) {
	manager, err := r.getManagerForOEM()
	if err != nil {
		return "", err
	}
	resp, err := manager.GetClient().Get(uri)
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

func (r *DellRedfishBMC) getManagerForOEM() (*redfish.Manager, error) {
	manager, err := r.GetManager("")
	if err != nil {
		return nil, fmt.Errorf("not able to get Manager: %v", err)
	}
	if manager.Manufacturer == "" {
		manager.Manufacturer = r.manufacturer
	}
	return manager, nil
}

func (r *DellRedfishBMC) getCurrentBMCSettingAttribute() ([]dellAttributes, error) {
	manager, err := r.getManagerForOEM()
	if err != nil {
		return nil, err
	}

	type temp struct {
		DellOEMData dellManagerLinksOEM `json:"Dell"`
	}

	tempData := &temp{}
	err = json.Unmarshal(manager.OemLinks, tempData)
	if err != nil {
		return nil, err
	}

	bmcDellAttributes := []dellAttributes{}
	var errs []error
	for _, data := range tempData.DellOEMData.DellLinkAttributes {
		bmcDellAttribute := &dellAttributes{}
		eTag, err := r.getObjFromUri(data.String(), bmcDellAttribute)
		if err != nil {
			errs = append(errs, err)
		}
		bmcDellAttribute.Etag = eTag
		bmcDellAttributes = append(bmcDellAttributes, *bmcDellAttribute)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return bmcDellAttributes, nil
}

func (r *DellRedfishBMC) getFilteredBMCRegistryAttributes(readOnly bool, immutable bool) (map[string]redfish.Attribute, error) {
	registries, err := r.client.Service.Registries()
	if err != nil {
		return nil, err
	}
	bmcRegistryAttribute := &redfish.AttributeRegistry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "ManagerAttributeRegistry") {
			_, err = r.getObjFromUri(registry.Location[0].URI, bmcRegistryAttribute)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	filteredAttr := make(map[string]redfish.Attribute)
	for _, entry := range bmcRegistryAttribute.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filteredAttr[entry.AttributeName] = entry
		}
	}
	return filteredAttr, nil
}

// --- BMC interface method overrides ---

func (r *DellRedfishBMC) GetBMCAttributeValues(ctx context.Context, bmcUUID string, attributes map[string]string) (redfish.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	bmcDellAttributes, err := r.getCurrentBMCSettingAttribute()
	if err != nil {
		return nil, err
	}

	var mergedBMCAttributes = make(redfish.SettingsAttributes)
	for _, bmcAttrValue := range bmcDellAttributes {
		for k, v := range bmcAttrValue.Attributes {
			if _, ok := mergedBMCAttributes[k]; !ok {
				mergedBMCAttributes[k] = v
			} else {
				return nil,
					fmt.Errorf("duplicate attributes in BMC settings are not supported duplicate key %v. in attribute %v",
						k, bmcDellAttributes)
			}
		}
	}

	filteredAttr, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	if len(filteredAttr) == 0 {
		return nil, fmt.Errorf("'ManagerAttributeRegistry' not found")
	}

	result := make(redfish.SettingsAttributes, len(attributes))
	var errs []error
	for name := range attributes {
		var entry redfish.Attribute
		var ok bool
		if entry, ok = filteredAttr[name]; !ok {
			if entry, ok = dellCommonBMCAttributes[name]; !ok {
				errs = append(errs, fmt.Errorf("setting key '%v' not found in possible settings", name))
				continue
			}
		}
		if strings.ToLower(string(entry.Type)) == string(redfish.EnumerationAttributeType) {
			for _, attrValue := range entry.Value {
				if attrValue.ValueDisplayName == mergedBMCAttributes[name] {
					result[name] = attrValue.ValueName
					break
				}
			}
			if _, ok := result[name]; !ok {
				errs = append(errs,
					fmt.Errorf("current setting '%v' for key '%v' not found in possible values for it (%v)",
						mergedBMCAttributes[name], name, entry.Value))
			}
		} else {
			result[name] = mergedBMCAttributes[name]
		}
	}
	if len(errs) > 0 {
		return result, fmt.Errorf("some errors found in the settings '%v'.\nPossible settings %v",
			errs, maps.Keys(filteredAttr))
	}

	return result, nil
}

func (r *DellRedfishBMC) GetBMCPendingAttributeValues(ctx context.Context, bmcUUID string) (redfish.SettingsAttributes, error) {
	bmcAttrValues, err := r.getCurrentBMCSettingAttribute()
	if err != nil {
		return nil, err
	}

	var mergedPendingBMCAttributes = make(redfish.SettingsAttributes)
	var tBMCSetting struct {
		Attributes redfish.SettingsAttributes `json:"Attributes"`
	}

	for _, bmcAttrValue := range bmcAttrValues {
		_, err := r.getObjFromUri(bmcAttrValue.Settings.SettingsObject.String(), &tBMCSetting)
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

func (r *DellRedfishBMC) SetBMCAttributesImmediately(ctx context.Context, bmcUUID string, attributes redfish.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}

	bmcAttrValues, err := r.getCurrentBMCSettingAttribute()
	if err != nil {
		return err
	}

	payloads := make(map[string]redfish.SettingsAttributes, len(bmcAttrValues))
	for key, value := range attributes {
		for _, eachAttr := range bmcAttrValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				if data, ok := payloads[eachAttr.Settings.SettingsObject.String()]; ok {
					data[key] = value
				} else {
					payloads[eachAttr.Settings.SettingsObject.String()] = make(redfish.SettingsAttributes)
					payloads[eachAttr.Settings.SettingsObject.String()][key] = value
				}
				break
			}
		}
	}

	if len(payloads) > 0 {
		manager, err := r.getManagerForOEM()
		if err != nil {
			return err
		}
		var errs []error
		for settingPath, payload := range payloads {
			etag, err := func() (string, error) {
				resp, err := manager.GetClient().Get(settingPath)
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
			data["@Redfish.SettingsApplyTime"] = map[string]string{"ApplyTime": string(common.ImmediateApplyTime)}
			var header = make(map[string]string)
			if etag != "" {
				header["If-Match"] = etag
			}

			err = func() error {
				resp, err := manager.GetClient().PatchWithHeaders(settingPath, data, header)
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

func (r *DellRedfishBMC) CheckBMCAttributes(ctx context.Context, bmcUUID string, attrs redfish.SettingsAttributes) (bool, error) {
	filteredAttr, err := r.getFilteredBMCRegistryAttributes(false, false)
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
	return checkAttributes(attrs, filteredAttr)
}

// --- Firmware upgrade overrides ---

func (r *DellRedfishBMC) dellBuildRequestBody(parameters *redfish.SimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.RedfishOperationApplyTime = redfish.ImmediateOperationApplyTime
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Passord = parameters.Passord
	body.Username = parameters.Username
	body.Targets = parameters.Targets
	body.TransferProtocol = parameters.TransferProtocol
	return body
}

func (r *DellRedfishBMC) dellExtractTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body: %w", err)
	}

	if taskMonitor, ok := response.Header["Location"]; ok && len(taskMonitor) > 0 {
		return taskMonitor[0], nil
	}

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

func (r *DellRedfishBMC) dellParseTaskDetails(_ context.Context, taskMonitorResponse *http.Response) (*redfish.Task, error) {
	task := &redfish.Task{}
	rawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(rawBody, &task); err != nil {
		return nil, err
	}
	return task, nil
}

func (r *DellRedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.dellBuildRequestBody, r.dellExtractTaskMonitorURI)
}

func (r *DellRedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*redfish.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.dellParseTaskDetails)
}

func (r *DellRedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *redfish.SimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.dellBuildRequestBody, r.dellExtractTaskMonitorURI)
}

func (r *DellRedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*redfish.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.dellParseTaskDetails)
}

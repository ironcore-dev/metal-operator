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

	"github.com/stmcginnis/gofish/schemas"
)

// DellRedfishBMC is the Dell-specific implementation of the BMC interface.
type DellRedfishBMC struct {
	*RedfishBaseBMC
}

// --- Dell iDRAC manager types ---

type dellAttributes struct {
	Id         string
	Attributes schemas.SettingsAttributes
	Settings   schemas.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type dellManagerLinksOEM struct {
	DellLinkAttributes  schemas.Links `json:"DellAttributes"`
	DellAttributesCount int           `json:"DellAttributes@odata.count"`
}

// dellCommonBMCAttributes defines commonly configured Dell iDRAC attributes
// that may not be in the standard registry but are supported by Dell iDRAC.
var dellCommonBMCAttributes = map[string]schemas.Attributes{
	"SysLog.1.SysLogEnable": {
		Type: schemas.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"SysLog.1.SysLogServer1": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"SysLog.1.SysLogServer2": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"NTPConfigGroup.1.NTPEnable": {
		Type: schemas.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP1": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"NTPConfigGroup.1.NTP2": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"EmailAlert.1.Enable": {
		Type: schemas.BooleanAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"EmailAlert.1.Address": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: false,
	},
	"SNMP.1.AgentEnable": {
		Type: schemas.BooleanAttributeType, ReadOnly: false, ResetRequired: true,
	},
	"SNMP.1.AgentCommunity": {
		Type: schemas.StringAttributeType, ReadOnly: false, ResetRequired: true,
	},
}

// --- Dell helper methods ---

func (r *DellRedfishBMC) getObjFromURI(c schemas.Client, uri string, respObj any) (string, error) {
	resp, err := c.Get(uri)
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

func (r *DellRedfishBMC) getManagerForOEM() (*schemas.Manager, error) {
	manager, err := r.GetManager("")
	if err != nil {
		return nil, fmt.Errorf("not able to get Manager: %v", err)
	}
	if manager.Manufacturer == "" {
		manager.Manufacturer = r.manufacturer
	}
	return manager, nil
}

func (r *DellRedfishBMC) getCurrentBMCSettingAttribute(manager *schemas.Manager) ([]dellAttributes, error) {
	type temp struct {
		DellOEMData dellManagerLinksOEM `json:"Dell"`
	}

	tempData := &temp{}
	err := json.Unmarshal(manager.RawData, tempData)
	if err != nil {
		return nil, err
	}

	c := manager.GetClient()
	bmcDellAttributes := []dellAttributes{}
	var errs []error
	for _, data := range tempData.DellOEMData.DellLinkAttributes {
		bmcDellAttribute := &dellAttributes{}
		eTag, err := r.getObjFromURI(c, data.String(), bmcDellAttribute)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		bmcDellAttribute.Etag = eTag
		bmcDellAttributes = append(bmcDellAttributes, *bmcDellAttribute)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return bmcDellAttributes, nil
}

func (r *DellRedfishBMC) getFilteredBMCRegistryAttributes(manager *schemas.Manager, readOnly bool, immutable bool) (map[string]schemas.Attributes, error) {
	registries, err := r.client.Service.Registries()
	if err != nil {
		return nil, err
	}
	c := manager.GetClient()
	bmcRegistryAttribute := &schemas.AttributeRegistry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "ManagerAttributeRegistry") {
			if len(registry.Location) == 0 {
				return nil, fmt.Errorf("ManagerAttributeRegistry %q has no Location entries", registry.ID)
			}
			_, err = r.getObjFromURI(c, registry.Location[0].URI, bmcRegistryAttribute)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	filteredAttr := make(map[string]schemas.Attributes)
	for _, entry := range bmcRegistryAttribute.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filteredAttr[entry.AttributeName] = entry
		}
	}
	return filteredAttr, nil
}

// --- BMC interface method overrides ---

func (r *DellRedfishBMC) GetBMCAttributeValues(ctx context.Context, bmcUUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	manager, err := r.getManagerForOEM()
	if err != nil {
		return nil, err
	}

	bmcDellAttributes, err := r.getCurrentBMCSettingAttribute(manager)
	if err != nil {
		return nil, err
	}

	var mergedBMCAttributes = make(schemas.SettingsAttributes)
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

	filteredAttr, err := r.getFilteredBMCRegistryAttributes(manager, false, false)
	if err != nil {
		return nil, err
	}
	if len(filteredAttr) == 0 {
		return nil, fmt.Errorf("'ManagerAttributeRegistry' not found")
	}

	result := make(schemas.SettingsAttributes, len(attributes))
	var errs []error
	for name := range attributes {
		var entry schemas.Attributes
		var ok bool
		if entry, ok = filteredAttr[name]; !ok {
			if entry, ok = dellCommonBMCAttributes[name]; !ok {
				errs = append(errs, fmt.Errorf("setting key '%v' not found in possible settings", name))
				continue
			}
		}
		if strings.EqualFold(string(entry.Type), string(schemas.EnumerationAttributeType)) {
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

func (r *DellRedfishBMC) GetBMCPendingAttributeValues(ctx context.Context, bmcUUID string) (schemas.SettingsAttributes, error) {
	manager, err := r.getManagerForOEM()
	if err != nil {
		return nil, err
	}

	bmcAttrValues, err := r.getCurrentBMCSettingAttribute(manager)
	if err != nil {
		return nil, err
	}

	c := manager.GetClient()
	var mergedPendingBMCAttributes = make(schemas.SettingsAttributes)

	for _, bmcAttrValue := range bmcAttrValues {
		var tBMCSetting struct {
			Attributes schemas.SettingsAttributes `json:"Attributes"`
		}
		_, err := r.getObjFromURI(c, bmcAttrValue.Settings.SettingsObject, &tBMCSetting)
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

func (r *DellRedfishBMC) SetBMCAttributesImmediately(ctx context.Context, bmcUUID string, attributes schemas.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}

	manager, err := r.getManagerForOEM()
	if err != nil {
		return err
	}

	bmcAttrValues, err := r.getCurrentBMCSettingAttribute(manager)
	if err != nil {
		return err
	}

	payloads := make(map[string]schemas.SettingsAttributes, len(bmcAttrValues))
	for key, value := range attributes {
		for _, eachAttr := range bmcAttrValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				if data, ok := payloads[eachAttr.Settings.SettingsObject]; ok {
					data[key] = value
				} else {
					payloads[eachAttr.Settings.SettingsObject] = make(schemas.SettingsAttributes)
					payloads[eachAttr.Settings.SettingsObject][key] = value
				}
				break
			}
		}
	}

	if len(payloads) > 0 {
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
			data["@Redfish.SettingsApplyTime"] = map[string]string{"ApplyTime": string(schemas.ImmediateSettingsApplyTime)}
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

func (r *DellRedfishBMC) CheckBMCAttributes(ctx context.Context, bmcUUID string, attrs schemas.SettingsAttributes) (bool, error) {
	manager, err := r.getManagerForOEM()
	if err != nil {
		return false, err
	}

	filteredAttr, err := r.getFilteredBMCRegistryAttributes(manager, false, false)
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

func (r *DellRedfishBMC) dellBuildRequestBody(parameters *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.RedfishOperationApplyTime = schemas.ImmediateOperationApplyTime
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Password = parameters.Password
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

func (r *DellRedfishBMC) dellParseTaskDetails(_ context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
	rawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(rawBody, &task); err != nil {
		return nil, err
	}
	return task, nil
}

func (r *DellRedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.dellBuildRequestBody, r.dellExtractTaskMonitorURI)
}

func (r *DellRedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.dellParseTaskDetails)
}

func (r *DellRedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.dellBuildRequestBody, r.dellExtractTaskMonitorURI)
}

func (r *DellRedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.dellParseTaskDetails)
}

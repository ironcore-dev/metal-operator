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
	"path"
	"strconv"
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
	SourceURI  string
	Attributes schemas.SettingsAttributes
	Settings   schemas.Settings `json:"@Redfish.Settings"`
	Etag       string
}

type dellManagerLinksOEM struct {
	DellLinkAttributes  schemas.Links `json:"DellAttributes"`
	DellAttributesCount int           `json:"DellAttributes@odata.count"`
}

// dellRegistryAttribute embeds schemas.Attributes and overrides only the fields
// that Dell iDRAC serializes differently:
//   - LowerBound/UpperBound: Dell sends negative values (e.g. -1 = "no limit"),
//     which cannot be represented as *uint64.
//   - ReadOnly: Dell uses the JSON key "Readonly" (lowercase 'o').
type dellRegistryAttribute struct {
	schemas.Attributes
	LowerBound *int64 `json:"LowerBound,omitempty"` // shadows Attributes.LowerBound (*uint64)
	UpperBound *int64 `json:"UpperBound,omitempty"` // shadows Attributes.UpperBound (*uint64)
	ReadOnly   bool   `json:"Readonly"`             // Dell uses "Readonly", not "ReadOnly"
}

type dellAttributeRegistry struct {
	RegistryEntries struct {
		Attributes []dellRegistryAttribute `json:"Attributes"`
	} `json:"RegistryEntries"`
}

// toSchemaAttributes converts a dellRegistryAttribute to schemas.Attributes.
// LowerBound and UpperBound are omitted when negative (not representable as uint64).
func (d dellRegistryAttribute) toSchemaAttributes() schemas.Attributes {
	a := d.Attributes
	a.ReadOnly = d.ReadOnly
	if d.LowerBound != nil && *d.LowerBound >= 0 {
		v := uint64(*d.LowerBound)
		a.LowerBound = &v
	}
	if d.UpperBound != nil && *d.UpperBound >= 0 {
		v := uint64(*d.UpperBound)
		a.UpperBound = &v
	}
	return a
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
		return nil, fmt.Errorf("failed to get Manager: %w", err)
	}
	if manager.Manufacturer == "" {
		manager.Manufacturer = r.manufacturer
	}
	return manager, nil
}

func (r *DellRedfishBMC) getCurrentBMCSettingAttribute(manager *schemas.Manager) ([]dellAttributes, error) {
	type temp struct {
		Links struct {
			Oem struct {
				DellOEMData dellManagerLinksOEM `json:"Dell"`
			} `json:"Oem"`
		} `json:"Links"`
	}

	tempData := &temp{}
	err := json.Unmarshal(manager.RawData, tempData)
	if err != nil {
		return nil, err
	}

	c := manager.GetClient()
	bmcDellAttributes := []dellAttributes{}
	var errs []error
	for _, data := range tempData.Links.Oem.DellOEMData.DellLinkAttributes {
		bmcDellAttribute := &dellAttributes{}
		eTag, err := r.getObjFromURI(c, data.String(), bmcDellAttribute)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		bmcDellAttribute.Etag = eTag
		bmcDellAttribute.SourceURI = data.String()
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
	bmcRegistryAttribute := &dellAttributeRegistry{}
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
			filteredAttr[entry.AttributeName] = entry.toSchemaAttributes()
		}
	}
	return filteredAttr, nil
}

// --- BMC interface method overrides ---

func (r *DellRedfishBMC) GetBMCAttributeValues(ctx context.Context, req GetBMCAttributeValuesRequest) (schemas.SettingsAttributes, error) {
	attributes := req.Attributes
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
		currentVal, hasCurrentVal := mergedBMCAttributes[name]
		if !hasCurrentVal {
			errs = append(errs, fmt.Errorf("attribute '%v' not found in any DellAttributes endpoint", name))
			continue
		}

		switch entry.Type {
		case schemas.EnumerationAttributeType:
			// iDRAC reports enumeration current values as ValueDisplayName (e.g. "Enabled").
			// The spec may use either ValueDisplayName ("Enabled") or ValueName ("1").
			// To make the diff comparison work correctly regardless of which format the spec
			// uses, we return the current value in the same format as the spec value:
			//   - spec uses ValueName  → translate current DisplayName → ValueName
			//   - spec uses DisplayName → return current DisplayName as-is
			// This avoids spurious diffs (and unnecessary BMC resets) when the attribute is
			// already at the desired value but the two strings differ only in format.
			specVal := attributes[name]
			// Build lookup maps in a single pass.
			displayNameToValueName := make(map[string]string)
			valueNameExists := make(map[string]bool)
			for _, attrValue := range entry.Value {
				displayNameToValueName[attrValue.ValueDisplayName] = attrValue.ValueName
				valueNameExists[attrValue.ValueName] = true
			}
			currentDisplayName, ok := currentVal.(string)
			if !ok {
				errs = append(errs,
					fmt.Errorf("current setting '%v' for key '%v' has unexpected type",
						currentVal, name))
				continue
			}
			// Validate that iDRAC returned a known DisplayName.
			if _, exists := displayNameToValueName[currentDisplayName]; !exists {
				errs = append(errs,
					fmt.Errorf("current setting '%v' for key '%v' not found in possible values: %v",
						currentVal, name, entry.Value))
				continue
			}
			if valueNameExists[specVal] {
				// Spec uses ValueName; translate current DisplayName to ValueName.
				result[name] = displayNameToValueName[currentDisplayName]
			} else {
				// Spec uses ValueDisplayName; return as-is.
				result[name] = currentDisplayName
			}
		case schemas.IntegerAttributeType:
			// Convert raw JSON value to the correct Go type based on registry metadata.
			// JSON numbers unmarshal as float64 into map[string]any; convert to int
			// for IntegerAttributeType so downstream validation (checkAttributes) passes.
			if f, ok := currentVal.(float64); ok {
				result[name] = int(f)
			} else {
				result[name] = currentVal
			}
		default:
			result[name] = currentVal
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

func (r *DellRedfishBMC) SetBMCAttributesImmediately(ctx context.Context, bmcUUID string, attributes schemas.SettingsAttributes) (map[string]ApplyResult, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	manager, err := r.getManagerForOEM()
	if err != nil {
		return nil, err
	}

	bmcAttrValues, err := r.getCurrentBMCSettingAttribute(manager)
	if err != nil {
		return nil, err
	}

	payloads := make(map[string]schemas.SettingsAttributes, len(bmcAttrValues))
	for key, value := range attributes {
		for _, eachAttr := range bmcAttrValues {
			if _, ok := eachAttr.Attributes[key]; ok {
				target := eachAttr.Settings.SettingsObject
				if target == "" {
					target = eachAttr.SourceURI
				}
				if target == "" {
					return nil, fmt.Errorf("attribute '%v' has no target endpoint to patch", key)
				}
				if data, ok := payloads[target]; ok {
					data[key] = value
				} else {
					payloads[target] = make(schemas.SettingsAttributes)
					payloads[target][key] = value
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
				errs = append(errs, fmt.Errorf("failed to get Etag for %v: %w", settingPath, err))
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
				errs = append(errs, fmt.Errorf("failed to patch settings at %v: %w", settingPath, err))
				continue
			}
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("some settings failed to apply %v", errs)
		}
	}
	return nil, nil
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

// CheckBMCPendingComponentUpgrade checks for staged component upgrades (Dell: Staged=true).
func (r *DellRedfishBMC) CheckBMCPendingComponentUpgrade(ctx context.Context, componentType ComponentType) (bool, error) {
	if componentType != ComponentTypeBMC && componentType != ComponentTypeBIOS {
		return false, fmt.Errorf("unsupported component type: %q", componentType)
	}
	return checkPendingComponentUpgrade(ctx, r.RedfishBaseBMC, componentType, r.dellGetComponentFilters, r.dellMatchesComponentFilter, r.dellCheckPending)
}

func (r *DellRedfishBMC) dellGetComponentFilters(componentType ComponentType) []string {
	switch componentType {
	case ComponentTypeBMC:
		return []string{"iDRAC"}
	case ComponentTypeBIOS:
		return []string{"BIOS", "BIOS-PRIMARY"}
	default:
		return []string{}
	}
}

func (r *DellRedfishBMC) dellMatchesComponentFilter(fw *schemas.SoftwareInventory, filters []string) bool {
	idUpper := strings.ToUpper(fw.ID)
	for _, filter := range filters {
		if strings.Contains(idUpper, strings.ToUpper(filter)) {
			return true
		}
	}
	return false
}

func (r *DellRedfishBMC) dellCheckPending(fw *schemas.SoftwareInventory) bool {
	return fw.Staged
}

// --- Repository-based firmware update (FirmwareUpdaterDell) ---

// FirmwareUpdaterDell is a Dell-only interface for OEM repository-based
// firmware updates. It is intentionally not part of the BMC union interface —
// no other vendor exposes an equivalent OEM action today. Callers discover
// support via a type assertion: updater, ok := bmcClient.(bmc.FirmwareUpdaterDell).
type FirmwareUpdaterDell interface {
	InstallFirmwareFromRepository(ctx context.Context, systemURI string, parameters *RepositoryUpdateParameters) (jobID string, isFatal bool, err error)
	GetRepositoryUpdateList(ctx context.Context, systemURI string) (hasPendingPackages bool, packageListXML string, err error)
	ListJobs(ctx context.Context, UUID string) ([]string, error)
	GetJob(ctx context.Context, UUID string, jobID string) (*DellJob, error)
}

// RepositoryUpdateParameters are the parameters for Dell's InstallFromRepository OEM action.
type RepositoryUpdateParameters struct {
	ShareType              string
	IPAddress              string
	ShareName              string
	CatalogFile            string
	UserName               string
	Password               string
	Workgroup              string
	IgnoreCertWarning      bool
	ApplyUpdate            bool
	RebootNeeded           bool
	ApplySameVersions      bool
	ApplyDowngradeVersions bool
}

// DellJob represents a Dell iDRAC job resource.
type DellJob struct {
	ID      string
	Name    string
	JobType string
	// State is the raw Dell "JobState" string — not a typed gofish enum, since
	// Dell's OEM states ("Failed", "CompletedWithErrors", "RebootFailed",
	// "Scheduled") have no equivalent in schemas.JobState or schemas.TaskState.
	State           string
	Message         string
	PercentComplete int32
}

// Known Dell iDRAC JobState values (no formal enum published by Dell).
const (
	dellJobStateCompleted           = "Completed"
	dellJobStateCompletedWithErrors = "CompletedWithErrors"
	dellJobStateFailed              = "Failed"
	dellJobStateRebootFailed        = "RebootFailed"
)

// IsCompleted reports whether the job has reached a successful terminal state.
func (j *DellJob) IsCompleted() bool {
	return j.State == dellJobStateCompleted
}

// IsFailed reports whether the job has reached a known terminal failure state.
func (j *DellJob) IsFailed() bool {
	switch j.State {
	case dellJobStateFailed, dellJobStateCompletedWithErrors, dellJobStateRebootFailed:
		return true
	}
	return false
}

// IsTerminal reports whether the job has reached any terminal state.
func (j *DellJob) IsTerminal() bool {
	return j.IsCompleted() || j.IsFailed()
}

// FirmwareUpdaterDell is not part of the BMC union; assert separately from bmc.go.
var _ FirmwareUpdaterDell = (*DellRedfishBMC)(nil)

const dellSoftwareInstallationServicePath = "/Oem/Dell/DellSoftwareInstallationService"

// dellRepositoryUpdateRequestBody is the JSON body for Dell's InstallFromRepository OEM action.
//
// NOTE: field names and encodings are based on Dell's publicly documented iDRAC Redfish
// scripting examples and have not been verified against a real iDRAC — Dell does not
// publish an OpenAPI schema for this OEM action.
type dellRepositoryUpdateRequestBody struct {
	ShareType              string `json:"ShareType"`
	IPAddress              string `json:"IPAddress,omitempty"`
	ShareName              string `json:"ShareName,omitempty"`
	CatalogFile            string `json:"CatalogFile,omitempty"`
	UserName               string `json:"UserName,omitempty"`
	Password               string `json:"Password,omitempty"`
	Workgroup              string `json:"Workgroup,omitempty"`
	IgnoreCertWarning      string `json:"IgnoreCertWarning,omitempty"`
	ApplyUpdate            string `json:"ApplyUpdate"`
	RebootNeeded           bool   `json:"RebootNeeded"`
	ApplySameVersions      string `json:"ApplySameVersions,omitempty"`
	ApplyDowngradeVersions string `json:"ApplyDowngradeVersions,omitempty"`
}

func dellBoolString(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func dellOnOffString(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

func dellBuildRepositoryUpdateRequestBody(parameters *RepositoryUpdateParameters) *dellRepositoryUpdateRequestBody {
	return &dellRepositoryUpdateRequestBody{
		ShareType:              parameters.ShareType,
		IPAddress:              parameters.IPAddress,
		ShareName:              parameters.ShareName,
		CatalogFile:            parameters.CatalogFile,
		UserName:               parameters.UserName,
		Password:               parameters.Password,
		Workgroup:              parameters.Workgroup,
		IgnoreCertWarning:      dellOnOffString(parameters.IgnoreCertWarning),
		ApplyUpdate:            dellBoolString(parameters.ApplyUpdate),
		RebootNeeded:           parameters.RebootNeeded,
		ApplySameVersions:      dellBoolString(parameters.ApplySameVersions),
		ApplyDowngradeVersions: dellBoolString(parameters.ApplyDowngradeVersions),
	}
}

func dellRepositoryActionTarget(systemURI, action string) string {
	return path.Join(systemURI, dellSoftwareInstallationServicePath, "Actions", "DellSoftwareInstallationService."+action)
}

// dellExtractJobID extracts the iDRAC job ID from the Location header (mirrors dellExtractTaskMonitorURI).
func dellExtractJobID(response *http.Response) (string, error) {
	if location, ok := response.Header["Location"]; ok && len(location) > 0 {
		return path.Base(location[0]), nil
	}
	return "", fmt.Errorf("unable to extract job ID from Dell iDRAC response: no Location header")
}

func (r *DellRedfishBMC) InstallFirmwareFromRepository(_ context.Context, systemURI string, parameters *RepositoryUpdateParameters) (string, bool, error) {
	client := r.client.GetService().GetClient()
	target := dellRepositoryActionTarget(systemURI, "InstallFromRepository")

	resp, err := client.Post(target, dellBuildRepositoryUpdateRequestBody(parameters))
	if err != nil {
		// Any error after POST is treated as fatal: the request may have reached
		// the BMC and started executing (including dropped-response errors), so
		// retrying risks triggering a second install.
		return "", true, fmt.Errorf("failed to issue repository firmware install: %w", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	jobID, err := dellExtractJobID(resp)
	if err != nil {
		return "", true, fmt.Errorf("failed to extract job ID from repository firmware install response: %w", err)
	}
	return jobID, false, nil
}

type dellGetRepoBasedUpdateListResponse struct {
	PackageList string `json:"PackageList"`
}

func (r *DellRedfishBMC) GetRepositoryUpdateList(_ context.Context, systemURI string) (bool, string, error) {
	client := r.client.GetService().GetClient()
	target := dellRepositoryActionTarget(systemURI, "GetRepoBasedUpdateList")

	resp, err := client.Post(target, map[string]any{})
	if err != nil {
		// Treat a "no catalog match" or SUP099 (no applicable updates) as nothing pending.
		var redfishErr *schemas.Error
		if errors.As(err, &redfishErr) {
			msg := strings.ToLower(redfishErr.Message)
			if strings.Contains(msg, "match catalog") {
				return false, "", nil
			}
			for _, ei := range redfishErr.ExtendedInfos {
				if ei.MessageID == "IDRAC.2.1.SUP099" {
					return false, "", nil
				}
			}
		}
		return false, "", fmt.Errorf("failed to get repository update list: %w", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("failed to read GetRepoBasedUpdateList response body: %w", err)
	}

	var parsed dellGetRepoBasedUpdateListResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return false, "", fmt.Errorf("failed to unmarshal GetRepoBasedUpdateList response: %w", err)
	}

	return dellPackageListHasPendingPackages(parsed.PackageList), parsed.PackageList, nil
}

// dellPackageListHasPendingPackages checks for pending packages in Dell's PackageList XML.
// The schema isn't formally documented — any <PACKAGE> or <INSTANCE> element is treated as pending.
func dellPackageListHasPendingPackages(packageListXML string) bool {
	if strings.TrimSpace(packageListXML) == "" {
		return false
	}
	upper := strings.ToUpper(packageListXML)
	return strings.Contains(upper, "<PACKAGE") || strings.Contains(upper, "<INSTANCE")
}

type dellJob struct {
	ID              string          `json:"Id"`
	Name            string          `json:"Name"`
	JobType         string          `json:"JobType"`
	JobState        string          `json:"JobState"`
	Message         string          `json:"Message"`
	PercentComplete json.RawMessage `json:"PercentComplete"`
}

// dellParsePercentComplete accepts either a number or a quoted string (both seen in iDRAC responses).
func dellParsePercentComplete(raw json.RawMessage) int32 {
	if len(raw) == 0 {
		return 0
	}
	var n int32
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if v, err := strconv.ParseInt(s, 10, 32); err == nil {
			return int32(v)
		}
	}
	return 0
}

type dellJobsCollection struct {
	Members []struct {
		ODataID string `json:"@odata.id"`
	} `json:"Members"`
}

func (r *DellRedfishBMC) ListJobs(_ context.Context, UUID string) ([]string, error) {
	manager, err := r.GetManager(UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get manager: %w", err)
	}

	client := r.client.GetService().GetClient()
	jobsURI := path.Join(manager.ODataID, "Oem", "Dell", "Jobs")
	resp, err := client.Get(jobsURI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read jobs collection response body: %w", err)
	}

	var collection dellJobsCollection
	if err := json.Unmarshal(rawBody, &collection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal jobs collection: %w", err)
	}

	jobIDs := make([]string, 0, len(collection.Members))
	for _, m := range collection.Members {
		jobIDs = append(jobIDs, path.Base(m.ODataID))
	}
	return jobIDs, nil
}

func (r *DellRedfishBMC) GetJob(_ context.Context, UUID string, jobID string) (*DellJob, error) {
	manager, err := r.GetManager(UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get manager: %w", err)
	}

	client := r.client.GetService().GetClient()
	jobURI := path.Join(manager.ODataID, "Oem", "Dell", "Jobs", jobID)
	resp, err := client.Get(jobURI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read job response body: %w", err)
	}

	var dj dellJob
	if err := json.Unmarshal(rawBody, &dj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &DellJob{
		ID:              dj.ID,
		Name:            dj.Name,
		JobType:         dj.JobType,
		State:           dj.JobState,
		Message:         dj.Message,
		PercentComplete: dellParsePercentComplete(dj.PercentComplete),
	}, nil
}

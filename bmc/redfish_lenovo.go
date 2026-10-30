// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

// LenovoRedfishBMC is the Lenovo-specific implementation of the BMC interface.
type LenovoRedfishBMC struct {
	*RedfishBaseBMC
}

// --- BMC interface method overrides ---

func (r *LenovoRedfishBMC) GetBMCAttributeValues(ctx context.Context, req GetBMCAttributeValuesRequest) (schemas.SettingsAttributes, error) {
	attributes := req.Attributes
	if len(attributes) == 0 {
		return nil, nil
	}
	result, err := httpBasedGetBMCSettingAttribute(r.client.GetService().GetClient(), attributes)
	if err != nil {
		return result, err
	}
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Fetched data from BMC Settings ", "Result", result)
	return result, nil
}

func (r *LenovoRedfishBMC) GetBMCPendingAttributeValues(_ context.Context, _ string) (schemas.SettingsAttributes, error) {
	return schemas.SettingsAttributes{}, nil
}

func (r *LenovoRedfishBMC) SetBMCAttributesImmediately(_ context.Context, _ string, attributes schemas.SettingsAttributes) (map[string]ApplyResult, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	return httpBasedUpdateBMCAttributes(r.client.GetService().GetClient(), attributes, schemas.ImmediateSettingsApplyTime)
}

func (r *LenovoRedfishBMC) CheckBMCAttributes(_ context.Context, _ string, _ schemas.SettingsAttributes) (bool, error) {
	return false, nil
}

// FetchETags returns the current ETag for each URI. Lenovo XCC exposes ETags on BIOS Pending resources.
func (r *LenovoRedfishBMC) FetchETags(_ context.Context, uris []string) (map[string]string, error) {
	return httpFetchETags(r.client.GetService().GetClient(), uris)
}

// --- Firmware upgrade overrides ---

func (r *LenovoRedfishBMC) lenovoBuildRequestBody(parameters *schemas.UpdateServiceSimpleUpdateParameters) *SimpleUpdateRequestBody {
	body := &SimpleUpdateRequestBody{}
	body.ForceUpdate = parameters.ForceUpdate
	body.ImageURI = parameters.ImageURI
	body.Password = parameters.Password
	body.Username = parameters.Username
	body.Targets = parameters.Targets
	body.TransferProtocol = parameters.TransferProtocol
	return body
}

func (r *LenovoRedfishBMC) lenovoExtractTaskMonitorURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read the response body %w %v", err, rawBody)
	}

	var tResp struct {
		TaskMonitor string `json:"@odata.id,omitempty"`
	}
	if err = json.Unmarshal(rawBody, &tResp); err != nil {
		return "", fmt.Errorf("failed to Unmarshal taskMonitor URI %w", err)
	}

	if tResp.TaskMonitor == "" {
		return "", fmt.Errorf("lenovoExtractTaskMonitorURI: missing @odata.id in response")
	}

	return tResp.TaskMonitor, nil
}

func (r *LenovoRedfishBMC) lenovoParseTaskDetails(ctx context.Context, taskMonitorResponse *http.Response) (*schemas.Task, error) {
	task := &schemas.Task{}
	rawBody, err := io.ReadAll(taskMonitorResponse.Body)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(rawBody, &task); err != nil {
		return nil, err
	}

	if len(task.Messages) > 0 && task.TaskState == schemas.CompletedTaskState && task.TaskStatus == schemas.OKHealth {
		for _, msg := range task.Messages {
			if strings.Contains(msg.MessageID, "OperationTransitionedToJob") && len(msg.MessageArgs) > 0 {
				respJob, err := r.client.GetService().GetClient().Get(msg.MessageArgs[0])
				if err != nil {
					return nil, err
				}
				defer respJob.Body.Close() // nolint: errcheck

				if respJob.StatusCode != http.StatusAccepted && respJob.StatusCode != http.StatusOK {
					respJobRawBody, err := io.ReadAll(respJob.Body)
					if err != nil {
						return nil,
							fmt.Errorf("failed to get the upgrade Task details and read the response body: %v, statusCode: %v",
								err, respJob.StatusCode)
					}
					return nil,
						fmt.Errorf("failed to get the upgrade Task details. %v, statusCode %v",
							string(respJobRawBody), respJob.StatusCode)
				}

				respJobRawBody, err := io.ReadAll(respJob.Body)
				if err != nil {
					return nil,
						fmt.Errorf("failed to get the upgrade Task details and read the response body: %v, statusCode: %v",
							err, respJob.StatusCode)
				}

				job := &schemas.Job{}
				if err = json.Unmarshal(respJobRawBody, &job); err != nil {
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

func (r *LenovoRedfishBMC) UpgradeBiosVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.lenovoBuildRequestBody, r.lenovoExtractTaskMonitorURI)
}

func (r *LenovoRedfishBMC) GetBiosUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.lenovoParseTaskDetails)
}

func (r *LenovoRedfishBMC) UpgradeBMCVersion(ctx context.Context, _ string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	return upgradeVersion(ctx, r.RedfishBaseBMC, parameters, r.lenovoBuildRequestBody, r.lenovoExtractTaskMonitorURI)
}

func (r *LenovoRedfishBMC) GetBMCUpgradeTask(ctx context.Context, _ string, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.lenovoParseTaskDetails)
}

// CheckBMCPendingComponentUpgrade checks for pending component upgrades (Lenovo: "-Pending" suffix).
func (r *LenovoRedfishBMC) CheckBMCPendingComponentUpgrade(ctx context.Context, componentType ComponentType) (bool, error) {
	if componentType != ComponentTypeBMC && componentType != ComponentTypeBIOS {
		return false, fmt.Errorf("unsupported component type: %q", componentType)
	}
	return checkPendingComponentUpgrade(ctx, r.RedfishBaseBMC, componentType, r.lenovoGetComponentFilters, r.lenovoMatchesComponentFilter, r.lenovoCheckPending)
}

func (r *LenovoRedfishBMC) lenovoGetComponentFilters(componentType ComponentType) []string {
	switch componentType {
	case ComponentTypeBMC:
		return []string{"Firmware:BMC"}
	case ComponentTypeBIOS:
		return []string{"Firmware:UEFI", "BIOS"}
	default:
		return []string{}
	}
}

func (r *LenovoRedfishBMC) lenovoMatchesComponentFilter(fw *schemas.SoftwareInventory, filters []string) bool {
	idUpper := strings.ToUpper(fw.ID)
	for _, filter := range filters {
		if strings.Contains(idUpper, strings.ToUpper(filter)) {
			return true
		}
	}
	return false
}

func (r *LenovoRedfishBMC) lenovoCheckPending(fw *schemas.SoftwareInventory) bool {
	return strings.Contains(strings.ToUpper(fw.ID), "-PENDING")
}

// --- Repository-based firmware update (FirmwareUpdaterLenovo) ---

// FirmwareUpdaterLenovo is a Lenovo-only interface for the XCC OEM
// repository-based firmware update. It is intentionally not part of the BMC
// union interface. Callers discover support via a type assertion:
// updater, ok := bmcClient.(bmc.FirmwareUpdaterLenovo).
//
// The repository is expected to contain no XCC/BMC or UEFI packages: those
// components are managed by the BMCVersion and BIOSVersion controllers.
type FirmwareUpdaterLenovo interface {
	InstallFirmwareFromRepository(ctx context.Context, parameters *LenovoRepositoryUpdateParameters) (taskURI string, isFatal bool, err error)
	GetRepositoryUpdateTask(ctx context.Context, taskURI string) (*schemas.Task, error)
	GetPendingFirmwareComponents(ctx context.Context) ([]string, error)
	GetRepositoryUpdateDetail(ctx context.Context) (*LenovoRepositoryUpdateDetail, error)
}

// LenovoRepositoryUpdateDetail is the response of the XCC
// LenovoUpdateService.GetRepoUpdateDetail OEM action. It only describes the
// last repository update run on the XCC
type LenovoRepositoryUpdateDetail struct {
	// BundleErrorCode is the undocumented XCC result code of the whole run.
	// Non-zero values were observed on successful runs.
	BundleErrorCode int                               `json:"BundleErrorCode"`
	FinishedList    []LenovoRepositoryUpdateComponent `json:"FinishedList"`
}

// LenovoRepositoryUpdateComponent is one update package processed by a repository update.
type LenovoRepositoryUpdateComponent struct {
	// Category is the package category, e.g. "LXPM", "DEVICE", "DISK" or "FPGA".
	Category string `json:"category"`
	// Method is the update method, e.g. "Core" or "OOB".
	Method string `json:"method"`
	// Payload is the path of the package inside the repository.
	Payload string `json:"payload"`
	// Reboot is "reboot" if a host reboot is needed to activate the package, "noreboot" otherwise.
	Reboot string `json:"reboot"`
	// Ret is the undocumented XCC result code of the package update. It must
	// not be treated as an error code: 2 was observed for a successfully applied
	// package that needed a reboot, 0 for a package that was already installed.
	Ret     int                            `json:"ret"`
	Devices []LenovoRepositoryUpdateDevice `json:"devices"`
}

// LenovoRepositoryUpdateDevice is a device updated by a repository update package.
type LenovoRepositoryUpdateDevice struct {
	Name         string `json:"name"`
	Location     string `json:"location"`
	PartNumber   string `json:"partnum"`
	SerialNumber string `json:"serialnum"`
	SoftwareID   string `json:"swid"`
	// Version is the firmware version that was active on the device when the
	// run started, not the version of the package.
	Version string `json:"version"`
	OOB     bool   `json:"oob"`
}

// LenovoRepositoryUpdateParameters are the parameters for the XCC
// LenovoUpdateService.UpdateFromRepository OEM action.
type LenovoRepositoryUpdateParameters struct {
	// RepoURI is the HTTPS URI of the repository zip archive.
	RepoURI string
}

// FirmwareUpdaterLenovo is not part of the BMC union; assert separately from bmc.go.
var _ FirmwareUpdaterLenovo = (*LenovoRedfishBMC)(nil)

const (
	lenovoUpdateFromRepositoryPath = "/redfish/v1/UpdateService/Actions/Oem/LenovoUpdateService.UpdateFromRepository"
	lenovoGetRepoUpdateDetailPath  = "/redfish/v1/UpdateService/Actions/Oem/LenovoUpdateService.GetRepoUpdateDetail"
	lenovoPendingSlotSuffix        = "-PENDING"
	lenovoTaskServicePathPrefix    = "/redfish/v1/TaskService/"
)

// lenovoPendingExcludedPrefixes lists (upper-case) firmware inventory ID
// prefixes of components that are not handled by the repository update.
var lenovoPendingExcludedPrefixes = []string{"BMC", "XCC", "UEFI"}

type lenovoRepositoryUpdateRequestBody struct {
	RepoURI string `json:"RepoURI"`
}

// lenovoExtractRepositoryTaskURI prefers the task's @odata.id from the response
// body (/redfish/v1/TaskService/Tasks/<id>) over the Location header
// (/redfish/v1/TaskService/TaskMonitors/<id>).
func lenovoExtractRepositoryTaskURI(response *http.Response) (string, error) {
	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if len(rawBody) > 0 {
		var task struct {
			ODataID string `json:"@odata.id"`
		}
		if err := json.Unmarshal(rawBody, &task); err == nil && task.ODataID != "" {
			return lenovoNormalizeTaskURI(task.ODataID)
		}
	}
	if location := response.Header.Get("Location"); location != "" {
		return lenovoNormalizeTaskURI(location)
	}
	return "", fmt.Errorf("missing task URI in response: no @odata.id in body and no Location header")
}

// lenovoNormalizeTaskURI turns an absolute URL into its path (gofish prefixes
// every request with the BMC endpoint) and rejects URIs outside the TaskService.
func lenovoNormalizeTaskURI(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", fmt.Errorf("invalid task URI %q: %w", rawURI, err)
	}
	cleaned := path.Clean(parsed.Path)
	if !strings.HasPrefix(cleaned, lenovoTaskServicePathPrefix) {
		return "", fmt.Errorf("task URI %q is not a TaskService resource", rawURI)
	}
	return (&url.URL{Path: cleaned, RawQuery: parsed.RawQuery}).RequestURI(), nil
}

func lenovoBuildRepositoryUpdateRequestBody(parameters *LenovoRepositoryUpdateParameters) *lenovoRepositoryUpdateRequestBody {
	return &lenovoRepositoryUpdateRequestBody{
		RepoURI: parameters.RepoURI,
	}
}

// InstallFirmwareFromRepository starts an update from the repository given in
// parameters and returns the URI of the resulting task
// (/redfish/v1/TaskService/Tasks/<id>). isFatal is true once the request may
// have reached the XCC, because retrying could start a second update.
func (r *LenovoRedfishBMC) InstallFirmwareFromRepository(ctx context.Context, parameters *LenovoRepositoryUpdateParameters) (string, bool, error) {
	if parameters == nil || parameters.RepoURI == "" {
		return "", false, fmt.Errorf("repository URI must not be empty")
	}
	client := r.client.GetService().GetClient()

	resp, err := client.Post(lenovoUpdateFromRepositoryPath, lenovoBuildRepositoryUpdateRequestBody(parameters))
	if err != nil {
		// Any error after POST is treated as fatal: the request may have reached
		// the XCC and started executing (including dropped-response errors), so
		// retrying risks triggering a second update.
		return "", true, fmt.Errorf("failed to issue repository firmware update: %w", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		rawBody, _ := io.ReadAll(resp.Body)
		return "", true, fmt.Errorf("failed to start repository firmware update: %s, statusCode %d", string(rawBody), resp.StatusCode)
	}

	taskURI, err := lenovoExtractRepositoryTaskURI(resp)
	if err != nil {
		return "", true, fmt.Errorf("failed to extract task URI from repository firmware update response: %w", err)
	}

	return taskURI, false, nil
}

// GetRepositoryUpdateTask returns the task started by
// InstallFirmwareFromRepository. A completed task means the packages were
// applied; packages that need a host reboot are only staged until then. The
// XCC may restart while the task is running (e.g. when it updates itself), so
// errors returned here can be transient and should be retried.
func (r *LenovoRedfishBMC) GetRepositoryUpdateTask(ctx context.Context, taskURI string) (*schemas.Task, error) {
	return getUpgradeTask(ctx, r.RedfishBaseBMC, taskURI, r.lenovoParseTaskDetails)
}

// GetRepositoryUpdateDetail returns the packages processed by the last
// repository update run on the XCC, regardless of who started it (runs started
// by LXCA show up here too). Read it right after the own task has completed and
// check that the payloads belong to the own repository.
//
// BundleErrorCode and the per-package ret codes are not documented by Lenovo
// and are returned without interpretation. Observed on an SR650 V4 (XCC 28L-2.00):
// ret 2 for a package that was applied and activated by the next host reboot,
// ret 0 for packages whose version already matched the installed one.
func (r *LenovoRedfishBMC) GetRepositoryUpdateDetail(_ context.Context) (*LenovoRepositoryUpdateDetail, error) {
	client := r.client.GetService().GetClient()

	resp, err := client.Post(lenovoGetRepoUpdateDetailPath, map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to get repository update detail: %w", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GetRepoUpdateDetail response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get repository update detail: %s, statusCode %d", string(rawBody), resp.StatusCode)
	}

	detail := &LenovoRepositoryUpdateDetail{}
	if err := json.Unmarshal(rawBody, detail); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GetRepoUpdateDetail response: %w", err)
	}
	return detail, nil
}

// GetPendingFirmwareComponents returns the IDs of FirmwareInventory "-Pending"
// slots holding staged firmware that waits for activation by a host reboot.
// XCC/BMC and UEFI slots are ignored. Components without a "-Pending" slot
// (e.g. network adapters, "PCIeDevice_*.Bundle") keep reporting their old
// version until the reboot, so their staged updates are not visible here;
// use GetRepositoryUpdateDetail for those.
func (r *LenovoRedfishBMC) GetPendingFirmwareComponents(ctx context.Context) ([]string, error) {
	updateService, err := r.client.GetService().UpdateService()
	if err != nil {
		return nil, fmt.Errorf("failed to get UpdateService: %w", err)
	}
	firmwareInventory, err := updateService.FirmwareInventory()
	if err != nil {
		return nil, fmt.Errorf("failed to get FirmwareInventory: %w", err)
	}

	var pending []string
	for _, fw := range firmwareInventory {
		if !lenovoIsStagedRepositoryFirmware(fw) {
			continue
		}
		pending = append(pending, fw.ID)
	}
	return pending, nil
}

// lenovoIsStagedRepositoryFirmware reports whether fw is an XCC "-Pending" slot
// holding staged firmware. The slot exists for every component; after
// activation (host reboot) it is Disabled with an empty Version. Any other
// state with a Version is treated as staged, even if the Version equals the
// active one. XCC/BMC and UEFI slots are excluded.
func lenovoIsStagedRepositoryFirmware(fw *schemas.SoftwareInventory) bool {
	id := strings.ToUpper(fw.ID)
	if !strings.HasSuffix(id, lenovoPendingSlotSuffix) {
		return false
	}
	for _, prefix := range lenovoPendingExcludedPrefixes {
		if strings.HasPrefix(id, prefix) {
			return false
		}
	}
	return fw.Status.State != schemas.DisabledState && strings.TrimSpace(fw.Version) != ""
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"

	ctrl "sigs.k8s.io/controller-runtime"
)

var _ BMC = (*RedfishBMC)(nil)

const (
	// DefaultResourcePollingInterval is the default interval for polling resources.
	DefaultResourcePollingInterval = 30 * time.Second
	// DefaultResourcePollingTimeout is the default timeout for polling resources.
	DefaultResourcePollingTimeout = 5 * time.Minute
	// DefaultPowerPollingInterval is the default interval for polling power state.
	DefaultPowerPollingInterval = 30 * time.Second
	// DefaultPowerPollingTimeout is the default timeout for polling power state.
	DefaultPowerPollingTimeout = 5 * time.Minute
)

// Options contain the options for the BMC redfish client.
type Options struct {
	Endpoint  string
	Username  string
	Password  string
	BasicAuth bool

	ResourcePollingInterval time.Duration
	ResourcePollingTimeout  time.Duration
	PowerPollingInterval    time.Duration
	PowerPollingTimeout     time.Duration
}

// RedfishBMC is an implementation of the BMC interface for Redfish.
type RedfishBMC struct {
	client  *gofish.APIClient
	options Options
}

var pxeBootWithSettingUEFIBootMode = redfish.Boot{
	BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
	BootSourceOverrideMode:    redfish.UEFIBootSourceOverrideMode,
	BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
}
var pxeBootWithoutSettingUEFIBootMode = redfish.Boot{
	BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
	BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
}

// NewRedfishBMCClient creates a new RedfishBMC with the given connection details.
func NewRedfishBMCClient(
	ctx context.Context,
	options Options,
) (*RedfishBMC, error) {
	clientConfig := gofish.ClientConfig{
		Endpoint:         options.Endpoint,
		Username:         options.Username,
		Password:         options.Password,
		Insecure:         true,
		ReuseConnections: true,
		BasicAuth:        options.BasicAuth,
	}
	client, err := gofish.ConnectContext(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redfish endpoint: %w", err)
	}
	bmc := &RedfishBMC{client: client}
	if options.ResourcePollingInterval == 0 {
		options.ResourcePollingInterval = DefaultResourcePollingInterval
	}
	if options.ResourcePollingTimeout == 0 {
		options.ResourcePollingTimeout = DefaultResourcePollingTimeout
	}
	if options.PowerPollingInterval == 0 {
		options.PowerPollingInterval = DefaultPowerPollingInterval
	}
	if options.PowerPollingTimeout == 0 {
		options.PowerPollingTimeout = DefaultPowerPollingTimeout
	}
	bmc.options = options

	return bmc, nil
}

// Logout closes the BMC client connection by logging out
func (r *RedfishBMC) Logout() {
	if r.client != nil {
		r.client.Logout()
	}
}

// PowerOn powers on the system using Redfish.
func (r *RedfishBMC) PowerOn(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	powerState := system.PowerState
	if powerState != redfish.OnPowerState {
		if err := system.Reset(redfish.OnResetType); err != nil {
			return fmt.Errorf("failed to reset system to power on state: %w", err)
		}
	}
	return nil
}

// PowerOff gracefully shuts down the system using Redfish.
func (r *RedfishBMC) PowerOff(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if err := system.Reset(redfish.GracefulShutdownResetType); err != nil {
		return fmt.Errorf("failed to reset system to power on state: %w", err)
	}
	return nil
}

// ForcePowerOff powers off the system using Redfish.
func (r *RedfishBMC) ForcePowerOff(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if err := system.Reset(redfish.ForceOffResetType); err != nil {
		return fmt.Errorf("failed to reset system to power on state: %w", err)
	}
	return nil
}

// Reset performs a reset on the system using Redfish.
func (r *RedfishBMC) Reset(ctx context.Context, systemUUID string, resetType redfish.ResetType) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if err := system.Reset(resetType); err != nil {
		return fmt.Errorf("failed to reset system: %w", err)
	}
	return nil
}

// GetSystems get managed systems
func (r *RedfishBMC) GetSystems(ctx context.Context) ([]Server, error) {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return nil, fmt.Errorf("failed to get systems: %w", err)
	}
	servers := make([]Server, 0, len(systems))
	for _, s := range systems {
		servers = append(servers, Server{
			UUID:         s.UUID,
			Model:        s.Model,
			Manufacturer: s.Manufacturer,
			PowerState:   PowerState(s.PowerState),
			SerialNumber: s.SerialNumber,
		})
	}
	return servers, nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishBMC) SetPXEBootOnce(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	var setBoot redfish.Boot
	// TODO: cover setting BootSourceOverrideMode with BIOS settings profile
	if system.Boot.BootSourceOverrideMode != redfish.UEFIBootSourceOverrideMode {
		setBoot = pxeBootWithSettingUEFIBootMode
	} else {
		setBoot = pxeBootWithoutSettingUEFIBootMode
	}
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	return nil
}

func (r *RedfishBMC) GetManager(bmcUUID string) (*redfish.Manager, error) {
	if r.client == nil {
		return nil, fmt.Errorf("no client found")
	}
	managers, err := r.client.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("failed to get managers: %w", err)
	}
	if len(managers) == 0 {
		return nil, fmt.Errorf("zero managers found")
	}

	if len(bmcUUID) == 0 {
		// take the first one available
		return managers[0], nil
	}

	for _, m := range managers {
		if bmcUUID == m.UUID {
			return m, nil
		}
	}
	return nil, fmt.Errorf("matching managers not found for UUID %v", bmcUUID)
}

func (r *RedfishBMC) getOEMManager(bmcUUID string) (OEMManagerInterface, error) {
	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return nil, fmt.Errorf("not able to Manager %v", err)
	}

	// some vendors (like Dell) does not publich this. get through the system
	if manager.Manufacturer == "" {
		manufacturer, err := r.getSystemManufacturer()
		if err != nil {
			return nil, fmt.Errorf("not able to determine manufacturer: %v", err)
		}
		manager.Manufacturer = manufacturer
	}

	// togo: improve. as of now use first one similar to r.GetManager()
	oemManager, err := NewOEMManager(manager, r.client.Service)
	if err != nil {
		return nil, fmt.Errorf("not able create oem Manager: %v", err)
	}

	return oemManager, nil
}

func (r *RedfishBMC) ResetManager(ctx context.Context, bmcUUID string, resetType redfish.ResetType) error {

	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return fmt.Errorf("failed to get managers: %w", err)
	}
	if len(manager.SupportedResetTypes) > 0 && !slices.Contains(manager.SupportedResetTypes, resetType) {
		return fmt.Errorf("reset type of %v is not supported for manager %v", resetType, manager.UUID)
	}

	err = manager.Reset(resetType)
	if err != nil {
		return fmt.Errorf("failed to reset managers %v with error: %w", manager.UUID, err)
	}
	return nil
}

// GetSystemInfo retrieves information about the system using Redfish.
func (r *RedfishBMC) GetSystemInfo(ctx context.Context, systemUUID string) (SystemInfo, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get systems: %w", err)
	}

	memoryString := fmt.Sprintf("%.fGi", system.MemorySummary.TotalSystemMemoryGiB)
	quantity, err := resource.ParseQuantity(memoryString)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to parse memory quantity: %w", err)
	}

	systemProcessors, err := system.Processors()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get processors: %w", err)
	}

	processors := make([]Processor, 0, len(systemProcessors))
	for _, p := range systemProcessors {
		processors = append(processors, Processor{
			ID:             p.ID,
			Type:           string(p.ProcessorType),
			Architecture:   string(p.ProcessorArchitecture),
			InstructionSet: string(p.InstructionSet),
			Manufacturer:   p.Manufacturer,
			Model:          p.Model,
			MaxSpeedMHz:    int32(p.MaxSpeedMHz),
			TotalCores:     int32(p.TotalCores),
			TotalThreads:   int32(p.TotalThreads),
		})
	}

	return SystemInfo{
		SystemUUID:        system.UUID,
		Manufacturer:      system.Manufacturer,
		Model:             system.Model,
		Status:            system.Status,
		PowerState:        system.PowerState,
		SerialNumber:      system.SerialNumber,
		SKU:               system.SKU,
		IndicatorLED:      string(system.IndicatorLED),
		TotalSystemMemory: quantity,
		Processors:        processors,
	}, nil
}

func (r *RedfishBMC) GetBootOrder(ctx context.Context, systemUUID string) ([]string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return []string{}, err
	}
	return system.Boot.BootOrder, nil
}

func (r *RedfishBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return "", err
	}
	return system.BIOSVersion, nil
}

func (r *RedfishBMC) GetBMCVersion(ctx context.Context, bmcUUID string) (string, error) {
	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return "", err
	}
	return manager.FirmwareVersion, nil
}

func (r *RedfishBMC) GetBiosAttributeValues(
	ctx context.Context,
	systemUUID string,
	attributes []string,
) (
	result redfish.SettingsAttributes,
	err error,
) {
	if len(attributes) == 0 {
		return result, err
	}
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return result, err
	}
	bios, err := system.Bios()
	if err != nil {
		return result, err
	}
	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return result, err
	}
	result = make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			result[name] = bios.Attributes[name]
		}
	}
	return result, err
}

func (r *RedfishBMC) GetBMCAttributeValues(
	ctx context.Context,
	bmcUUID string,
	attributes []string,
) (
	result redfish.SettingsAttributes,
	err error,
) {
	if len(attributes) == 0 {
		return nil, nil
	}
	oemManager, err := r.getOEMManager(bmcUUID)
	if err != nil {
		return nil, err
	}

	return oemManager.GetOEMBMCSettingAttribute(attributes)
}

func (r *RedfishBMC) GetBiosPendingAttributeValues(
	ctx context.Context,
	systemUUID string,
) (
	result redfish.SettingsAttributes,
	err error,
) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return result, err
	}

	var tSys struct {
		Bios        common.Link
		BiosVersion string
	}

	err = json.Unmarshal(system.RawData, &tSys)
	if err != nil {
		return result, err
	}

	var tBios struct {
		Attributes redfish.SettingsAttributes `json:"Attributes"`
		Settings   common.Settings            `json:"@Redfish.Settings"`
	}
	err = r.GetEntityFromUri(tSys.Bios.String(), system.GetClient(), &tBios)
	if err != nil {
		return result, err
	}

	var tBiosPendingSetting struct {
		Attributes redfish.SettingsAttributes `json:"Attributes"`
	}
	err = r.GetEntityFromUri(tBios.Settings.SettingsObject.String(), system.GetClient(), &tBiosPendingSetting)
	if err != nil {
		return result, err
	}

	// unfortunately, some vendors fill the pending attribute with copy of actual bios attribute
	// remove if there are the same
	if len(tBios.Attributes) == len(tBiosPendingSetting.Attributes) {
		pendingAttr := redfish.SettingsAttributes{}
		for key, attr := range tBiosPendingSetting.Attributes {
			if value, ok := tBios.Attributes[key]; !ok || value != attr {
				pendingAttr[key] = attr
			}
		}
		return pendingAttr, nil
	}

	return tBiosPendingSetting.Attributes, nil
}

func (r *RedfishBMC) GetEntityFromUri(uri string, client common.Client, entity any) error {
	Resp, err := client.Get(uri)
	if err != nil {
		return err
	}
	defer Resp.Body.Close() // nolint: errcheck

	RespRawBody, err := io.ReadAll(Resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(RespRawBody, &entity)
}

func (r *RedfishBMC) GetBMCPendingAttributeValues(
	ctx context.Context,
	bmcUUID string,
) (
	result redfish.SettingsAttributes,
	err error,
) {
	oemManager, err := r.getOEMManager(bmcUUID)
	if err != nil {
		return nil, err
	}

	return oemManager.GetBMCPendingAttributeValues()
}

// SetBiosAttributesOnReset sets given bios attributes.
func (r *RedfishBMC) SetBiosAttributesOnReset(
	ctx context.Context,
	systemUUID string,
	attributes redfish.SettingsAttributes,
) (err error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return
	}
	bios, err := system.Bios()
	if err != nil {
		return
	}

	attrs := make(redfish.SettingsAttributes, len(attributes))
	for name, value := range attributes {
		attrs[name] = value
	}
	return bios.UpdateBiosAttributesApplyAt(attrs, common.OnResetApplyTime)
}

func (r *RedfishBMC) SetBMCAttributesImediately(
	ctx context.Context,
	bmcUUID string,
	attributes redfish.SettingsAttributes,
) (err error) {
	if len(attributes) == 0 {
		return nil
	}
	oemManager, err := r.getOEMManager(bmcUUID)
	if err != nil {
		return err
	}
	return oemManager.UpdateBMCAttributesApplyAt(attributes, common.ImmediateApplyTime)
}

// SetBootOrder sets bios boot order
func (r *RedfishBMC) SetBootOrder(ctx context.Context, systemUUID string, bootOrder []string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return err
	}
	return system.SetBoot(
		redfish.Boot{
			BootSourceOverrideEnabled: redfish.ContinuousBootSourceOverrideEnabled,
			BootSourceOverrideTarget:  redfish.NoneBootSourceOverrideTarget,
			BootOrder:                 bootOrder,
		},
	)
}

func (r *RedfishBMC) getFilteredBiosRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	filtered map[string]RegistryEntryAttributes,
	err error,
) {
	registries, err := r.client.Service.Registries()
	biosRegistry := &Registry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "BiosAttributeRegistry") {
			err = registry.Get(r.client, registry.Location[0].URI, biosRegistry)
			if err != nil {
				return
			}
		}
	}
	// filter out immutable, readonly and hidden attributes
	filtered = make(map[string]RegistryEntryAttributes)
	for _, entry := range biosRegistry.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filtered[entry.AttributeName] = entry
		}
	}
	return
}

// CheckBiosAttributes checks if the attributes need to reboot when changed and are the correct type.
func (r *RedfishBMC) CheckBiosAttributes(attrs redfish.SettingsAttributes) (reset bool, err error) {
	reset = false
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return reset, err
	}
	return r.checkAttribues(attrs, filtered)
}

func (r *RedfishBMC) checkAttribues(
	attrs redfish.SettingsAttributes,
	filtered map[string]RegistryEntryAttributes,
) (reset bool, err error) {
	reset = false
	var errs []error
	//TODO: add more types like maps and Enumerations
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			errs = append(errs, fmt.Errorf("attribute %s not found or immutable/hidden", name))
			continue
		}
		if entryAttribute.ResetRequired {
			reset = true
		}
		switch strings.ToLower(entryAttribute.Type) {
		case "integer":
			if _, ok := value.(int); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case "string":
			if _, ok := value.(string); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
			}
		case "enumeration":
			if _, ok := value.(string); !ok {
				errs = append(
					errs,
					fmt.Errorf(
						"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
						name,
						value,
						entryAttribute.Type,
						entryAttribute,
					))
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
			errs = append(
				errs,
				fmt.Errorf(
					"attribute '%s's' value '%v' has wrong type. needed '%s' for '%v'",
					name,
					value,
					entryAttribute.Type,
					entryAttribute,
				))
		}
	}
	return reset, errors.Join(errs...)
}

func (r *RedfishBMC) getSystemManufacturer() (string, error) {
	systems, err := r.client.Service.Systems()
	if err != nil {
		return "", err
	}
	if len(systems) > 0 {
		return systems[0].Manufacturer, nil
	}

	return "", fmt.Errorf("no system found to determine the Manufacturer")
}

// check if the arrtibutes need to reboot when changed, and are correct type.
// supported attrType, bmc and bios
func (r *RedfishBMC) CheckBMCAttributes(bmcUUID string, attrs redfish.SettingsAttributes) (reset bool, err error) {
	oemManager, err := r.getOEMManager(bmcUUID)
	if err != nil {
		return false, err
	}

	return oemManager.CheckBMCAttributes(attrs)
}

func (r *RedfishBMC) GetStorages(ctx context.Context, systemUUID string) ([]Storage, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return nil, err
	}
	var systemStorage []*redfish.Storage
	err = wait.PollUntilContextTimeout(
		ctx,
		r.options.ResourcePollingInterval,
		r.options.ResourcePollingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			systemStorage, err = system.Storage()
			if err != nil {
				return false, nil
			}
			return true, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to wait for for server storages to be ready: %w", err)
	}
	result := make([]Storage, 0, len(systemStorage))
	for _, s := range systemStorage {
		storage := Storage{
			Entity: Entity{ID: s.ID, Name: s.Name},
		}
		volumes, err := s.Volumes()
		if err != nil {
			return nil, err
		}
		storage.Volumes = make([]Volume, 0, len(volumes))
		for _, v := range volumes {
			storage.Volumes = append(storage.Volumes, Volume{
				Entity:    Entity{ID: v.ID, Name: v.Name},
				SizeBytes: int64(v.CapacityBytes),
				RAIDType:  v.RAIDType,
				State:     v.Status.State,
			})
		}
		drives, err := s.Drives()
		if err != nil {
			return nil, err
		}
		storage.Drives = make([]Drive, 0, len(drives))
		for _, d := range drives {
			storage.Drives = append(storage.Drives, Drive{
				Entity:    Entity{ID: d.ID, Name: d.Name},
				MediaType: string(d.MediaType),
				Type:      d.DriveFormFactor,
				SizeBytes: d.CapacityBytes,
				Vendor:    d.Manufacturer,
				Model:     d.Model,
				State:     d.Status.State,
			})
		}
		result = append(result, storage)
	}
	if len(result) == 0 {
		// if no storage is found, fall back to simpleStorage (outdated storage API)
		simpleStorages, err := system.SimpleStorages()
		result = make([]Storage, 0, len(systemStorage))
		if err != nil {
			return nil, err
		}
		for _, s := range simpleStorages {
			storage := Storage{
				Entity: Entity{ID: s.ID, Name: s.Name},
			}

			storage.Drives = make([]Drive, 0, len(s.Devices))
			for _, d := range s.Devices {
				storage.Drives = append(storage.Drives, Drive{
					Entity:    Entity{Name: d.Name},
					SizeBytes: d.CapacityBytes,
					Vendor:    d.Manufacturer,
					Model:     d.Model,
					State:     d.Status.State,
				})
			}
			result = append(result, storage)
		}
		return result, nil
	}
	return result, nil
}

func (r *RedfishBMC) CreateOrUpdateAccount(
	ctx context.Context, userName,
	role, password string, enabled bool,
) error {
	service, err := r.client.GetService().AccountService()
	if err != nil {
		return fmt.Errorf("failed to get account service: %w", err)
	}
	accounts, err := service.Accounts()
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}
	//log.V(1).Info("Accounts", "accounts", accounts)
	for _, a := range accounts {
		if a.UserName == userName {
			a.RoleID = role
			a.UserName = userName
			a.Enabled = enabled
			if err := a.Update(); err != nil {
				return fmt.Errorf("failed to update account: %w", err)
			}
			if password != "" {
				if err := a.ChangePassword(password, r.options.Password); err != nil {
					return fmt.Errorf("failed to change account password: %w", err)
				}
			}
		}
	}
	_, err = service.CreateAccount(userName, password, role)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}
	return nil
}

func (r *RedfishBMC) SetAccountPassword(ctx context.Context, accountName, password string) error {
	service, err := r.client.GetService().AccountService()
	if err != nil {
		return fmt.Errorf("failed to get account service: %w", err)
	}
	accounts, err := service.Accounts()
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}
	if len(accounts) == 0 {
		return errors.New("no account found")
	}
	for _, a := range accounts {
		if a.Name == accountName {
			return a.ChangePassword(password, r.options.Password)
		}
	}
	return errors.New("account not found")
}

func (r *RedfishBMC) getSystemByUUID(ctx context.Context, systemUUID string) (*redfish.ComputerSystem, error) {
	service := r.client.GetService()
	var systems []*redfish.ComputerSystem
	err := wait.PollUntilContextTimeout(
		ctx,
		r.options.ResourcePollingInterval,
		r.options.ResourcePollingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var err error
			systems, err = service.Systems()
			return err == nil, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to wait for for server systems to be ready: %w", err)
	}
	for _, system := range systems {
		if strings.ToLower(system.UUID) == systemUUID {
			return system, nil
		}
	}
	return nil, fmt.Errorf("no system found for %v", systemUUID)
}

func (r *RedfishBMC) WaitForServerPowerState(
	ctx context.Context,
	systemUUID string,
	powerState redfish.PowerState,
) error {
	if err := wait.PollUntilContextTimeout(
		ctx,
		r.options.PowerPollingInterval,
		r.options.PowerPollingTimeout,
		true,
		func(ctx context.Context) (done bool, err error) {
			sysInfo, err := r.getSystemByUUID(ctx, systemUUID)
			if err != nil {
				return false, fmt.Errorf("failed to get system info: %w", err)
			}
			return sysInfo.PowerState == powerState, nil
		}); err != nil {
		return fmt.Errorf("failed to wait for for server power state: %w", err)
	}
	return nil
}

// UpgradeBiosVersion upgrade given bios versions.
func (r *RedfishBMC) UpgradeBiosVersion(
	ctx context.Context,
	manufacturer string,
	parameters *redfish.SimpleUpdateParameters,
) (string, bool, error) {
	log := ctrl.LoggerFrom(ctx)
	fatal := false
	service := r.client.GetService()

	upgradeServices, err := service.UpdateService()
	if err != nil {
		return "", fatal, err
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

	err = json.Unmarshal(upgradeServices.RawData, &tUS)
	if err != nil {
		return "", fatal, err
	}

	oem, err := NewOEM(manufacturer, service)
	if err != nil {
		return "", fatal, err
	}

	RequestBody := oem.GetUpdateRequestBody(parameters)

	resp, err := upgradeServices.PostWithResponse(tUS.Actions.SimpleUpdate.Target, &RequestBody)

	if err != nil {
		return "", fatal, err
	}
	defer resp.Body.Close() // nolint: errcheck

	// any error post this point is fatal, as we can not issue multiple upgrade requests.
	// expectation is to move to failed state, and manually check the status before retrying
	fatal = true

	log.V(1).Info("update has been issued", "Response status code", resp.StatusCode)

	if resp.StatusCode != http.StatusAccepted {
		biosRawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "",
				fatal,
				fmt.Errorf("failed to accept the upgrade request. and read the response body %v, statusCode %v",
					err,
					resp.StatusCode,
				)
		}
		return "",
			fatal,
			fmt.Errorf("failed to accept the upgrade request %v, statusCode %v",
				string(biosRawBody),
				resp.StatusCode,
			)
	}

	taskMonitorURI, err := oem.GetUpdateTaskMonitorURI(resp)
	if err != nil {
		log.V(1).Error(err,
			"failed to extract Task created for upgrade. However, upgrade might be running on server.")
		return "", fatal, fmt.Errorf("failed to read task monitor URI. %v", err)
	}

	log.V(1).Info("update has been accepted.", "Response", taskMonitorURI)

	return taskMonitorURI, false, nil
}

func (r *RedfishBMC) GetBiosUpgradeTask(
	ctx context.Context,
	manufacturer string,
	taskURI string,
) (*redfish.Task, error) {
	respTask, err := r.client.GetService().GetClient().Get(taskURI)
	if err != nil {
		return nil, err
	}
	defer respTask.Body.Close() // nolint: errcheck

	if respTask.StatusCode != http.StatusAccepted && respTask.StatusCode != http.StatusOK {
		respTaskRawBody, err := io.ReadAll(respTask.Body)
		if err != nil {
			return nil,
				fmt.Errorf("failed to get the upgrade Task details. and read the response body %v, statusCode %v",
					err,
					respTask.StatusCode)
		}
		return nil,
			fmt.Errorf("failed to get the upgrade Task details. %v, statusCode %v",
				string(respTaskRawBody),
				respTask.StatusCode)
	}

	oem, err := NewOEM(manufacturer, r.client.GetService())
	if err != nil {
		return nil, fmt.Errorf("failed to get oem object, %v", err)
	}

	return oem.GetTaskMonitorDetails(ctx, respTask)
}

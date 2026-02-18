// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"

	ctrl "sigs.k8s.io/controller-runtime"
)

// RedfishBaseBMC implements all standard Redfish BMC methods.
// Vendor-specific structs embed this and override methods as needed.
var _ BMC = (*RedfishBaseBMC)(nil)

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

// RedfishBaseBMC is the base implementation of the BMC interface for Redfish.
// Vendor-specific structs embed this and override methods as needed.
type RedfishBaseBMC struct {
	client       *gofish.APIClient
	options      Options
	manufacturer string
}

var pxeBootWithSettingUEFIBootMode = schemas.Boot{
	BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
	BootSourceOverrideMode:    schemas.UEFIBootSourceOverrideMode,
	BootSourceOverrideTarget:  schemas.PxeBootSource,
}
var pxeBootWithoutSettingUEFIBootMode = schemas.Boot{
	BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
	BootSourceOverrideTarget:  schemas.PxeBootSource,
}

type InvalidBIOSSettingsError struct {
	SettingName  string
	SettingValue any
	Message      string
}

func (e *InvalidBIOSSettingsError) Error() string {
	return fmt.Sprintf("Settings Name: %s\nSettings Value: %v\nError: %s", e.SettingName, e.SettingValue, e.Message)
}

// newRedfishBaseBMCClient creates a new RedfishBaseBMC with the given connection details (internal use only).
func newRedfishBaseBMCClient(ctx context.Context, options Options) (*RedfishBaseBMC, error) {
	clientConfig := gofish.ClientConfig{
		Endpoint:  options.Endpoint,
		Username:  options.Username,
		Password:  options.Password,
		Insecure:  true,
		BasicAuth: options.BasicAuth,
	}
	client, err := gofish.ConnectContext(ctx, clientConfig)
	if err != nil {
		return nil, err
	}
	bmc := &RedfishBaseBMC{client: client}
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

// NewRedfishBMCClient creates a vendor-specific BMC client by connecting to the
// Redfish endpoint, detecting the manufacturer, and returning the appropriate
// vendor-specific struct. The returned BMC interface implementation will have
// vendor-specific method overrides where needed.
func NewRedfishBMCClient(ctx context.Context, options Options) (BMC, error) {
	base, err := newRedfishBaseBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}

	manufacturer, err := base.getSystemManufacturer()
	if err != nil {
		// If we can't determine the manufacturer (e.g. no systems yet during
		// endpoint discovery), fall back to the base implementation.
		return base, nil
	}
	base.manufacturer = manufacturer

	switch Manufacturer(manufacturer) {
	case ManufacturerDell:
		return &DellRedfishBMC{RedfishBaseBMC: base}, nil
	case ManufacturerHPE:
		return &HPERedfishBMC{RedfishBaseBMC: base}, nil
	case ManufacturerLenovo:
		return &LenovoRedfishBMC{RedfishBaseBMC: base}, nil
	case ManufacturerSupermicro:
		return &SupermicroRedfishBMC{RedfishBaseBMC: base}, nil
	default:
		return base, nil
	}
}

// Logout closes the BMC client connection by logging out
func (r *RedfishBaseBMC) Logout() {
	if r.client != nil {
		r.client.Logout()
	}
}

// PowerOn powers on the system using Redfish.
func (r *RedfishBaseBMC) PowerOn(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	powerState := system.PowerState
	if powerState != schemas.OnPowerState {
		if _, err := system.Reset(schemas.OnResetType); err != nil {
			return fmt.Errorf("failed to reset system to power on state: %w", err)
		}
	}
	return nil
}

// PowerOff gracefully shuts down the system using Redfish.
func (r *RedfishBaseBMC) PowerOff(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if _, err := system.Reset(schemas.GracefulShutdownResetType); err != nil {
		return fmt.Errorf("failed to reset system to power off state: %w", err)
	}
	return nil
}

// ForcePowerOff powers off the system using Redfish.
func (r *RedfishBaseBMC) ForcePowerOff(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if _, err := system.Reset(schemas.ForceOffResetType); err != nil {
		return fmt.Errorf("failed to reset system to force power off state: %w", err)
	}
	return nil
}

// Reset performs a reset on the system using Redfish.
func (r *RedfishBaseBMC) Reset(ctx context.Context, systemURI string, resetType redfish.ResetType) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if _, err := system.Reset(resetType); err != nil {
		return fmt.Errorf("failed to reset system: %w", err)
	}
	return nil
}

// GetSystems get managed systems
func (r *RedfishBaseBMC) GetSystems(ctx context.Context) ([]Server, error) {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return nil, fmt.Errorf("failed to get systems: %w", err)
	}
	servers := make([]Server, 0, len(systems))
	for _, s := range systems {
		servers = append(servers, Server{
			UUID:         s.UUID,
			URI:          s.ODataID,
			Model:        s.Model,
			Manufacturer: s.Manufacturer,
			PowerState:   s.PowerState,
			SerialNumber: s.SerialNumber,
		})
	}
	return servers, nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishBaseBMC) SetPXEBootOnce(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	var setBoot schemas.Boot
	// TODO: cover setting BootSourceOverrideMode with BIOS settings profile
	// Only skip setting BootSourceOverrideMode for older BMCs that don't report it
	if system.Boot.BootSourceOverrideMode != "" && system.Boot.BootSourceOverrideMode != schemas.UEFIBootSourceOverrideMode {
		setBoot = pxeBootWithSettingUEFIBootMode
	} else {
		setBoot = pxeBootWithoutSettingUEFIBootMode
	}

	// TODO: pass logging context from caller
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Setting PXE boot once", "SystemURI", systemURI, "Boot settings", setBoot)
	if err := system.SetBoot(&setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	return nil
}

func (r *RedfishBaseBMC) GetManager(bmcUUID string) (*schemas.Manager, error) {
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

func (r *RedfishBaseBMC) ResetManager(ctx context.Context, bmcUUID string, resetType schemas.ResetType) error {
	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return fmt.Errorf("failed to get managers: %w", err)
	}
	if len(manager.SupportedResetTypes) > 0 && !slices.Contains(manager.SupportedResetTypes, resetType) {
		return fmt.Errorf("reset type of %v is not supported for manager %v", resetType, manager.UUID)
	}

	if _, err = manager.Reset(resetType); err != nil {
		return fmt.Errorf("failed to reset managers %v with error: %w", manager.UUID, err)
	}
	return nil
}

// GetSystemInfo retrieves information about the system using Redfish.
func (r *RedfishBaseBMC) GetSystemInfo(ctx context.Context, systemURI string) (SystemInfo, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get systems: %w", err)
	}

	memoryString := fmt.Sprintf("%.fGi", gofish.Deref(system.MemorySummary.TotalSystemMemoryGiB))
	quantity, err := resource.ParseQuantity(memoryString)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to parse memory quantity: %w", err)
	}

	return SystemInfo{
		SystemUUID:        system.UUID,
		SystemURI:         system.ODataID,
		Manufacturer:      system.Manufacturer,
		Model:             system.Model,
		Status:            system.Status,
		PowerState:        system.PowerState,
		SerialNumber:      system.SerialNumber,
		SKU:               system.SKU,
		IndicatorLED:      string(system.IndicatorLED), //nolint:staticcheck
		TotalSystemMemory: quantity,
	}, nil
}

func (r *RedfishBaseBMC) GetProcessors(ctx context.Context, systemURI string) ([]Processor, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return nil, fmt.Errorf("failed to get systems: %w", err)
	}
	systemProcessors, err := system.Processors()
	if err != nil {
		return nil, fmt.Errorf("failed to get processors: %w", err)
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
			MaxSpeedMHz:    int32(gofish.Deref(p.MaxSpeedMHz)),
			TotalCores:     int32(gofish.Deref(p.TotalCores)),
			TotalThreads:   int32(gofish.Deref(p.TotalThreads)),
		})
	}
	return processors, nil
}

func (r *RedfishBaseBMC) GetBootOrder(ctx context.Context, systemURI string) ([]string, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return []string{}, err
	}
	return system.Boot.BootOrder, nil
}

func (r *RedfishBaseBMC) GetBiosVersion(ctx context.Context, systemURI string) (string, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return "", err
	}
	return system.BiosVersion, nil
}

func (r *RedfishBaseBMC) GetBMCVersion(ctx context.Context, bmcUUID string) (string, error) {
	manager, err := r.GetManager(bmcUUID)
	if err != nil {
		return "", err
	}
	return manager.FirmwareVersion, nil
}

func (r *RedfishBaseBMC) GetBiosAttributeValues(ctx context.Context, systemURI string, attributes []string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return nil, err
	}
	bios, err := system.Bios()
	if err != nil {
		return nil, err
	}
	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(schemas.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			result[name] = bios.Attributes[name]
		}
	}
	return result, err
}

func (r *RedfishBaseBMC) GetBMCAttributeValues(_ context.Context, _ string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("BMC attribute operations not supported for manufacturer %q", r.manufacturer)
}

func (r *RedfishBaseBMC) GetBiosPendingAttributeValues(ctx context.Context, systemURI string) (schemas.SettingsAttributes, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return nil, err
	}

	var tSys struct {
		Bios        schemas.Link
		BiosVersion string
	}

	err = json.Unmarshal(system.RawData, &tSys)
	if err != nil {
		return nil, err
	}

	var tBios struct {
		Attributes schemas.SettingsAttributes `json:"Attributes"`
		Settings   schemas.Settings           `json:"@Redfish.Settings"`
	}
	if err = r.GetEntityFromUri(ctx, tSys.Bios.String(), system.GetClient(), &tBios); err != nil {
		return nil, err
	}

	var tBiosPendingSetting struct {
		Attributes schemas.SettingsAttributes `json:"Attributes"`
	}
	if err = r.GetEntityFromUri(ctx, tBios.Settings.SettingsObject, system.GetClient(), &tBiosPendingSetting); err != nil {
		return nil, err
	}

	// unfortunately, some vendors fill the pending attribute with copy of actual bios attribute
	// remove if there are the same
	if len(tBios.Attributes) == len(tBiosPendingSetting.Attributes) {
		pendingAttr := schemas.SettingsAttributes{}
		for key, attr := range tBiosPendingSetting.Attributes {
			if value, ok := tBios.Attributes[key]; !ok || value != attr {
				pendingAttr[key] = attr
			}
		}
		return pendingAttr, nil
	}

	return tBiosPendingSetting.Attributes, nil
}

func (r *RedfishBaseBMC) GetEntityFromUri(ctx context.Context, uri string, client schemas.Client, entity any) error {
	log := ctrl.LoggerFrom(ctx)

	resp, err := client.Get(uri)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		if err = Body.Close(); err != nil {
			log.Error(err, "failed to close response body")
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, &entity)
}

func (r *RedfishBaseBMC) GetBMCPendingAttributeValues(_ context.Context, _ string) (schemas.SettingsAttributes, error) {
	return nil, fmt.Errorf("BMC pending attribute operations not supported for manufacturer %q", r.manufacturer)
}

// SetBiosAttributesOnReset sets given bios attributes.
func (r *RedfishBaseBMC) SetBiosAttributesOnReset(ctx context.Context, systemURI string, attributes schemas.SettingsAttributes) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return err
	}
	bios, err := system.Bios()
	if err != nil {
		return err
	}

	attrs := make(schemas.SettingsAttributes, len(attributes))
	maps.Copy(attrs, attributes)
	return bios.UpdateBiosAttributesApplyAt(attrs, schemas.OnResetSettingsApplyTime)
}

func (r *RedfishBaseBMC) SetBMCAttributesImmediately(_ context.Context, _ string, attributes schemas.SettingsAttributes) error {
	if len(attributes) == 0 {
		return nil
	}
	return fmt.Errorf("BMC attribute operations not supported for manufacturer %q", r.manufacturer)
}

// SetBootOrder sets bios boot order
func (r *RedfishBaseBMC) SetBootOrder(ctx context.Context, systemURI string, bootOrder []string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return err
	}
	return system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.NoneBootSource,
		BootOrder:                 bootOrder,
	},
	)
}

func (r *RedfishBaseBMC) getFilteredBiosRegistryAttributes(readOnly bool, immutable bool) (map[string]RegistryEntryAttributes, error) {
	registries, err := r.client.Service.Registries()
	if err != nil {
		return nil, err
	}
	biosRegistry := &Registry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "BiosAttributeRegistry") {
			if err := registry.Get(r.client, registry.Location[0].URI, biosRegistry); err != nil {
				return nil, err
			}
		}
	}
	// filter out immutable, readonly and hidden attributes
	filtered := make(map[string]RegistryEntryAttributes)
	for _, entry := range biosRegistry.RegistryEntries.Attributes {
		if entry.Immutable == immutable && entry.ReadOnly == readOnly && !entry.Hidden {
			filtered[entry.AttributeName] = entry
		}
	}
	return filtered, nil
}

// CheckBiosAttributes checks if the attributes need to reboot when changed and are the correct type.
func (r *RedfishBaseBMC) CheckBiosAttributes(attrs schemas.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return false, err
	}
	return r.checkAttributes(attrs, filtered)
}

func (r *RedfishBaseBMC) checkAttributes(attrs schemas.SettingsAttributes, filtered map[string]RegistryEntryAttributes) (bool, error) {
	reset := false
	var errs []error
	// TODO: add more types like maps and Enumerations
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			err := &InvalidBIOSSettingsError{
				SettingName:  name,
				SettingValue: value,
				Message:      "attribute not found or is immutable/hidden",
			}
			errs = append(errs, err)
			continue
		}
		// if ResetRequired is nil, assume true
		if entryAttribute.ResetRequired == nil || *entryAttribute.ResetRequired {
			reset = true
		}
		switch strings.ToLower(entryAttribute.Type) {
		case "integer":
			if _, ok := value.(int); !ok {
				err := &InvalidBIOSSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
			}
		case "string":
			if _, ok := value.(string); !ok {
				err := &InvalidBIOSSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
			}
		case "enumeration":
			if _, ok := value.(string); !ok {
				err := &InvalidBIOSSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type (Non String). needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
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
				err := &InvalidBIOSSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message:      fmt.Sprintf("attributes value is unknown. Valid Attributes %v", entryAttribute.Value),
				}
				errs = append(errs, err)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				err := &InvalidBIOSSettingsError{
					SettingName:  name,
					SettingValue: value,
					Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
						entryAttribute.Type,
					),
				}
				errs = append(errs, err)
			}
		default:
			err := &InvalidBIOSSettingsError{
				SettingName:  name,
				SettingValue: value,
				Message: fmt.Sprintf("attribute value has wrong type. needed '%s'",
					entryAttribute.Type,
				),
			}
			errs = append(errs, err)
		}
	}
	return reset, errors.Join(errs...)
}

func (r *RedfishBaseBMC) CheckBMCAttributes(_ context.Context, _ string, _ redfish.SettingsAttributes) (bool, error) {
	return false, fmt.Errorf("BMC attribute checking not supported for manufacturer %q", r.manufacturer)
}

func (r *RedfishBaseBMC) getSystemManufacturer() (string, error) {
	systems, err := r.client.Service.Systems()
	if err != nil {
		return "", err
	}
	if len(systems) > 0 {
		return systems[0].Manufacturer, nil
	}

	return "", fmt.Errorf("no system found to determine the Manufacturer")
}

func (r *RedfishBaseBMC) GetStorages(ctx context.Context, systemURI string) ([]Storage, error) {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return nil, err
	}
	var systemStorage []*schemas.Storage
	err = wait.PollUntilContextTimeout(
		ctx,
		r.options.ResourcePollingInterval,
		r.options.ResourcePollingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			systemStorage, err = system.Storage()
			if err != nil {
				log := ctrl.LoggerFrom(ctx)
				log.V(1).Info("Storage not ready yet", "error", err)
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
				SizeBytes: int64(gofish.Deref(v.CapacityBytes)),
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
				SizeBytes: int64(gofish.Deref(d.CapacityBytes)),
				Vendor:    d.Manufacturer,
				Model:     d.Model,
				State:     d.Status.State,
			})
		}
		result = append(result, storage)
	}
	if len(result) == 0 {
		// if no storage is found, fall back to simpleStorage (outdated storage API)
		simpleStorages, err := system.SimpleStorage()
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
					SizeBytes: int64(gofish.Deref(d.CapacityBytes)),
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

func (r *RedfishBaseBMC) CreateOrUpdateAccount(
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
	for _, a := range accounts {
		if a.UserName == userName {
			a.RoleID = role
			a.UserName = userName
			a.Enabled = enabled
			if err := a.Update(); err != nil {
				return fmt.Errorf("failed to update account: %w", err)
			}
			if password != "" {
				if _, err := a.ChangePassword(password, r.options.Password); err != nil {
					return fmt.Errorf("failed to change account password: %w", err)
				}
			}
			return nil
		}
	}
	_, err = service.CreateAccount(userName, password, role)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}
	return nil
}

func (r *RedfishBaseBMC) DeleteAccount(ctx context.Context, userName, id string) error {
	log := ctrl.LoggerFrom(ctx)
	service, err := r.client.GetService().AccountService()
	if err != nil {
		return fmt.Errorf("failed to get account service: %w", err)
	}
	accounts, err := service.Accounts()
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}
	for _, a := range accounts {
		// make sure we delete the correct account
		if a.UserName == userName && a.ID == id {
			resp, err := r.client.Delete(a.ODataID)
			if err != nil {
				return err
			}
			if err = resp.Body.Close(); err != nil {
				log.Error(err, "failed to close response body")
			}
			return nil
		}
	}
	return fmt.Errorf("account %s not found", userName)
}

func (r *RedfishBaseBMC) GetAccountService() (*schemas.AccountService, error) {
	service, err := r.client.GetService().AccountService()
	if err != nil {
		return nil, fmt.Errorf("failed to get account service: %w", err)
	}
	return service, nil
}

func (r *RedfishBaseBMC) GetAccounts() ([]*schemas.ManagerAccount, error) {
	service, err := r.client.GetService().AccountService()
	if err != nil {
		return nil, fmt.Errorf("failed to get account service: %w", err)
	}
	accounts, err := service.Accounts()
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}
	return accounts, nil
}

func (r *RedfishBaseBMC) getSystemFromUri(ctx context.Context, systemURI string) (*schemas.ComputerSystem, error) {
	if len(systemURI) == 0 {
		return nil, fmt.Errorf("can not process empty URI")
	}
	var system *schemas.ComputerSystem
	if err := wait.PollUntilContextTimeout(
		ctx,
		r.options.ResourcePollingInterval,
		r.options.ResourcePollingTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			var err error
			system, err = schemas.GetObject[schemas.ComputerSystem](r.client, systemURI)
			return err == nil, nil
		}); err != nil {
		return nil, fmt.Errorf("failed to wait for for server systems to be ready: %w", err)
	}
	if system.UUID != "" {
		return system, nil
	}
	return nil, fmt.Errorf("no system found for %v", systemURI)
}

func (r *RedfishBaseBMC) WaitForServerPowerState(ctx context.Context, systemURI string, powerState schemas.PowerState) error {
	if err := wait.PollUntilContextTimeout(
		ctx,
		r.options.PowerPollingInterval,
		r.options.PowerPollingTimeout,
		true,
		func(ctx context.Context) (done bool, err error) {
			sysInfo, err := r.getSystemFromUri(ctx, systemURI)
			if err != nil {
				return false, fmt.Errorf("failed to get system info: %w", err)
			}
			return sysInfo.PowerState == powerState, nil
		}); err != nil {
		return fmt.Errorf("failed to wait for for server power state: %w", err)
	}
	return nil
}

// UpgradeBiosVersion is a fallback for unknown vendors. Vendor-specific structs override this.
func (r *RedfishBaseBMC) UpgradeBiosVersion(_ context.Context, _ string, _ *redfish.SimpleUpdateParameters) (string, bool, error) {
	return "", false, fmt.Errorf("firmware upgrade not supported for manufacturer %q", r.manufacturer)
}

func (r *RedfishBaseBMC) GetBiosUpgradeTask(_ context.Context, _ string, _ string) (*redfish.Task, error) {
	return nil, fmt.Errorf("firmware upgrade task not supported for manufacturer %q", r.manufacturer)
}

// UpgradeBMCVersion is a fallback for unknown vendors. Vendor-specific structs override this.
func (r *RedfishBaseBMC) UpgradeBMCVersion(_ context.Context, _ string, _ *redfish.SimpleUpdateParameters) (string, bool, error) {
	return "", false, fmt.Errorf("firmware upgrade not supported for manufacturer %q", r.manufacturer)
}

func (r *RedfishBaseBMC) GetBMCUpgradeTask(_ context.Context, _ string, _ string) (*redfish.Task, error) {
	return nil, fmt.Errorf("firmware upgrade task not supported for manufacturer %q", r.manufacturer)
}

const (
	charLower = "abcdefghijklmnopqrstuvwxyz"
	charUpper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	charDigit = "0123456789"
)

// ManufacturerPasswordConfig holds vendor-specific constraints, including max length and allowed special characters.
type ManufacturerPasswordConfig struct {
	SpecialChars string
}

// Vendor-specific constraints map.
var manufacturerPasswordConfigs = map[Manufacturer]ManufacturerPasswordConfig{
	ManufacturerDell: {
		SpecialChars: "!#$%%&()*.?-@[]^_`{}|~+=",
	},
	ManufacturerHPE: {
		SpecialChars: "~`!@#$%^&*()_-+={[}]|.?/",
	},
	ManufacturerLenovo: {
		SpecialChars: ";@!$%-+=[]{}|/?~_",
	},
	"default": {
		SpecialChars: "!@#$%&*()_-+=[]{}/?~|",
	},
}

// GenerateSecurePassword generates a secure password for BMC accounts based on vendor-specific requirements.
func GenerateSecurePassword(manufacturer Manufacturer, length int) (string, error) {
	config, ok := manufacturerPasswordConfigs[manufacturer]
	if !ok {
		config = manufacturerPasswordConfigs["default"]
	}

	// Define the total character pool using the vendor-specific special characters.
	allChars := charLower + charUpper + charDigit + config.SpecialChars

	// Ensure the special character set is not empty (it shouldn't be with the defined constants)
	if len(config.SpecialChars) == 0 {
		return "", fmt.Errorf("vendor %s has an empty special character set, complexity cannot be guaranteed", manufacturer)
	}

	// Ensure minimum complexity (at least one of each type).
	mustInclude := []string{charLower, charUpper, charDigit, config.SpecialChars}
	if length < len(mustInclude) {
		return "", fmt.Errorf("password length must be at least %d to meet complexity requirements", len(mustInclude))
	}

	passwordRunes := make([]rune, length)
	currentIdx := 0

	// A. Add mandatory characters (one from each group).
	for i, charSet := range mustInclude {
		if len(charSet) == 0 {
			return "", fmt.Errorf("character set %d is empty, cannot generate secure password", i)
		}
		char, err := randomChar(charSet)
		if err != nil {
			return "", err
		}
		passwordRunes[currentIdx] = char
		currentIdx++
	}

	// B. Fill the remainder randomly.
	remainingLength := length - len(mustInclude)
	for range remainingLength {
		char, err := randomChar(allChars)
		if err != nil {
			return "", err
		}
		passwordRunes[currentIdx] = char
		currentIdx++
	}

	// C. Shuffle to randomize positions.
	if err := shuffleRunes(passwordRunes); err != nil {
		return "", err
	}

	return string(passwordRunes), nil
}

func randomChar(charSet string) (rune, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
	if err != nil {
		return 0, err
	}
	return rune(charSet[n.Int64()]), nil
}

func shuffleRunes(a []rune) error {
	for i := len(a) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := n.Int64()
		a[i], a[j] = a[j], a[i]
	}
	return nil
}

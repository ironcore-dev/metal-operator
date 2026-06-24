// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/stmcginnis/gofish/schemas"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ErrNotSupported is returned when a BMC operation is not supported by the vendor.
var ErrNotSupported = errors.New("operation not supported by this vendor")

type Manufacturer string

const (
	ManufacturerDell       Manufacturer = "Dell Inc."
	ManufacturerLenovo     Manufacturer = "Lenovo"
	ManufacturerHPE        Manufacturer = "HPE"
	ManufacturerSupermicro Manufacturer = "Supermicro"
)

// ComponentType represents a firmware component type.
type ComponentType string

const (
	ComponentTypeBMC  ComponentType = "BMC"
	ComponentTypeBIOS ComponentType = "BIOS"
)

// The BMC interface is intentionally split into smaller, capability-focused
// interfaces below. Each consumer should accept the narrowest combination of
// interfaces it actually uses, following the "accept interfaces, return
// structs" idiom and io.Reader-style interface segregation. BMC itself is
// retained as the union of all capabilities for callers (such as the BMC
// factory) that need to hand out a fully-featured client.

// PowerController manages the power state of a system.
type PowerController interface {
	// PowerOn powers on the system.
	PowerOn(ctx context.Context, systemURI string) error

	// PowerOff gracefully shuts down the system.
	PowerOff(ctx context.Context, systemURI string) error

	// ForcePowerOff powers off the system.
	ForcePowerOff(ctx context.Context, systemURI string) error

	// Reset performs a reset on the system.
	Reset(ctx context.Context, systemURI string, resetType schemas.ResetType) error

	// WaitForServerPowerState waits for the server to reach the specified power state.
	WaitForServerPowerState(ctx context.Context, systemURI string, powerState schemas.PowerState) error
}

// BootController manages boot overrides and boot order for a system.
type BootController interface {
	// SetBootOverride configures the system to network-boot on its next
	// power-on, bypassing the persistent boot order. If persistent is false
	// the override applies to a single boot only; if true it applies to every
	// subsequent boot until ClearBootOverride is called.
	SetBootOverride(ctx context.Context, systemURI string, persistent bool) error

	// ClearBootOverride removes any active boot override so the system uses
	// its persistent boot order on the next power-on. No-op when no override
	// is currently set.
	ClearBootOverride(ctx context.Context, systemURI string) error

	// GetBootOrder retrieves the boot order for the system.
	GetBootOrder(ctx context.Context, systemURI string) ([]string, error)

	// SetBootOrder sets the boot order for the system.
	SetBootOrder(ctx context.Context, systemURI string, order []string) error
}

// SystemInspector reads inventory information from the managed systems.
type SystemInspector interface {
	// GetSystems returns the managed systems.
	GetSystems(ctx context.Context) ([]Server, error)

	// GetSystemInfo retrieves information about the system.
	GetSystemInfo(ctx context.Context, systemURI string) (SystemInfo, error)

	// GetProcessors retrieves processor information for the system.
	GetProcessors(ctx context.Context, systemURI string) ([]Processor, error)

	// GetStorages retrieves storage information for the system.
	GetStorages(ctx context.Context, systemURI string) ([]Storage, error)
}

// BIOSManager reads and updates BIOS attributes on a system.
type BIOSManager interface {
	// GetBiosVersion retrieves the BIOS version for the system.
	GetBiosVersion(ctx context.Context, systemURI string) (string, error)

	// GetBiosAttributeValues retrieves BIOS attribute values for the system.
	GetBiosAttributeValues(ctx context.Context, systemURI string, attributes []string) (schemas.SettingsAttributes, error)

	// GetBiosPendingAttributeValues retrieves pending BIOS attribute values for the system.
	GetBiosPendingAttributeValues(ctx context.Context, systemURI string) (schemas.SettingsAttributes, error)

	// SetBiosAttributesOnReset sets BIOS attributes on the system and applies them on the next reset.
	SetBiosAttributesOnReset(ctx context.Context, systemURI string, attributes schemas.SettingsAttributes) (err error)

	// CheckBiosAttributes checks if the BIOS attributes are valid and returns whether a reset is required.
	CheckBiosAttributes(attrs schemas.SettingsAttributes) (reset bool, err error)
}

// BMCSettingsManager reads and updates BMC attributes (manager settings).
type BMCSettingsManager interface {
	// GetBMCVersion retrieves the BMC version for the system.
	GetBMCVersion(ctx context.Context, UUID string) (string, error)

	// GetBMCAttributeValues retrieves BMC attribute values for the system.
	GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (schemas.SettingsAttributes, error)

	// GetBMCPendingAttributeValues retrieves pending BMC attribute values for the system.
	GetBMCPendingAttributeValues(ctx context.Context, UUID string) (result schemas.SettingsAttributes, err error)

	// SetBMCAttributesImmediately sets BMC attributes on the system and applies them immediately.
	SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes schemas.SettingsAttributes) (err error)

	// CheckBMCAttributes checks if the BMC attributes are valid and returns whether a reset is required.
	CheckBMCAttributes(ctx context.Context, UUID string, attrs schemas.SettingsAttributes) (reset bool, err error)
}

// FirmwareUpdater drives BIOS and BMC firmware upgrades.
type FirmwareUpdater interface {
	// UpgradeBiosVersion upgrades the BIOS version for the system.
	UpgradeBiosVersion(ctx context.Context, manufacturer string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error)

	// GetBiosUpgradeTask retrieves the task for the BIOS upgrade.
	GetBiosUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*schemas.Task, error)

	// UpgradeBMCVersion upgrades the BMC version for the system.
	UpgradeBMCVersion(ctx context.Context, manufacturer string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error)

	// GetBMCUpgradeTask retrieves the task for the BMC upgrade.
	GetBMCUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*schemas.Task, error)

	// CheckBMCPendingComponentUpgrade checks if there are pending/staged firmware upgrades
	// for the given component type.
	CheckBMCPendingComponentUpgrade(ctx context.Context, componentType ComponentType) (bool, error)
}

// ManagerController interacts with the BMC's own Manager resource.
type ManagerController interface {
	// GetManager returns the manager.
	GetManager(UUID string) (*schemas.Manager, error)

	// DiscoverManager returns the first manager that exposes graphical console capabilities.
	DiscoverManager(ctx context.Context) (*schemas.Manager, error)

	// ResetManager performs a reset on the Manager.
	ResetManager(ctx context.Context, UUID string, resetType schemas.ResetType) error
}

// AccountManager manages BMC user accounts.
type AccountManager interface {
	// CreateOrUpdateAccount creates or updates a BMC user account.
	CreateOrUpdateAccount(ctx context.Context, userName, role, password string, enabled bool) error

	// DeleteAccount deletes a BMC user account.
	DeleteAccount(ctx context.Context, userName, id string) error

	// GetAccounts retrieves all BMC user accounts.
	GetAccounts() ([]*schemas.ManagerAccount, error)

	// GetAccountService retrieves the account service.
	GetAccountService() (*schemas.AccountService, error)
}

// EventSubscriber manages Redfish event subscriptions on the BMC.
type EventSubscriber interface {
	// CreateEventSubscription creates an event subscription for the manager.
	CreateEventSubscription(ctx context.Context, destination string, eventType schemas.EventFormatType, protocol schemas.DeliveryRetryPolicy) (string, error)

	// DeleteEventSubscription deletes an event subscription for the manager.
	DeleteEventSubscription(ctx context.Context, uri string) error
}

// Logouter closes the BMC client connection.
type Logouter interface {
	// Logout closes the BMC client connection by logging out.
	Logout()
}

// BMC is the union of every capability a Redfish BMC client may offer. It
// exists so that constructors can return a single fully-featured value;
// individual consumers should depend on the narrower capability interfaces
// above whenever possible.
type BMC interface {
	PowerController
	BootController
	SystemInspector
	BIOSManager
	BMCSettingsManager
	FirmwareUpdater
	ManagerController
	AccountManager
	EventSubscriber
	Logouter
}

// VendorFactory wraps a *RedfishBaseBMC in a vendor-specific implementation.
// External packages can supply their own factories via Options.Vendors to
// extend the BMC client with new manufacturers without modifying this package.
type VendorFactory func(base *RedfishBaseBMC) BMC

// DefaultVendors returns the built-in vendor factories for Dell, HPE, Lenovo
// and Supermicro. Callers can copy and extend this map, or build their own
// from scratch, and pass the result through Options.Vendors.
func DefaultVendors() map[Manufacturer]VendorFactory {
	return map[Manufacturer]VendorFactory{
		ManufacturerDell:       func(b *RedfishBaseBMC) BMC { return &DellRedfishBMC{RedfishBaseBMC: b} },
		ManufacturerHPE:        func(b *RedfishBaseBMC) BMC { return &HPERedfishBMC{RedfishBaseBMC: b} },
		ManufacturerLenovo:     func(b *RedfishBaseBMC) BMC { return &LenovoRedfishBMC{RedfishBaseBMC: b} },
		ManufacturerSupermicro: func(b *RedfishBaseBMC) BMC { return &SupermicroRedfishBMC{RedfishBaseBMC: b} },
	}
}

// Compile-time guarantees that every built-in implementation continues to
// satisfy the full BMC union. New vendor structs added to this package should
// extend this list so regressions surface at build time, not on first call.
var (
	_ BMC = (*RedfishBaseBMC)(nil)
	_ BMC = (*DellRedfishBMC)(nil)
	_ BMC = (*HPERedfishBMC)(nil)
	_ BMC = (*LenovoRedfishBMC)(nil)
	_ BMC = (*SupermicroRedfishBMC)(nil)
)

type Entity struct {
	// ID uniquely identifies the resource.
	ID string `json:"Id"`
	// Name is the name of the resource or array element.
	Name string `json:"name"`
}

type AllowedValues struct {
	ValueDisplayName string
	ValueName        string
}

type RegistryEntryAttributes struct {
	AttributeName string
	CurrentValue  any
	DisplayName   string
	DisplayOrder  int
	HelpText      string
	Hidden        bool
	Immutable     bool
	MaxLength     int
	MenuPath      string
	MinLength     int
	ReadOnly      bool
	ResetRequired *bool
	Type          string
	WriteOnly     bool
	Value         []AllowedValues
}

type RegistryEntry struct {
	Attributes []RegistryEntryAttributes
}

// Registry describes the Message Registry file locator Resource.
type Registry struct {
	schemas.Entity
	// ODataContext is the odata context.
	ODataContext string `json:"@odata.context"`
	// ODataType is the odata type.
	ODataType string `json:"@odata.type"`
	// Description provides a description of this resource.
	Description string
	// Languages is the RFC5646-conformant language codes for the
	// available Message Registries.
	Languages []string
	// Registry shall contain the Message Registry name and it major and
	// minor versions, as defined by the Redfish Specification.
	RegistryEntries RegistryEntry
}

type NetworkInterface struct {
	ID                  string
	MACAddress          string
	PermanentMACAddress string
}

type Server struct {
	UUID         string
	URI          string
	Model        string
	Manufacturer string
	PowerState   schemas.PowerState
	SerialNumber string
}

// Volume represents a storage volume.
type Volume struct {
	Entity
	// CapacityBytes specifies the capacity of the volume in bytes.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Status specifies the status of the volume.
	State schemas.State `json:"state,omitempty"`
	// RAIDType specifies the RAID type of the associated Volume.
	RAIDType schemas.RAIDType `json:"raidType,omitempty"`
	// VolumeUsage specifies the volume usage type for the Volume.
	VolumeUsage string `json:"volumeUsage,omitempty"`
}

// Drive represents a storage drive.
type Drive struct {
	Entity
	// MediaType specifies the media type of the storage device.
	MediaType string `json:"mediaType,omitempty"`
	// Type specifies the type of the storage device.
	Type schemas.FormFactor `json:"type,omitempty"`
	// SizeBytes specifies the size of the storage device in bytes.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Vendor specifies the vendor of the storage device.
	Vendor string `json:"vendor,omitempty"`
	// Model specifies the model of the storage device.
	Model string `json:"model,omitempty"`
	// State specifies the state of the storage device.
	State schemas.State `json:"state,omitempty"`
}

// Storage represents a storage resource.
type Storage struct {
	Entity
	// State specifies the state of the storage.
	State schemas.State `json:"state,omitempty"`
	// Drives is a collection of drives associated with this storage.
	Drives []Drive `json:"drives,omitempty"`
	// Volumes is a collection of volumes associated with this storage.
	Volumes []Volume `json:"volumes,omitempty"`
}

// Processor represents a processor in the system.
type Processor struct {
	// ID uniquely identifies the resource.
	ID string
	// Type specifies the type of processor.
	Type string
	// Architecture specifies the architecture of the processor.
	Architecture string
	// InstructionSet specifies the instruction set of the processor.
	InstructionSet string
	// Manufacturer specifies the manufacturer of the processor.
	Manufacturer string
	// Model specifies the model of the processor.
	Model string
	// MaxSpeedMHz specifies the maximum speed of the processor in MHz.
	MaxSpeedMHz int32
	// TotalCores specifies the total number of cores in the processor.
	TotalCores int32
	// TotalThreads specifies the total number of threads in the processor.
	TotalThreads int32
}

// SystemInfo represents basic information about the system.
type SystemInfo struct {
	Manufacturer      string
	Model             string
	Status            schemas.Status
	PowerState        schemas.PowerState
	TotalSystemMemory resource.Quantity
	SystemURI         string
	SystemUUID        string
	SystemInfo        string
	SerialNumber      string
	SKU               string
	IndicatorLED      string
}

// Manager represents the manager information.
type Manager struct {
	UUID            string
	Manufacturer    string
	FirmwareVersion string
	SerialNumber    string
	SKU             string
	Model           string
	PowerState      string
	State           string
	MACAddress      string
	OemLinks        json.RawMessage
}

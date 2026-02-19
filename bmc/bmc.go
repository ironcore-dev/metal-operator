// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ironcore-dev/metal-operator/bmc/oem"
)

type Manufacturer string

const (
	ManufacturerDell       Manufacturer = "Dell Inc."
	ManufacturerLenovo     Manufacturer = "Lenovo"
	ManufacturerHPE        Manufacturer = "HPE"
	ManufacturerSupermicro Manufacturer = "Supermicro"
)

// BMC defines an interface for interacting with a Baseboard Management Controller.
type BMC interface {
	// PowerOn powers on the system.
	PowerOn(ctx context.Context, systemURI string) error

	// PowerOff gracefully shuts down the system.
	PowerOff(ctx context.Context, systemURI string) error

	// ForcePowerOff powers off the system.
	ForcePowerOff(ctx context.Context, systemURI string) error

	// Reset performs a reset on the system.
	Reset(ctx context.Context, systemURI string, resetType schemas.ResetType) error

	// SetPXEBootOnce sets the boot device for the next system boot.
	SetPXEBootOnce(ctx context.Context, systemURI string) error

	// GetSystemInfo retrieves information about the system.
	GetSystemInfo(ctx context.Context, systemURI string) (SystemInfo, error)

	// Logout closes the BMC client connection by logging out
	Logout()

	// GetSystems returns the managed systems
	GetSystems(ctx context.Context) ([]Server, error)

	// GetManager returns the manager
	GetManager(UUID string) (*schemas.Manager, error)

	// ResetManager performs a reset on the Manager.
	ResetManager(ctx context.Context, UUID string, resetType schemas.ResetType) error

	// GetBootOrder retrieves the boot order for the system.
	GetBootOrder(ctx context.Context, systemURI string) ([]string, error)

	// GetBiosAttributeValues retrieves BIOS attribute values for the system.
	GetBiosAttributeValues(ctx context.Context, systemURI string, attributes []string) (schemas.SettingsAttributes, error)

	// GetBiosPendingAttributeValues retrieves pending BIOS attribute values for the system.
	GetBiosPendingAttributeValues(ctx context.Context, systemURI string) (schemas.SettingsAttributes, error)

	// GetBMCAttributeValues retrieves BMC attribute values for the system.
	GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (schemas.SettingsAttributes, error)

	// GetBMCPendingAttributeValues retrieves pending BMC attribute values for the system.
	GetBMCPendingAttributeValues(ctx context.Context, UUID string) (result schemas.SettingsAttributes, err error)

	// CheckBiosAttributes checks if the BIOS attributes are valid and returns whether a reset is required.
	CheckBiosAttributes(attrs schemas.SettingsAttributes) (reset bool, err error)

	// CheckBMCAttributes checks if the BMC attributes are valid and returns whether a reset is required.
	CheckBMCAttributes(ctx context.Context, UUID string, attrs schemas.SettingsAttributes) (reset bool, err error)

	// SetBiosAttributesOnReset sets BIOS attributes on the system and applies them on the next reset.
	SetBiosAttributesOnReset(ctx context.Context, systemURI string, attributes schemas.SettingsAttributes) (err error)

	// SetBMCAttributesImmediately sets BMC attributes on the system and applies them immediately.
	SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes schemas.SettingsAttributes) (err error)

	// GetBiosVersion retrieves the BIOS version for the system.
	GetBiosVersion(ctx context.Context, systemURI string) (string, error)

	// GetBMCVersion retrieves the BMC version for the system.
	GetBMCVersion(ctx context.Context, UUID string) (string, error)

	// SetBootOrder sets the boot order for the system.
	SetBootOrder(ctx context.Context, systemURI string, order []string) error

	// GetStorages retrieves storage information for the system.
	GetStorages(ctx context.Context, systemURI string) ([]Storage, error)

	// GetProcessors retrieves processor information for the system.
	GetProcessors(ctx context.Context, systemURI string) ([]Processor, error)

	// UpgradeBiosVersion upgrades the BIOS version for the system.
	UpgradeBiosVersion(ctx context.Context, manufacturer string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error)

	// GetBiosUpgradeTask retrieves the task for the BIOS upgrade.
	GetBiosUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*schemas.Task, error)

	// WaitForServerPowerState waits for the server to reach the specified power state.
	WaitForServerPowerState(ctx context.Context, systemURI string, powerState schemas.PowerState) error

	// UpgradeBMCVersion upgrades the BMC version for the system.
	UpgradeBMCVersion(ctx context.Context, manufacturer string, parameters *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error)

	// GetBMCUpgradeTask retrieves the task for the BMC upgrade.
	GetBMCUpgradeTask(ctx context.Context, manufacturer string, taskURI string) (*schemas.Task, error)

	// SetVirtualMediaBootOnce sets one-time boot from virtual media (CD/DVD).
	// This sets the boot source override to CD/DVD for one boot cycle only.
	SetVirtualMediaBootOnce(ctx context.Context, systemURI string) error

	// MountVirtualMedia mounts an ISO image via BMC virtual media.
	// The mediaURL should be an HTTP/HTTPS URL accessible by the BMC.
	// The slotID indicates which virtual media slot to use (e.g., "1" or "2").
	MountVirtualMedia(ctx context.Context, systemURI string, mediaURL string, slotID string) error

	// EjectVirtualMedia ejects virtual media from the specified slot.
	EjectVirtualMedia(ctx context.Context, systemURI string, slotID string) error

	// GetVirtualMediaStatus retrieves the status of all virtual media devices.
	GetVirtualMediaStatus(ctx context.Context, systemURI string) ([]*redfish.VirtualMedia, error)

	// CreateOrUpdateAccount creates or updates a BMC user account.
	CreateOrUpdateAccount(ctx context.Context, userName, role, password string, enabled bool) error

	// DeleteAccount deletes a BMC user account.
	DeleteAccount(ctx context.Context, userName, id string) error

	// GetAccounts retrieves all BMC user accounts.
	GetAccounts() ([]*schemas.ManagerAccount, error)

	// GetAccountService retrieves the account service.
	GetAccountService() (*schemas.AccountService, error)
}

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

func NewOEMManager(manager *schemas.Manager, service *gofish.Service) (oem.ManagerInterface, error) {
	var OEMManager oem.ManagerInterface
	switch manager.Manufacturer {
	case string(ManufacturerDell):
		OEMManager = &oem.DellIdracManager{
			BMC:     manager,
			Service: service,
		}
	case string(ManufacturerHPE):
		OEMManager = &oem.HPEILOManager{
			BMC:     manager,
			Service: service,
		}
	case string(ManufacturerLenovo):
		OEMManager = &oem.LenovoXCCManager{
			BMC:     manager,
			Service: service,
		}
	default:
		return nil, fmt.Errorf("unsupported manufacturer: %v", manager.Manufacturer)
	}
	return OEMManager, nil
}

func NewOEMInterface(manufacturer string, service *gofish.Service) (oem.OEMInterface, error) {
	var oemInterface oem.OEMInterface
	switch manufacturer {
	case string(ManufacturerDell):
		return &oem.Dell{
			Service: service,
		}, nil
	case string(ManufacturerHPE):
		return &oem.HPE{
			Service: service,
		}, nil
	case string(ManufacturerLenovo):
		return &oem.Lenovo{
			Service: service,
		}, nil
	default:
		return oemInterface, fmt.Errorf("unsupported manufacturer: %v", manufacturer)
	}
}

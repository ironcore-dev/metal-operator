// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	"k8s.io/apimachinery/pkg/api/resource"
)

// BMC defines an interface for interacting with a Baseboard Management Controller.
type BMC interface {
	// PowerOn powers on the system.
	PowerOn(ctx context.Context, systemUUID string) error

	// PowerOff gracefully shuts down the system.
	PowerOff(ctx context.Context, systemUUID string) error

	// ForcePowerOff powers off the system.
	ForcePowerOff(ctx context.Context, systemUUID string) error

	// Reset performs a reset on the system.
	Reset(ctx context.Context, systemUUID string, resetType redfish.ResetType) error

	// SetPXEBootOnce sets the boot device for the next system boot.
	SetPXEBootOnce(ctx context.Context, systemUUID string) error

	// GetSystemInfo retrieves information about the system.
	GetSystemInfo(ctx context.Context, systemUUID string) (SystemInfo, error)

	// Logout closes the BMC client connection by logging out
	Logout()

	// GetSystems returns the managed systems
	GetSystems(ctx context.Context) ([]Server, error)

	// GetManager returns the manager
	GetManager() (*Manager, error)

	GetBootOrder(ctx context.Context, systemUUID string) ([]string, error)

	GetBiosAttributeValues(ctx context.Context, systemUUID string, attributes []string) (redfish.SettingsAttributes, error)

	CheckBiosAttributes(attrs map[string]string) (reset bool, err error)

	SetBiosAttributesOnReset(ctx context.Context, systemUUID string, attributes map[string]string) (err error)

	GetBiosVersion(ctx context.Context, systemUUID string) (string, error)

	SetBootOrder(ctx context.Context, systemUUID string, order []string) error

	GetStorages(ctx context.Context, systemUUID string) ([]Storage, error)

	WaitForServerPowerState(ctx context.Context, systemUUID string, powerState redfish.PowerState) error
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
	CurrentValue  interface{}
	DisplayName   string
	DisplayOrder  int
	HelpText      string
	Hidden        bool
	Immutable     bool
	MaxLength     int
	MenuPath      string
	MinLength     int
	ReadOnly      bool
	ResetRequired bool
	Type          string
	WriteOnly     bool
	Value         []AllowedValues
}

type RegistryEntry struct {
	Attributes []RegistryEntryAttributes
}

// BiosRegistry describes the Message Registry file locator Resource.
type BiosRegistry struct {
	common.Entity
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
	Model        string
	Manufacturer string
	PowerState   PowerState
	SerialNumber string
}

// Volume represents a storage volume.
type Volume struct {
	Entity
	// CapacityBytes specifies the capacity of the volume in bytes.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Status specifies the status of the volume.
	State common.State `json:"state,omitempty"`
	// RAIDType specifies the RAID type of the associated Volume.
	RAIDType redfish.RAIDType `json:"raidType,omitempty"`
	// VolumeUsage specifies the volume usage type for the Volume.
	VolumeUsage string `json:"volumeUsage,omitempty"`
}

// Drive represents a storage drive.
type Drive struct {
	Entity
	// MediaType specifies the media type of the storage device.
	MediaType string `json:"mediaType,omitempty"`
	// Type specifies the type of the storage device.
	Type redfish.FormFactor `json:"type,omitempty"`
	// SizeBytes specifies the size of the storage device in bytes.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Vendor specifies the vendor of the storage device.
	Vendor string `json:"vendor,omitempty"`
	// Model specifies the model of the storage device.
	Model string `json:"model,omitempty"`
	// State specifies the state of the storage device.
	State common.State `json:"state,omitempty"`
}

// Storage represents a storage resource.
type Storage struct {
	Entity
	// State specifies the state of the storage.
	State common.State `json:"state,omitempty"`
	// Drives is a collection of drives associated with this storage.
	Drives []Drive `json:"drives,omitempty"`
	// Volumes is a collection of volumes associated with this storage.
	Volumes []Volume `json:"volumes,omitempty"`
}

// PowerState is the power state of the system.
type PowerState string

const (
	// OnPowerState the system is powered on.
	OnPowerState PowerState = "On"
	// OffPowerState the system is powered off, although some components may
	// continue to have AUX power such as management controller.
	OffPowerState PowerState = "Off"
	// PausedPowerState the system is paused.
	PausedPowerState PowerState = "Paused"
	// PoweringOnPowerState A temporary state between Off and On. This
	// temporary state can be very short.
	PoweringOnPowerState PowerState = "PoweringOn"
	// PoweringOffPowerState A temporary state between On and Off. The power
	// off action can take time while the OS is in the shutdown process.
	PoweringOffPowerState PowerState = "PoweringOff"
)

type Processor struct {
	ID                    string
	ProcessorType         string
	ProcessorArchitecture string
	InstructionSet        string
	Manufacturer          string
	Model                 string
	MaxSpeedMHz           int32
	TotalCores            int32
	TotalThreads          int32
}

// SystemInfo represents basic information about the system.
type SystemInfo struct {
	Manufacturer      string
	Model             string
	Status            common.Status
	PowerState        redfish.PowerState
	NetworkInterfaces []NetworkInterface
	Processors        []Processor
	TotalSystemMemory resource.Quantity
	SystemUUID        string
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
}

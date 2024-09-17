// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// BMC defines an interface for interacting with a Baseboard Management Controller.
type BMC interface {
	// PowerOn powers on the system.
	PowerOn(systemUUID string) error

	// PowerOff gracefully shuts down the system.
	PowerOff(systemUUID string) error

	// ForcePowerOff powers off the system.
	ForcePowerOff(systemUUID string) error

	// Reset performs a reset on the system.
	Reset(systemUUID string, resetType redfish.ResetType) error

	// SetPXEBootOnce sets the boot device for the next system boot.
	SetPXEBootOnce(systemUUID string) error

	// GetSystemInfo retrieves information about the system.
	GetSystemInfo(systemUUID string) (SystemInfo, error)

	// Logout closes the BMC client connection by logging out
	Logout()

	// GetSystems returns the managed systems
	GetSystems() ([]Server, error)

	// GetManager returns the manager
	GetManager() (*Manager, error)

	GetBootOrder(systemUUID string) ([]string, error)

	GetBiosAttributeValues(systemUUID string, attributes []string) (map[string]string, error)

	SetBiosAttributes(systemUUID string, attributes map[string]string) (reset bool, err error)

	GetBiosVersion(systemUUID string) (string, error)

	SetBootOrder(systemUUID string, order []string) error
}

type Bios struct {
	Version    string
	Attributes map[string]string
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
}

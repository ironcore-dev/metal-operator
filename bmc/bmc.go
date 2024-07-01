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

	// PowerOff powers off the system.
	PowerOff(systemUUID string) error

	// Reset performs a reset on the system.
	Reset() error

	// SetPXEBootOnce sets the boot device for the next system boot.
	SetPXEBootOnce(systemID string) error

	// GetSystemInfo retrieves information about the system.
	GetSystemInfo(systemID string) (SystemInfo, error)

	// Logout closes the BMC client connection by logging out
	Logout()

	// GetSystems returns the managed systems
	GetSystems() ([]Server, error)

	// GetManager returns the manager
	GetManager() (*Manager, error)
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

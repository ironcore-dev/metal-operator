// Package hypervisor defines the south-side abstraction the Redfish emulator
// drives. A single Hypervisor instance represents exactly one managed machine
// (one Redfish "System"). Concrete drivers (raw QEMU, a fake, and later others)
// implement this interface; the Redfish server depends only on it.
package hypervisor

// PowerState mirrors the Redfish ComputerSystem PowerState values the emulator
// exposes. PoweringOn/PoweringOff are transient states used while a driver is
// starting or stopping the underlying machine.
type PowerState string

const (
	PowerOff         PowerState = "Off"
	PowerOn          PowerState = "On"
	PowerPoweringOn  PowerState = "PoweringOn"
	PowerPoweringOff PowerState = "PoweringOff"
)

// ResetType is a 1:1 mapping of the Redfish ComputerSystem.Reset ResetType
// enumeration values the emulator accepts.
type ResetType string

const (
	ResetOn               ResetType = "On"
	ResetForceOn          ResetType = "ForceOn"
	ResetForceOff         ResetType = "ForceOff"
	ResetGracefulShutdown ResetType = "GracefulShutdown"
	ResetGracefulRestart  ResetType = "GracefulRestart"
	ResetForceRestart     ResetType = "ForceRestart"
	ResetPowerCycle       ResetType = "PowerCycle"
	ResetNmi              ResetType = "Nmi"
)

// PowersOn reports whether the reset type should leave the machine running.
func (r ResetType) PowersOn() bool {
	switch r {
	case ResetOn, ResetForceOn, ResetGracefulRestart, ResetForceRestart, ResetPowerCycle:
		return true
	default:
		return false
	}
}

// Restarts reports whether the reset type implies an off-then-on cycle of an
// already-running machine (as opposed to a plain power on/off).
func (r ResetType) Restarts() bool {
	switch r {
	case ResetGracefulRestart, ResetForceRestart, ResetPowerCycle:
		return true
	default:
		return false
	}
}

// Graceful reports whether the reset type requests a clean guest shutdown
// rather than an abrupt power cut.
func (r ResetType) Graceful() bool {
	switch r {
	case ResetGracefulShutdown, ResetGracefulRestart:
		return true
	default:
		return false
	}
}

// BootTarget mirrors the Redfish Boot.BootSourceOverrideTarget enumeration.
// UefiHttp is the primary target this emulator is built to exercise.
type BootTarget string

const (
	BootNone     BootTarget = "None"
	BootPxe      BootTarget = "Pxe"
	BootHdd      BootTarget = "Hdd"
	BootCd       BootTarget = "Cd"
	BootUsb      BootTarget = "Usb"
	BootUefiHttp BootTarget = "UefiHttp"
)

// OverrideMode mirrors Redfish Boot.BootSourceOverrideEnabled.
type OverrideMode string

const (
	OverrideDisabled   OverrideMode = "Disabled"
	OverrideOnce       OverrideMode = "Once"
	OverrideContinuous OverrideMode = "Continuous"
)

// BootMode mirrors Redfish Boot.BootSourceOverrideMode: the firmware boot mode
// applied for the override. Modern hardware boots UEFI.
type BootMode string

const (
	BootModeLegacy BootMode = "Legacy"
	BootModeUEFI   BootMode = "UEFI"
)

// BootOverride is the boot configuration recorded by SetBootOverride and
// consumed at the next PowerOn. For a UefiHttp target, HTTPBootURI is the URI
// the guest firmware must fetch; it is empty for other targets.
type BootOverride struct {
	Enabled     OverrideMode
	Target      BootTarget
	Mode        BootMode // BootSourceOverrideMode; empty means unspecified (treated as UEFI)
	HTTPBootURI string
}

// MediaSpec describes a virtual media image to attach to the machine.
type MediaSpec struct {
	DeviceID  string // e.g. "Cd", stable identity within a machine
	Image     string // URI or path of the image to insert
	Inserted  bool
	WriteProt bool
}

// MediaState is the observed state of a virtual media device.
type MediaState struct {
	MediaSpec
	ConnectedVia string // Redfish VirtualMedia ConnectedVia (e.g. "URI", "NotConnected")
}

// VMStatus is a snapshot of a machine's runtime state, used to render Redfish
// resources and for diagnostics.
type VMStatus struct {
	Power         PowerState
	PID           int
	QMPConnected  bool
	Boot          BootOverride
	SerialLog     string
	StartedAtUnix int64
}

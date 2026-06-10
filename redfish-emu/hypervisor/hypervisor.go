package hypervisor

import "context"

// EventKind enumerates the normalized lifecycle events a driver reports on its
// Events channel. Drivers translate backend-specific signals (QMP events,
// serial-console markers, boot-server callbacks) into these.
type EventKind string

const (
	// EvBootRequested is emitted when a power-on is initiated (before the
	// machine is running).
	EvBootRequested EventKind = "BootRequested"
	// EvPoweredOn is emitted once the underlying machine process is running.
	EvPoweredOn EventKind = "PoweredOn"
	// EvGuestBooted is emitted when the guest OS is observed to have booted
	// (e.g. a serial-console marker was seen).
	EvGuestBooted EventKind = "GuestBooted"
	// EvHTTPBootFetch is emitted when the guest firmware's HTTP boot fetch is
	// observed against the boot server.
	EvHTTPBootFetch EventKind = "HTTPBootFetchObserved"
	// EvPoweredOff is emitted once the machine has fully stopped.
	EvPoweredOff EventKind = "PoweredOff"
	// EvCrashed is emitted when the machine exits abnormally.
	EvCrashed EventKind = "Crashed"
)

// Event is a single normalized lifecycle notification.
type Event struct {
	Kind    EventKind
	Message string
	AtUnix  int64
}

// Hypervisor is the pluggable south-side interface. One instance manages one
// machine. Implementations must be safe for concurrent use.
type Hypervisor interface {
	// Prepare provisions any host-side resources the machine needs. It is
	// called once before the first PowerOn.
	Prepare(ctx context.Context) error
	// Close stops the machine if running and releases all resources. The
	// Events channel is closed. Close is idempotent.
	Close() error

	// PowerOn starts the machine, baking the currently recorded BootOverride
	// into its launch configuration. It is a no-op error if already on.
	PowerOn(ctx context.Context) error
	// PowerOff stops the machine. When graceful is true the guest is asked to
	// shut down cleanly (with a driver-defined timeout) before being killed.
	PowerOff(ctx context.Context, graceful bool) error
	// Reset applies a Redfish ResetType. Restart-style resets are implemented
	// as a stop-then-start so a freshly recorded one-shot boot override takes
	// effect.
	Reset(ctx context.Context, t ResetType) error
	// GetPowerState returns the current power state.
	GetPowerState(ctx context.Context) (PowerState, error)

	// SetBootOverride records the boot configuration to apply at the next
	// PowerOn. It must NOT change power state.
	SetBootOverride(ctx context.Context, o BootOverride) error
	// GetBootOverride returns the currently recorded boot override.
	GetBootOverride(ctx context.Context) (BootOverride, error)

	// InsertMedia attaches (or updates) a virtual media image. Like boot
	// overrides, changes take effect at the next PowerOn for launch-time
	// drivers.
	InsertMedia(ctx context.Context, m MediaSpec) error
	// EjectMedia detaches the virtual media device identified by deviceID.
	EjectMedia(ctx context.Context, deviceID string) error
	// ListMedia returns the current virtual media devices.
	ListMedia(ctx context.Context) ([]MediaState, error)

	// Status returns a snapshot of the machine's runtime state.
	Status(ctx context.Context) (VMStatus, error)

	// Events returns the channel of lifecycle events. The channel is closed
	// when the Hypervisor is Closed.
	Events() <-chan Event
}

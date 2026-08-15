// Package fake provides an in-memory hypervisor.Hypervisor implementation that
// launches no real machine. It records boot overrides and virtual media, tracks
// a simple power state, and emits lifecycle events. It exists so the Redfish
// server can be unit-tested in milliseconds without QEMU, and as a fast default
// for tests that don't exercise the real HTTP-boot path.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

// Driver is an in-memory Hypervisor. The zero value is not usable; call New.
type Driver struct {
	// now is injected so tests can control timestamps; defaults to time.Now.
	now func() time.Time

	mu     sync.Mutex
	closed bool
	power  hypervisor.PowerState
	boot   hypervisor.BootOverride
	media  map[string]hypervisor.MediaState
	events chan hypervisor.Event
}

// New returns a fake Driver in the powered-off state.
func New() *Driver {
	return &Driver{
		now:    time.Now,
		power:  hypervisor.PowerOff,
		boot:   hypervisor.BootOverride{Enabled: hypervisor.OverrideDisabled, Target: hypervisor.BootNone},
		media:  make(map[string]hypervisor.MediaState),
		events: make(chan hypervisor.Event, 32),
	}
}

var _ hypervisor.Hypervisor = (*Driver)(nil)

// emit sends an event without blocking if the buffer is full: the oldest event
// is dropped so a slow consumer can never deadlock a state transition. Caller
// must hold d.mu.
func (d *Driver) emit(kind hypervisor.EventKind, msg string) {
	ev := hypervisor.Event{Kind: kind, Message: msg, AtUnix: d.now().Unix()}
	select {
	case d.events <- ev:
	default:
		select {
		case <-d.events:
		default:
		}
		select {
		case d.events <- ev:
		default:
		}
	}
}

func (d *Driver) Prepare(context.Context) error { return nil }

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.power = hypervisor.PowerOff
	close(d.events)
	return nil
}

func (d *Driver) PowerOn(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("fake: hypervisor closed")
	}
	if d.power == hypervisor.PowerOn {
		return nil
	}
	d.emit(hypervisor.EvBootRequested, fmt.Sprintf("boot target=%s uri=%s", d.boot.Target, d.boot.HTTPBootURI))
	d.power = hypervisor.PowerOn
	d.emit(hypervisor.EvPoweredOn, "")
	d.emit(hypervisor.EvGuestBooted, "")
	// A UefiHttp boot would fetch its URI; model that so tests waiting on the
	// fetch event don't hang against the fake.
	if d.boot.Target == hypervisor.BootUefiHttp && d.boot.HTTPBootURI != "" {
		d.emit(hypervisor.EvHTTPBootFetch, d.boot.HTTPBootURI)
	}
	return nil
}

func (d *Driver) PowerOff(_ context.Context, _ bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("fake: hypervisor closed")
	}
	if d.power == hypervisor.PowerOff {
		return nil
	}
	d.power = hypervisor.PowerOff
	d.emit(hypervisor.EvPoweredOff, "")
	return nil
}

func (d *Driver) Reset(ctx context.Context, t hypervisor.ResetType) error {
	if t.PowersOn() {
		if t.Restarts() {
			if err := d.PowerOff(ctx, t.Graceful()); err != nil {
				return err
			}
		}
		return d.PowerOn(ctx)
	}
	return d.PowerOff(ctx, t.Graceful())
}

func (d *Driver) GetPowerState(context.Context) (hypervisor.PowerState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.power, nil
}

func (d *Driver) SetBootOverride(_ context.Context, o hypervisor.BootOverride) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("fake: hypervisor closed")
	}
	d.boot = o
	return nil
}

func (d *Driver) GetBootOverride(context.Context) (hypervisor.BootOverride, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.boot, nil
}

func (d *Driver) InsertMedia(_ context.Context, m hypervisor.MediaSpec) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("fake: hypervisor closed")
	}
	m.Inserted = true
	connected := "NotConnected"
	if m.Image != "" {
		connected = "URI"
	}
	d.media[m.DeviceID] = hypervisor.MediaState{MediaSpec: m, ConnectedVia: connected}
	return nil
}

func (d *Driver) EjectMedia(_ context.Context, deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("fake: hypervisor closed")
	}
	delete(d.media, deviceID)
	return nil
}

func (d *Driver) ListMedia(context.Context) ([]hypervisor.MediaState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]hypervisor.MediaState, 0, len(d.media))
	for _, m := range d.media {
		out = append(out, m)
	}
	return out, nil
}

func (d *Driver) Status(context.Context) (hypervisor.VMStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return hypervisor.VMStatus{Power: d.power, Boot: d.boot}, nil
}

func (d *Driver) Events() <-chan hypervisor.Event { return d.events }

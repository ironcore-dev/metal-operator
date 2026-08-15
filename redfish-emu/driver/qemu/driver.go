// Package qemu implements a hypervisor.Hypervisor that drives a real QEMU
// virtual machine spawned as a child process and controlled over QMP. It is the
// reference south-side driver for the Redfish emulator.
//
// Because a UEFI boot target is a launch-time / firmware property (there is no
// QMP command to set the UEFI boot order at runtime), the driver spawns a fresh
// QEMU process on every power-on with the recorded boot override baked into a
// per-VM OVMF NVRAM vars file. Restart-style resets are therefore implemented as
// a stop-then-start so a freshly recorded one-shot override takes effect.
package qemu

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-qemu/qmp"
	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
)

// bootMarker is the string the test artifact prints to the serial console once
// the guest has booted. Observing it maps to hypervisor.EvGuestBooted.
const bootMarker = "REDFISH-EMU-BOOT-OK"

// gracefulTimeout is how long a graceful PowerOff waits for the guest to shut
// down before escalating to a forced quit.
const gracefulTimeout = 20 * time.Second

// qmpConnectTimeout bounds how long PowerOn waits for the QMP socket.
const qmpConnectTimeout = 10 * time.Second

// Driver is a QEMU-backed Hypervisor for a single machine.
type Driver struct {
	cfg Config
	fw  VarsProvisioner
	now func() time.Time

	mu      sync.Mutex
	closed  bool
	state   hypervisor.PowerState
	boot    hypervisor.BootOverride
	media   map[string]hypervisor.MediaSpec
	workDir string

	// running-instance fields; nil/zero when powered off.
	cmd       *exec.Cmd
	qmp       *qmpConn
	instDir   string
	serialLog string
	startedAt int64
	// doneCh is closed by the supervisor goroutine when the current instance
	// has fully exited and On-state fields have been cleared.
	doneCh chan struct{}

	events chan hypervisor.Event
}

var _ hypervisor.Hypervisor = (*Driver)(nil)

// New returns a QEMU Driver. cfg is completed with Defaults; fw defaults to the
// virt-fw-vars provisioner when nil.
func New(cfg Config, fw VarsProvisioner) (*Driver, error) {
	if err := cfg.Defaults(); err != nil {
		return nil, err
	}
	if fw == nil {
		fw = VirtFwVarsProvisioner{}
	}
	return &Driver{
		cfg:    cfg,
		fw:     fw,
		now:    time.Now,
		state:  hypervisor.PowerOff,
		boot:   hypervisor.BootOverride{Enabled: hypervisor.OverrideDisabled, Target: hypervisor.BootNone},
		media:  make(map[string]hypervisor.MediaSpec),
		events: make(chan hypervisor.Event, 64),
	}, nil
}

func (d *Driver) Prepare(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.workDir != "" {
		return nil
	}
	base := d.cfg.WorkDir
	if base == "" {
		dir, err := os.MkdirTemp("", "redfish-emu-*")
		if err != nil {
			return err
		}
		d.workDir = dir
		return nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	d.workDir = base
	return nil
}

func (d *Driver) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	done := d.doneCh
	running := d.cmd != nil
	if running {
		d.forceKillLocked()
	}
	d.mu.Unlock()

	if running && done != nil {
		<-done // wait for supervisor to finish cleanup
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	close(d.events)
	return nil
}

func (d *Driver) emitLocked(kind hypervisor.EventKind, msg string) {
	ev := hypervisor.Event{Kind: kind, Message: msg, AtUnix: d.now().Unix()}
	select {
	case d.events <- ev:
	default:
	}
}

func (d *Driver) Events() <-chan hypervisor.Event { return d.events }

// PowerOn spawns a fresh QEMU instance with the recorded boot override baked in.
func (d *Driver) PowerOn(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("qemu: driver closed")
	}
	if d.state == hypervisor.PowerOn || d.state == hypervisor.PowerPoweringOn {
		return nil
	}
	if d.workDir == "" {
		return fmt.Errorf("qemu: Prepare not called")
	}

	d.state = hypervisor.PowerPoweringOn
	d.emitLocked(hypervisor.EvBootRequested,
		fmt.Sprintf("target=%s uri=%s", d.boot.Target, d.boot.HTTPBootURI))

	inst, err := d.buildInstanceLocked(ctx)
	if err != nil {
		d.state = hypervisor.PowerOff
		return err
	}

	if err := inst.cmd.Start(); err != nil {
		d.state = hypervisor.PowerOff
		return fmt.Errorf("qemu: start: %w", err)
	}

	// Connect QMP (socket appears asynchronously). On failure, kill the process.
	conn, err := dialQMP(ctx, inst.qmpSock, qmpConnectTimeout)
	if err != nil {
		_ = inst.cmd.Process.Kill()
		_ = inst.cmd.Wait()
		d.state = hypervisor.PowerOff
		return err
	}

	d.cmd = inst.cmd
	d.qmp = conn
	d.instDir = inst.dir
	d.serialLog = inst.serialLog
	d.startedAt = d.now().Unix()
	d.doneCh = make(chan struct{})
	d.state = hypervisor.PowerOn
	d.emitLocked(hypervisor.EvPoweredOn, fmt.Sprintf("pid=%d", inst.cmd.Process.Pid))

	go d.supervise(d.cmd, conn, inst.dir, inst.serialLog, d.doneCh)
	return nil
}

// instance holds the resolved artifacts for one QEMU launch.
type instance struct {
	cmd       *exec.Cmd
	dir       string
	qmpSock   string
	serialLog string
}

// buildInstanceLocked provisions a per-launch scratch dir, the NVRAM vars file
// (with the HTTP-boot entry injected), and the QEMU command. Caller holds d.mu.
func (d *Driver) buildInstanceLocked(ctx context.Context) (instance, error) {
	dir, err := os.MkdirTemp(d.workDir, "vm-*")
	if err != nil {
		return instance{}, err
	}
	vars := varsPath(dir)
	qmpSock := filepath.Join(dir, "qmp.sock")
	serialLog := filepath.Join(dir, "serial.log")

	// Provision the writable NVRAM vars file. For a UefiHttp target we inject a
	// one-shot HTTP-boot entry; otherwise we just materialize a fresh vars file.
	if d.boot.Target == hypervisor.BootUefiHttp && d.cfg.BootMode == BootFirmwareHTTP {
		if d.boot.HTTPBootURI == "" {
			return instance{}, fmt.Errorf("qemu: UefiHttp boot requires an HttpBootUri")
		}
		guestURI, err := guestBootURI(d.boot.HTTPBootURI, d.cfg.HostAlias)
		if err != nil {
			return instance{}, err
		}
		if err := d.fw.Provision(ctx, d.cfg, vars, guestURI); err != nil {
			return instance{}, err
		}
	} else {
		if err := prepareVarsFile(d.cfg, vars); err != nil {
			return instance{}, err
		}
	}

	spec := launchSpec{
		VarsPath:  vars,
		QMPSock:   qmpSock,
		SerialLog: serialLog,
	}
	if m, ok := d.media[mediaDevice]; ok && m.Inserted {
		spec.MediaImage = m.Image
	}

	args := buildArgs(d.cfg, spec)
	cmd := exec.Command(d.cfg.Binary, args...)
	cmd.Stdout = os.Stderr // qemu diagnostics go to our stderr
	cmd.Stderr = os.Stderr
	return instance{cmd: cmd, dir: dir, qmpSock: qmpSock, serialLog: serialLog}, nil
}

// mediaDevice is the single virtual media device id the driver models.
const mediaDevice = "Cd"

// supervise owns cmd.Wait and the QMP event pump for one instance. It runs
// until the process exits, then clears the On-state fields and closes done.
func (d *Driver) supervise(cmd *exec.Cmd, conn *qmpConn, dir, serialLog string, done chan struct{}) {
	defer close(done)

	// Pump QMP events (best-effort; the stream ends when QEMU exits).
	evCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if ch, err := conn.events(evCtx); err == nil {
		go d.pumpQMP(ch)
	}

	// Watch the serial log for the boot marker (best-effort).
	go d.watchSerial(evCtx, serialLog)

	waitErr := cmd.Wait()

	d.mu.Lock()
	// Only clear state if this instance is still the current one (guards against
	// a race with a concurrent PowerOn after PowerOff).
	if d.cmd == cmd {
		d.state = hypervisor.PowerOff
		d.cmd = nil
		d.qmp = nil
		d.instDir = ""
		d.serialLog = ""
		if !d.closed {
			if waitErr != nil && !isExpectedExit(waitErr) {
				d.emitLocked(hypervisor.EvCrashed, waitErr.Error())
			}
			d.emitLocked(hypervisor.EvPoweredOff, "")
		}
	}
	d.mu.Unlock()

	_ = conn.close()
	// Keep serial.log for post-mortem assertions; remove sockets/vars.
	_ = os.Remove(filepath.Join(dir, "qmp.sock"))
}

func (d *Driver) pumpQMP(ch <-chan qmp.Event) {
	for ev := range ch {
		switch ev.Event {
		case "RESET":
			d.mu.Lock()
			d.emitLocked(hypervisor.EvGuestBooted, "qmp:RESET")
			d.mu.Unlock()
		}
	}
}

// watchSerial tails serialLog until ctx is done, emitting EvGuestBooted when the
// boot marker first appears.
func (d *Driver) watchSerial(ctx context.Context, serialLog string) {
	seen := false
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if seen {
				continue
			}
			b, err := os.ReadFile(serialLog)
			if err != nil {
				continue
			}
			if strings.Contains(string(b), bootMarker) {
				seen = true
				d.mu.Lock()
				d.emitLocked(hypervisor.EvGuestBooted, "serial:"+bootMarker)
				d.mu.Unlock()
			}
		}
	}
}

func (d *Driver) PowerOff(ctx context.Context, graceful bool) error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return fmt.Errorf("qemu: driver closed")
	}
	if d.state == hypervisor.PowerOff || d.cmd == nil {
		d.mu.Unlock()
		return nil
	}
	d.state = hypervisor.PowerPoweringOff
	conn := d.qmp
	done := d.doneCh
	d.mu.Unlock()

	if graceful && conn != nil {
		_ = conn.systemPowerdown()
		select {
		case <-done:
			return nil
		case <-time.After(gracefulTimeout):
			// fall through to force
		case <-ctx.Done():
			// fall through to force
		}
	}

	d.mu.Lock()
	d.forceKillLocked()
	d.mu.Unlock()

	if done != nil {
		<-done
	}
	return nil
}

// forceKillLocked terminates the current instance. Caller holds d.mu.
func (d *Driver) forceKillLocked() {
	if d.qmp != nil {
		_ = d.qmp.quit()
	}
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
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
	return d.state, nil
}

func (d *Driver) SetBootOverride(_ context.Context, o hypervisor.BootOverride) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fmt.Errorf("qemu: driver closed")
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
		return fmt.Errorf("qemu: driver closed")
	}
	m.Inserted = true
	d.media[m.DeviceID] = m
	return nil
}

func (d *Driver) EjectMedia(_ context.Context, deviceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.media, deviceID)
	return nil
}

func (d *Driver) ListMedia(context.Context) ([]hypervisor.MediaState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]hypervisor.MediaState, 0, len(d.media))
	for _, m := range d.media {
		via := "NotConnected"
		if m.Image != "" {
			via = "URI"
		}
		out = append(out, hypervisor.MediaState{MediaSpec: m, ConnectedVia: via})
	}
	return out, nil
}

func (d *Driver) Status(context.Context) (hypervisor.VMStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := hypervisor.VMStatus{
		Power:         d.state,
		Boot:          d.boot,
		StartedAtUnix: d.startedAt,
	}
	if d.cmd != nil && d.cmd.Process != nil {
		st.PID = d.cmd.Process.Pid
		st.QMPConnected = d.qmp != nil
	}
	if d.serialLog != "" {
		if b, err := os.ReadFile(d.serialLog); err == nil {
			st.SerialLog = string(b)
		}
	}
	return st, nil
}

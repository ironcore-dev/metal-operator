package qemu

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/go-qemu/qmp"
)

// qmpConn wraps a go-qemu SocketMonitor with the small command set the driver
// needs and exposes the raw event channel.
type qmpConn struct {
	mon *qmp.SocketMonitor
}

// dialQMP connects to the QMP unix socket at path, retrying until deadline
// because QEMU creates the socket asynchronously after launch. It completes the
// QMP capabilities handshake before returning.
func dialQMP(ctx context.Context, path string, timeout time.Duration) (*qmpConn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mon, err := qmp.NewSocketMonitor("unix", path, 2*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Connect performs the greeting + qmp_capabilities handshake.
		if err := mon.Connect(); err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return &qmpConn{mon: mon}, nil
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return nil, fmt.Errorf("qemu: connect QMP %s: %w", path, lastErr)
}

// events returns the QMP event stream.
func (q *qmpConn) events(ctx context.Context) (<-chan qmp.Event, error) {
	return q.mon.Events(ctx)
}

func (q *qmpConn) run(cmd string) error {
	_, err := q.mon.Run([]byte(`{"execute":"` + cmd + `"}`))
	return err
}

// systemPowerdown requests an ACPI graceful shutdown.
func (q *qmpConn) systemPowerdown() error { return q.run("system_powerdown") }

// quit terminates the QEMU process immediately.
func (q *qmpConn) quit() error { return q.run("quit") }

// close disconnects the monitor. Safe to call once.
func (q *qmpConn) close() error {
	if q.mon == nil {
		return nil
	}
	return q.mon.Disconnect()
}

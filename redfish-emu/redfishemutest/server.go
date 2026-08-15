// Package redfishemutest provides an in-process, httptest-style fixture that stands
// up the Redfish emulator (server + a hypervisor driver) plus a boot server
// serving the guest's boot artifact. A BMC client under test points at the
// Server's BaseURL; assertions can confirm the guest firmware actually fetched
// its boot image.
package redfishemutest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"time"

	"github.com/ironcore-dev/metal-operator/redfish-emu/driver/fake"
	"github.com/ironcore-dev/metal-operator/redfish-emu/driver/qemu"
	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
	"github.com/ironcore-dev/metal-operator/redfish-emu/redfish"
)

// DriverKind selects the south-side driver the server wires up.
type DriverKind int

const (
	// DriverFake uses the in-memory fake driver: fast, no QEMU, no real boot.
	// This is the default and is right for exercising the Redfish surface.
	DriverFake DriverKind = iota
	// DriverQEMU spawns a real QEMU VM that performs a genuine UEFI HTTP boot.
	// Slower (TCG on Apple Silicon for foreign arch) and requires QEMU + OVMF +
	// virt-fw-vars; use for real end-to-end boot tests.
	DriverQEMU
)

// Options configures a Server.
type Options struct {
	// HypervisorTimeout is the timeout for any hypervisor operation.
	// Defaults to 5 seconds.
	HypervisorTimeout time.Duration
	Driver            DriverKind
	// QEMU is used only when Driver == DriverQEMU.
	QEMU qemu.Config
	// BootArtifactPath is a file served as "/boot.efi" by the boot server. When
	// empty, a placeholder artifact is served (fine for DriverFake).
	BootArtifactPath string
	// SystemID is the Redfish System id; defaults to "1".
	SystemID string
}

// Server is a running emulator fixture.
type Server struct {
	// BaseURL is the Redfish root, e.g. http://127.0.0.1:PORT/redfish/v1.
	BaseURL string
	// Client is an HTTP client for talking to BaseURL.
	Client *http.Client
	// BootURL is the guest-facing URL of the boot artifact (host rewritten to
	// the QEMU user-net alias so a real guest can reach it).
	BootURL string
	// Hyp is the underlying hypervisor, exposed for direct assertions.
	Hyp hypervisor.Hypervisor
	// Events is the hypervisor's lifecycle event stream.
	Events <-chan hypervisor.Event

	boot *bootServer
	ts   *httptest.Server
}

const bootArtifactPath = "/boot.efi"

// Start builds and starts a Server. It registers cleanup on t so callers only
// need `defer h.Stop()` if they want an early teardown.
func Start(ctx context.Context, opts Options) *Server {
	if opts.SystemID == "" {
		opts.SystemID = "1"
	}

	// Load the boot artifact (or a placeholder).
	artifact := []byte("PLACEHOLDER-BOOT-ARTIFACT")
	if opts.BootArtifactPath != "" {
		b, err := os.ReadFile(opts.BootArtifactPath)
		if err != nil {
			panic(fmt.Sprintf("redfishemutest.Server: read boot artifact: %v", err))
		}
		artifact = b
	}
	bs := newBootServer(map[string][]byte{bootArtifactPath: artifact})

	// Build the driver.
	var hyp hypervisor.Hypervisor
	switch opts.Driver {
	case DriverQEMU:
		d, err := qemu.New(opts.QEMU, nil)
		if err != nil {
			bs.close()
			panic(fmt.Sprintf("redfishemutest.Server: create qemu: %v", err))
		}
		hyp = d
	default:
		hyp = fake.New()
	}
	if err := hyp.Prepare(ctx); err != nil {
		bs.close()
		panic(fmt.Sprintf("redfishemutest.Server: prepare: hypervisor %v", err))
	}

	srv := redfish.NewServer(redfish.Config{
		Systems: []redfish.System{{ID: opts.SystemID, Name: "Emulated System", Hyp: hyp}},
	})
	ts := httptest.NewServer(srv)

	h := &Server{
		BaseURL: ts.URL + "/redfish/v1",
		Client:  ts.Client(),
		BootURL: bs.URL() + bootArtifactPath,
		Hyp:     hyp,
		Events:  hyp.Events(),
		boot:    bs,
		ts:      ts,
	}
	return h
}

// Stop tears down the server: powers off and closes the hypervisor, and shuts
// down both HTTP servers. It is safe to call more than once.
func (h *Server) Stop() {
	if h.ts != nil {
		h.ts.Close()
		h.ts = nil
	}
	if h.Hyp != nil {
		_ = h.Hyp.Close()
	}
	if h.boot != nil {
		h.boot.close()
		h.boot = nil
	}
}

// BootRequests returns the requests the boot server has observed for the boot
// artifact.
func (h *Server) BootRequests() []BootRequest {
	return h.boot.requestsFor(bootArtifactPath)
}

// FirmwareFetched reports whether a fetch with a UEFI-HTTP-boot User-Agent was
// observed, distinguishing a genuine firmware fetch from any other download.
func (h *Server) FirmwareFetched() bool {
	return slices.ContainsFunc(h.BootRequests(), looksLikeFirmwareFetch)
}

// SerialLog returns the guest serial console captured so far (empty for the
// fake driver).
func (h *Server) SerialLog(ctx context.Context) (string, error) {
	st, err := h.Hyp.Status(ctx)
	if err != nil {
		return "", err
	}
	return st.SerialLog, nil
}

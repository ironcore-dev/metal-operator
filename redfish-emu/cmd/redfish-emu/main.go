// Command redfish-emu runs the Redfish BMC emulator as a standalone HTTP server
// backed by a real QEMU virtual machine. A Redfish client can drive it to set a
// UEFI HTTP boot target and power the machine on, causing the guest firmware to
// perform a genuine HTTP boot.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ironcore-dev/metal-operator/redfish-emu/driver/fake"
	"github.com/ironcore-dev/metal-operator/redfish-emu/driver/qemu"
	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
	"github.com/ironcore-dev/metal-operator/redfish-emu/redfish"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		addr      = flag.String("addr", ":8000", "listen address for the Redfish API")
		systemID  = flag.String("system-id", "1", "Redfish System id to expose")
		driver    = flag.String("driver", "qemu", "south-side driver: qemu | fake")
		arch      = flag.String("arch", "", "guest arch (aarch64|x86_64); default host-friendly")
		machine   = flag.String("machine", "", "QEMU machine type; default per-arch")
		accel     = flag.String("accel", "", "QEMU accelerator; default per-arch/host")
		memoryMB  = flag.Int("memory", 2048, "guest memory in MiB")
		smp       = flag.Int("smp", 2, "guest vCPUs")
		fwCode    = flag.String("firmware-code", "", "OVMF/edk2 code image; default auto-detected")
		fwVars    = flag.String("firmware-vars", "", "OVMF/edk2 vars template; default auto-detected")
		workDir   = flag.String("work-dir", "", "scratch dir for per-VM state; default a temp dir")
		virtFwBin = flag.String("virt-fw-vars", "virt-fw-vars", "virt-fw-vars binary for NVRAM injection")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	hyp, err := buildHypervisor(*driver, qemu.Config{
		Arch: *arch, Machine: *machine, Accel: *accel,
		MemoryMB: *memoryMB, SMP: *smp, WorkDir: *workDir,
		VirtFwVarsBin: *virtFwBin,
		Firmware:      qemu.Firmware{Code: *fwCode, VarsTemplate: *fwVars},
	})
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := hyp.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare hypervisor: %w", err)
	}
	defer hyp.Close()

	// Log lifecycle events.
	go func() {
		for ev := range hyp.Events() {
			logger.Info("hypervisor event", "kind", ev.Kind, "message", ev.Message)
		}
	}()

	srv := redfish.NewServer(redfish.Config{
		Systems: []redfish.System{{ID: *systemID, Name: "Emulated System", Hyp: hyp}},
	})
	httpSrv := &http.Server{Addr: *addr, Handler: srv}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutCtx, c := context.WithTimeout(context.Background(), 25*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	logger.Info("redfish emulator listening",
		"addr", *addr, "driver", *driver,
		"serviceRoot", "http://"+*addr+"/redfish/v1",
		"system", "/redfish/v1/Systems/"+*systemID)

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func buildHypervisor(kind string, cfg qemu.Config) (hypervisor.Hypervisor, error) {
	switch kind {
	case "qemu":
		return qemu.New(cfg, nil)
	case "fake":
		return fake.New(), nil
	default:
		return nil, fmt.Errorf("unknown driver %q (want qemu|fake)", kind)
	}
}

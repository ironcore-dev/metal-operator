package qemu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// VarsProvisioner produces a per-VM writable OVMF NVRAM vars file with a
// one-shot UEFI HTTP-boot entry injected for the given boot URI. Implementations
// differ only in how they write the EFI variables.
type VarsProvisioner interface {
	// Provision creates the writable vars file at dst (deriving its initial
	// contents from the Config's firmware template), injects a one-shot
	// HTTP-boot Boot#### entry pointing at guestURI, sets BootNext to it, and
	// returns. guestURI is already rewritten for guest-side networking.
	Provision(ctx context.Context, cfg Config, dst, guestURI string) error
}

// prepareVarsFile copies the firmware vars template to dst, or synthesizes a
// zero-filled vars file when the platform ships no matching template (e.g.
// Homebrew's aarch64 build). It returns the path to the writable file.
func prepareVarsFile(cfg Config, dst string) error {
	if cfg.Firmware.VarsTemplate != "" {
		return copyFile(cfg.Firmware.VarsTemplate, dst)
	}
	// No template: create a zero-filled file matching the code image size,
	// which is the edk2 pflash pairing convention on this platform.
	size := cfg.Firmware.VarsSize
	if size == 0 {
		fi, err := os.Stat(cfg.Firmware.Code)
		if err != nil {
			return fmt.Errorf("qemu: stat firmware code to size vars: %w", err)
		}
		size = fi.Size()
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return fmt.Errorf("qemu: size vars file: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// VirtFwVarsProvisioner injects the HTTP-boot entry by shelling out to
// virt-fw-vars (from the virt-firmware project). This is the day-1 implementation;
// a pure-Go EFI variable writer can replace it later to drop the dependency.
type VirtFwVarsProvisioner struct{}

var _ VarsProvisioner = VirtFwVarsProvisioner{}

// ErrVirtFwVarsMissing indicates the virt-fw-vars binary was not found. Callers
// (and tests) can use it to skip the firmware-HTTP path gracefully.
var ErrVirtFwVarsMissing = errors.New("qemu: virt-fw-vars not found on PATH")

func (VirtFwVarsProvisioner) Provision(ctx context.Context, cfg Config, dst, guestURI string) error {
	bin := cfg.VirtFwVarsBin
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%w (set Config.VirtFwVarsBin or install virt-firmware)", ErrVirtFwVarsMissing)
	}
	if err := prepareVarsFile(cfg, dst); err != nil {
		return err
	}
	// virt-fw-vars edits the vars file in place, adding a URI boot entry and
	// marking it as the next boot. --set-boot-uri adds a one-shot HTTP boot
	// Boot#### option and sets BootNext to it.
	cmd := exec.CommandContext(ctx, bin,
		"--inplace", dst,
		"--set-boot-uri", guestURI,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu: virt-fw-vars failed: %w: %s", err, string(out))
	}
	return nil
}

// varsPath returns the conventional per-VM vars file path within workdir.
func varsPath(workdir string) string { return filepath.Join(workdir, "vars.fd") }

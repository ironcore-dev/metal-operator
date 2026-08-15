package qemu

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// BootMode selects how the boot target reaches the guest.
type BootMode string

const (
	// BootFirmwareHTTP is the primary mode: the guest firmware (OVMF) performs
	// a genuine UEFI HTTP boot of the recorded HttpBootUri, driven by an
	// injected NVRAM boot entry. This exercises the client's HTTP-boot path.
	BootFirmwareHTTP BootMode = "FirmwareHTTP"
	// BootDirectKernel is a fallback that loads the artifact via QEMU's
	// -kernel/-initrd. No firmware HTTP fetch happens; it only smoke-tests the
	// Redfish plumbing. Not the default.
	BootDirectKernel BootMode = "DirectKernel"
)

// Firmware locates the OVMF/edk2 pflash images.
type Firmware struct {
	// Code is the read-only firmware code image (e.g. edk2-aarch64-code.fd).
	Code string
	// VarsTemplate is a writable NVRAM vars template that is copied per-VM.
	// It may be empty when the platform ships no matching vars file, in which
	// case a zero-filled vars file the size of Code is created.
	VarsTemplate string
	// VarsSize overrides the size of a synthesized vars file (bytes). When 0
	// and VarsTemplate is empty, the size of Code is used (the edk2 pflash
	// convention on this platform).
	VarsSize int64
}

// Config configures the QEMU driver. Zero values are filled by Defaults during
// New, so callers typically set only what they need to override.
type Config struct {
	Binary   string // qemu-system-<arch>; auto-detected from Arch
	Arch     string // guest arch: "aarch64" or "x86_64"; defaults to host-friendly
	Machine  string // "-machine" type; "virt" (aarch64) or "q35" (x86_64)
	Accel    string // accelerator spec, e.g. "hvf:tcg" or "tcg"
	CPU      string // "-cpu"; "host" with HVF, "max"/"qemu64" under TCG
	MemoryMB int
	SMP      int

	Firmware Firmware
	BootMode BootMode

	// HostAlias is the address the guest uses to reach the host's HTTP server
	// under user-mode networking (SLIRP). Defaults to "10.0.2.2".
	HostAlias string

	// WorkDir is the base directory for per-VM scratch (vars file, sockets,
	// serial log). Defaults to os.MkdirTemp when empty.
	WorkDir string

	// VirtFwVarsBin is the path/name of the virt-fw-vars binary used to inject
	// the HTTP-boot NVRAM entry. Defaults to "virt-fw-vars" (looked up on PATH).
	VirtFwVarsBin string

	// ExtraArgs are appended verbatim to the QEMU command line.
	ExtraArgs []string
}

// homebrewFirmwareDir is where QEMU installed via Homebrew keeps edk2 images on
// this platform. Kept as the first search location; distro paths follow.
var firmwareSearchDirs = []string{
	"/opt/homebrew/share/qemu",
	"/usr/local/share/qemu",
	"/usr/share/qemu",
	"/usr/share/edk2/x64",
	"/usr/share/OVMF",
}

// Defaults fills unset fields with values appropriate for the host. On
// Apple Silicon it prefers a same-arch (aarch64) guest so HVF can accelerate;
// elsewhere it defaults to x86_64.
func (c *Config) Defaults() error {
	if c.Arch == "" {
		if runtime.GOARCH == "arm64" {
			c.Arch = "aarch64"
		} else {
			c.Arch = "x86_64"
		}
	}
	if c.Binary == "" {
		c.Binary = "qemu-system-" + c.Arch
	}
	if c.Machine == "" {
		if c.Arch == "aarch64" {
			c.Machine = "virt"
		} else {
			c.Machine = "q35"
		}
	}
	sameArch := (c.Arch == "aarch64" && runtime.GOARCH == "arm64") ||
		(c.Arch == "x86_64" && runtime.GOARCH == "amd64")
	if c.Accel == "" {
		if sameArch && runtime.GOOS == "darwin" {
			c.Accel = "hvf:tcg"
		} else {
			c.Accel = "tcg"
		}
	}
	if c.CPU == "" {
		// "host" is only valid with hardware acceleration; under pure TCG for a
		// foreign arch, use a safe generic model.
		if sameArch {
			c.CPU = "host"
		} else if c.Arch == "aarch64" {
			c.CPU = "cortex-a72"
		} else {
			c.CPU = "qemu64"
		}
	}
	if c.MemoryMB == 0 {
		c.MemoryMB = 2048
	}
	if c.SMP == 0 {
		c.SMP = 2
	}
	if c.BootMode == "" {
		c.BootMode = BootFirmwareHTTP
	}
	if c.HostAlias == "" {
		c.HostAlias = "10.0.2.2"
	}
	if c.VirtFwVarsBin == "" {
		c.VirtFwVarsBin = "virt-fw-vars"
	}
	if c.Firmware.Code == "" {
		code, vars, err := detectFirmware(c.Arch)
		if err != nil {
			return err
		}
		c.Firmware.Code = code
		if c.Firmware.VarsTemplate == "" {
			c.Firmware.VarsTemplate = vars
		}
	}
	return nil
}

// detectFirmware locates the code image (and a matching vars template if one
// exists) for the given arch across known install locations.
func detectFirmware(arch string) (code, vars string, err error) {
	// Candidate code filenames, most specific first.
	var codeNames []string
	switch arch {
	case "aarch64":
		codeNames = []string{"edk2-aarch64-code.fd", "AAVMF_CODE.fd", "QEMU_EFI.fd"}
	case "x86_64":
		codeNames = []string{"edk2-x86_64-code.fd", "OVMF_CODE.fd", "OVMF_CODE_4M.fd"}
	default:
		return "", "", fmt.Errorf("qemu: no firmware defaults for arch %q", arch)
	}
	for _, dir := range firmwareSearchDirs {
		for _, name := range codeNames {
			p := filepath.Join(dir, name)
			if fileExists(p) {
				return p, detectVars(dir, arch), nil
			}
		}
	}
	return "", "", fmt.Errorf("qemu: no OVMF/edk2 code image found for arch %q in %v (set Firmware.Code)", arch, firmwareSearchDirs)
}

// detectVars returns a matching vars template in dir, or "" if none ships (the
// driver then synthesizes a zero-filled vars file). On Homebrew, x86_64 has no
// dedicated vars file; the i386 vars file is the correct same-size companion.
func detectVars(dir, arch string) string {
	var names []string
	switch arch {
	case "aarch64":
		// edk2-arm-vars.fd is Homebrew's aarch64 vars pairing (it ships no
		// edk2-aarch64-vars.fd); without it detectVars returns "" and
		// prepareVarsFile zero-fills a code-sized file, which virt-fw-vars
		// cannot parse (an all-zero file is not a valid EFI varstore).
		names = []string{"edk2-aarch64-vars.fd", "edk2-arm-vars.fd", "AAVMF_VARS.fd"}
	case "x86_64":
		names = []string{"edk2-x86_64-vars.fd", "edk2-i386-vars.fd", "OVMF_VARS.fd", "OVMF_VARS_4M.fd"}
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

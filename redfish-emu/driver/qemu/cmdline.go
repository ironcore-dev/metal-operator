package qemu

import (
	"fmt"
	"net/url"
	"strconv"
)

// launchSpec carries the per-VM paths and boot decision that, combined with a
// resolved Config, fully determine the QEMU command line. It is produced by the
// driver at PowerOn time.
type launchSpec struct {
	VarsPath   string // per-VM writable NVRAM pflash
	QMPSock    string // unix socket path for the QMP monitor
	SerialLog  string // file the guest serial console is written to
	MediaImage string // optional CD image path/URI to attach (empty if none)

	// Direct-kernel fallback inputs (only used when BootMode == BootDirectKernel).
	KernelPath string
	InitrdPath string
	KernelArgs string
}

// buildArgs assembles the qemu-system-* argument vector (excluding argv[0]).
// It is pure: given the same Config and launchSpec it always yields the same
// slice, which makes it straightforward to table-test.
func buildArgs(c Config, s launchSpec) []string {
	args := []string{
		"-machine", c.Machine + ",accel=" + c.Accel,
		"-cpu", c.CPU,
		"-m", strconv.Itoa(c.MemoryMB),
		"-smp", strconv.Itoa(c.SMP),
		"-nodefaults",
		"-no-reboot",
	}

	// UEFI firmware: read-only code + per-VM writable vars, both as pflash.
	args = append(args,
		"-drive", "if=pflash,format=raw,readonly=on,file="+c.Firmware.Code,
		"-drive", "if=pflash,format=raw,file="+s.VarsPath,
	)

	// User-mode networking. The guest reaches the host HTTP server at
	// c.HostAlias (default 10.0.2.2). No privileges or bridge required.
	args = append(args,
		"-netdev", "user,id=net0",
		"-device", "virtio-net-pci,netdev=net0",
	)

	// Entropy source so the OVMF NetworkPkg TLS/entropy paths don't stall.
	args = append(args, "-device", "virtio-rng-pci")

	// Optional virtual media as a CD-ROM.
	if s.MediaImage != "" {
		args = append(args,
			"-drive", "if=none,id=cd0,media=cdrom,readonly=on,file="+s.MediaImage,
			"-device", "virtio-blk-pci,drive=cd0",
		)
	}

	// Direct-kernel fallback boot (no firmware HTTP fetch).
	if c.BootMode == BootDirectKernel && s.KernelPath != "" {
		args = append(args, "-kernel", s.KernelPath)
		if s.InitrdPath != "" {
			args = append(args, "-initrd", s.InitrdPath)
		}
		if s.KernelArgs != "" {
			args = append(args, "-append", s.KernelArgs)
		}
	}

	// Serial console captured to a file for boot assertions.
	args = append(args, "-serial", "file:"+s.SerialLog)

	// QMP monitor over a unix socket; do not wait for a client at startup.
	args = append(args, "-qmp", "unix:"+s.QMPSock+",server=on,wait=off")

	// Headless.
	args = append(args, "-display", "none")

	args = append(args, c.ExtraArgs...)
	return args
}

// guestBootURI rewrites a host-facing boot URI so the guest can reach it under
// user-mode networking, replacing a loopback/host host component with the
// configured HostAlias. Only the host part is adjusted; scheme, port, and path
// are preserved. Non-loopback hosts are returned unchanged.
func guestBootURI(hostURI, hostAlias string) (string, error) {
	u, err := url.Parse(hostURI)
	if err != nil {
		return "", fmt.Errorf("qemu: invalid boot URI %q: %w", hostURI, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("qemu: boot URI %q has no host", hostURI)
	}
	switch u.Hostname() {
	// "::" is the IPv6 any-address; net.Listen("tcp", "0.0.0.0:0") on a
	// dual-stack stack returns Addr "[::]:<port>", so a daemon that binds
	// 0.0.0.0 hands OVMF a http://[::]:port/ URI that the guest cannot route.
	// Treat it like the other loopback/any hosts and rewrite to HostAlias.
	case "127.0.0.1", "localhost", "::1", "0.0.0.0", "::":
		if p := u.Port(); p != "" {
			u.Host = hostAlias + ":" + p
		} else {
			u.Host = hostAlias
		}
	}
	return u.String(), nil
}

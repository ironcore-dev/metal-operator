package qemu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestConfigDefaults_AppleSiliconPrefersAarch64(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("defaults branch specific to Apple Silicon")
	}
	c := Config{Firmware: Firmware{Code: "/dev/null"}} // skip firmware detection
	if err := c.Defaults(); err != nil {
		t.Fatal(err)
	}
	if c.Arch != "aarch64" {
		t.Errorf("Arch = %q, want aarch64", c.Arch)
	}
	if c.Binary != "qemu-system-aarch64" {
		t.Errorf("Binary = %q", c.Binary)
	}
	if c.Machine != "virt" {
		t.Errorf("Machine = %q, want virt", c.Machine)
	}
	if !strings.Contains(c.Accel, "hvf") {
		t.Errorf("Accel = %q, want hvf preferred", c.Accel)
	}
	if c.CPU != "host" {
		t.Errorf("CPU = %q, want host (same-arch HVF)", c.CPU)
	}
}

func TestConfigDefaults_ForeignArchUsesTCG(t *testing.T) {
	c := Config{Arch: "x86_64", Firmware: Firmware{Code: "/dev/null"}}
	if err := c.Defaults(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOARCH != "amd64" {
		if c.Accel != "tcg" {
			t.Errorf("Accel = %q, want tcg for foreign arch", c.Accel)
		}
		if c.CPU == "host" {
			t.Errorf("CPU = host is invalid under TCG for foreign arch")
		}
	}
}

func TestBuildArgs_FirmwareHTTP(t *testing.T) {
	c := Config{
		Binary: "qemu-system-aarch64", Machine: "virt", Accel: "hvf:tcg",
		CPU: "host", MemoryMB: 2048, SMP: 2, BootMode: BootFirmwareHTTP,
		Firmware: Firmware{Code: "/fw/code.fd"},
	}
	spec := launchSpec{
		VarsPath: "/work/vars.fd", QMPSock: "/work/qmp.sock", SerialLog: "/work/serial.log",
	}
	args := buildArgs(c, spec)
	joined := strings.Join(args, " ")

	wantContains := []string{
		"-machine virt,accel=hvf:tcg",
		"-no-reboot",
		"if=pflash,format=raw,readonly=on,file=/fw/code.fd",
		"if=pflash,format=raw,file=/work/vars.fd",
		"user,id=net0",
		"virtio-net-pci,netdev=net0",
		"virtio-rng-pci",
		"file:/work/serial.log",
		"unix:/work/qmp.sock,server=on,wait=off",
		"-display none",
	}
	for _, w := range wantContains {
		if !strings.Contains(joined, w) {
			t.Errorf("args missing %q\n got: %s", w, joined)
		}
	}
	// No -kernel in firmware-HTTP mode.
	if slices.Contains(args, "-kernel") {
		t.Errorf("firmware-HTTP mode should not use -kernel: %s", joined)
	}
}

func TestBuildArgs_DirectKernelFallback(t *testing.T) {
	c := Config{
		Binary: "qemu-system-x86_64", Machine: "q35", Accel: "tcg", CPU: "qemu64",
		MemoryMB: 1024, SMP: 1, BootMode: BootDirectKernel,
		Firmware: Firmware{Code: "/fw/code.fd"},
	}
	spec := launchSpec{
		VarsPath: "/w/vars.fd", QMPSock: "/w/q.sock", SerialLog: "/w/s.log",
		KernelPath: "/w/vmlinuz", InitrdPath: "/w/initrd", KernelArgs: "console=ttyS0",
	}
	args := buildArgs(c, spec)
	joined := strings.Join(args, " ")
	for _, w := range []string{"-kernel /w/vmlinuz", "-initrd /w/initrd", "-append console=ttyS0"} {
		if !strings.Contains(joined, w) {
			t.Errorf("args missing %q\n got: %s", w, joined)
		}
	}
}

func TestBuildArgs_MediaAttached(t *testing.T) {
	c := Config{Binary: "q", Machine: "virt", Accel: "tcg", CPU: "max", MemoryMB: 512, SMP: 1,
		BootMode: BootFirmwareHTTP, Firmware: Firmware{Code: "/fw/code.fd"}}
	spec := launchSpec{VarsPath: "/v", QMPSock: "/q", SerialLog: "/s", MediaImage: "/img.iso"}
	joined := strings.Join(buildArgs(c, spec), " ")
	if !strings.Contains(joined, "media=cdrom,readonly=on,file=/img.iso") {
		t.Errorf("media not attached: %s", joined)
	}
}

func TestGuestBootURI(t *testing.T) {
	cases := []struct {
		in, alias, want string
	}{
		{"http://127.0.0.1:8080/boot.efi", "10.0.2.2", "http://10.0.2.2:8080/boot.efi"},
		{"http://localhost:9000/x", "10.0.2.2", "http://10.0.2.2:9000/x"},
		{"http://127.0.0.1/boot.efi", "10.0.2.2", "http://10.0.2.2/boot.efi"},
		// IPv6 any-address (what net.Listen("0.0.0.0:0") returns on a dual-stack
		// stack) is rewritten too, else the guest cannot route it.
		{"http://[::]:8080/boot.efi", "10.0.2.2", "http://10.0.2.2:8080/boot.efi"},
		{"http://[::]/boot.efi", "10.0.2.2", "http://10.0.2.2/boot.efi"},
		{"http://192.168.1.5:80/img", "10.0.2.2", "http://192.168.1.5:80/img"}, // non-loopback unchanged
	}
	for _, tc := range cases {
		got, err := guestBootURI(tc.in, tc.alias)
		if err != nil {
			t.Errorf("guestBootURI(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("guestBootURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGuestBootURI_Invalid(t *testing.T) {
	if _, err := guestBootURI("://nope", "10.0.2.2"); err == nil {
		t.Error("expected error for malformed URI")
	}
}

// TestPrepareVarsFile_Synthesized verifies that with no template a zero-filled
// vars file matching the code image size is produced.
func TestPrepareVarsFile_Synthesized(t *testing.T) {
	dir := t.TempDir()
	code := filepath.Join(dir, "code.fd")
	if err := os.WriteFile(code, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Firmware: Firmware{Code: code}} // no VarsTemplate
	dst := filepath.Join(dir, "vars.fd")
	if err := prepareVarsFile(cfg, dst); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 4096 {
		t.Errorf("vars size = %d, want 4096 (match code)", fi.Size())
	}
}

func TestPrepareVarsFile_FromTemplate(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "vars-template.fd")
	content := []byte("VARSDATA")
	if err := os.WriteFile(tmpl, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Firmware: Firmware{Code: "/unused", VarsTemplate: tmpl}}
	dst := filepath.Join(dir, "vars.fd")
	if err := prepareVarsFile(cfg, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("copied vars = %q, want %q", got, content)
	}
}

// TestVirtFwVarsMissing confirms the provisioner returns a sentinel error the
// harness can detect when virt-fw-vars is not installed.
func TestVirtFwVarsMissing(t *testing.T) {
	cfg := Config{VirtFwVarsBin: "definitely-not-a-real-binary-xyz", Firmware: Firmware{Code: "/x"}}
	err := VirtFwVarsProvisioner{}.Provision(context.Background(), cfg, filepath.Join(t.TempDir(), "v.fd"), "http://10.0.2.2/boot.efi")
	if err == nil {
		t.Fatal("expected error when virt-fw-vars is missing")
	}
	if !errors.Is(err, ErrVirtFwVarsMissing) {
		t.Errorf("error = %v, want ErrVirtFwVarsMissing", err)
	}
}

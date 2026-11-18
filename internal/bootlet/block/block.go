package block

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jaypipes/ghw/pkg/block"
)

// ---------------------------------------------------------------------------
// Disk discovery + selection (≈ list_block_devices + match_root_device_hints
// + guess_root_disk + get_os_install_device)
// ---------------------------------------------------------------------------

type Device struct {
	Name       string // "sda"
	Path       string // "/dev/sda"
	SizeBytes  int64
	Rotational bool
	Model      string
	Serial     string
	WWN        string
	Controller string // scsi / nvme / virtio / ...
	Removable  bool
	ReadOnly   bool // /sys/block/<name>/ro - optical / virtual media
	Mounted    bool // any partition of this disk currently has a mount point
}

// skip pseudo-devices; note sr* — the BMC virtual-media CD itself must never
// be an install target.
var notAnInstallTarget = regexp.MustCompile(`^(loop|ram|dm-|zram|sr|fd|md)`)

func normUnknown(s string) string {
	if s == "unknown" {
		return ""
	}
	return s
}

// ListDevices — does /sys/block walking
// /dev/disk/by-id resolving and /proc/mounts parsing.
func ListDevices() ([]Device, error) {
	info, err := block.New()
	if err != nil {
		return nil, err
	}
	var out []Device
	for _, d := range info.Disks {
		if notAnInstallTarget.MatchString(d.Name) {
			continue
		}
		dev := Device{
			Name:       d.Name,
			Path:       "/dev/" + d.Name,
			SizeBytes:  int64(d.SizeBytes),
			Rotational: d.DriveType == block.DriveTypeHDD,
			Model:      normUnknown(d.Model),
			Serial:     normUnknown(d.SerialNumber),
			WWN:        normUnknown(d.WWN),
			Controller: d.StorageController.String(),
			Removable:  d.IsRemovable,
		}
		for _, p := range d.Partitions { // ghw parses the mount table for us
			if p.MountPoint != "" {
				dev.Mounted = true
				break
			}
		}
		if roFlag, err := os.ReadFile(filepath.Join("/sys/block", d.Name, "ro")); err == nil {
			dev.ReadOnly = strings.TrimSpace(string(roFlag)) == "1"
		}
		out = append(out, dev)
	}
	return out, nil
}

// GuessRootDisk mirrors utils.guess_root_disk: sort by (size, name) —
// name as tiebreak helps BIOSes that only try the first disk — smallest >= min.
func GuessRootDisk(devs []Device, minBytes int64) (*Device, error) {
	sort.Slice(devs, func(i, j int) bool {
		if devs[i].SizeBytes != devs[j].SizeBytes {
			return devs[i].SizeBytes < devs[j].SizeBytes
		}
		return devs[i].Name < devs[j].Name
	})
	for i := range devs {
		if devs[i].SizeBytes >= minBytes {
			return &devs[i], nil
		}
	}
	return nil, fmt.Errorf("no disk >= %d bytes among %d candidates", minBytes, len(devs))
}

// SelectDisk implements get_os_install_device(): hints first, else smallest
// fitting disk. Disks with a mounted partition, read-only disks (optical and
// virtual media present like this) and explicitly excluded devices (e.g. the
// config drive the bootlet itself booted from) are never install targets.
func SelectDisk(pred func(Device) bool, minBytes int64, exclude ...string) (*Device, error) {
	devs, err := ListDevices()
	if err != nil {
		return nil, err
	}
	excluded := make(map[string]bool, len(exclude))
	for _, dev := range exclude {
		excluded[dev] = true
		excluded[strings.TrimPrefix(dev, "/dev/")] = true
	}
	var free []Device
	for _, d := range devs {
		if d.Mounted || d.ReadOnly || excluded[d.Name] || excluded[d.Path] {
			continue
		}
		free = append(free, d)
	}
	if pred != nil {
		for i := range free {
			if pred(free[i]) {
				return &free[i], nil
			}
		}
		return nil, fmt.Errorf("no device matches predicate")
	}
	return GuessRootDisk(free, minBytes)
}

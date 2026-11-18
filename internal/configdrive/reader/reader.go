package reader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/ironcore-dev/metal-operator/internal/configdrive"
	"github.com/u-root/u-root/pkg/mount/block"
)

type Reader interface {
	Device() string
	MetadataPath() string
	UserdataPath() string
	NetworkDataPath() string
	FileSystem() filesystem.FileSystem
	Close() error
}

var formatByLabel = map[string]configdrive.Format{
	configdrive.VolumeLabelNoCloud:   configdrive.FormatNoCloud,
	configdrive.VolumeLabelOpenStack: configdrive.FormatOpenStack,
}

func Probe(ctx context.Context) (Reader, error) {
	rd, lastErr := ProbeOnce()
	if lastErr == nil {
		return rd, nil
	}

	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			if lastErr == nil {
				return nil, fmt.Errorf("no config drive found")
			}
			return nil, lastErr
		case <-t.C:
			rd, lastErr = ProbeOnce()
			if lastErr == nil {
				return rd, nil
			}
		}
	}
}

func ProbeOnce() (Reader, error) {
	devs, err := block.GetBlockDevices()
	if err != nil {
		return nil, fmt.Errorf("listing block devices: %w", err)
	}

	var errs []error
	for _, dev := range devs.FilterZeroSize() {
		rd, err := probeDevice(dev)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if rd == nil {
			continue
		}
		return rd, nil
	}
	return nil, errors.Join(errs...)
}

func probeDevice(dev *block.BlockDev) (rd Reader, retErr error) {
	devPath := dev.DevicePath()

	d, err := diskfs.Open(devPath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", devPath, err)
	}
	defer func() {
		if retErr != nil || rd == nil {
			_ = d.Close()
		}
	}()

	fs := probeFilesystem(d)
	if fs == nil {
		return nil, nil
	}
	defer func() {
		if retErr != nil || rd == nil {
			_ = fs.Close()
		}
	}()

	format := detectConfigDriveFormat(fs)
	switch format {
	case configdrive.FormatNoCloud:
		return &reader{
			device:          devPath,
			disk:            d,
			fs:              fs,
			metadataPath:    configdrive.NoCloudMetadataPath,
			userdataPath:    configdrive.NoCloudUserDataPath,
			networkDataPath: configdrive.NoCloudNetworkDataPath,
		}, nil
	case configdrive.FormatOpenStack:
		return &reader{
			device:          devPath,
			disk:            d,
			fs:              fs,
			metadataPath:    configdrive.OpenStackMetadataPath,
			userdataPath:    configdrive.OpenStackUserDataPath,
			networkDataPath: configdrive.OpenStackNetworkDataPath,
		}, nil
	default:
		return nil, nil
	}
}

type reader struct {
	device          string
	disk            *disk.Disk
	fs              filesystem.FileSystem
	metadataPath    string
	userdataPath    string
	networkDataPath string
}

func (r *reader) Device() string {
	return r.device
}

func (r *reader) MetadataPath() string {
	return r.metadataPath
}

func (r *reader) UserdataPath() string {
	return r.userdataPath
}

func (r *reader) NetworkDataPath() string {
	return r.networkDataPath
}

func (r *reader) FileSystem() filesystem.FileSystem {
	return r.fs
}

func (r *reader) Close() error {
	return errors.Join(
		r.fs.Close(),
		r.disk.Close(),
	)
}

func probeFilesystem(d *disk.Disk) filesystem.FileSystem {
	if fs, err := fat32.Read(d.Backend, d.Size, 0, d.LogicalBlocksize); err == nil {
		return fs
	}
	if fs, err := iso9660.Read(d.Backend, d.Size, 0, 0); err == nil {
		return fs
	}
	return nil
}

func detectConfigDriveFormat(fsys filesystem.FileSystem) configdrive.Format {
	switch fsys.Type() {
	case filesystem.TypeFat32, filesystem.TypeISO9660:
	default:
		return configdrive.FormatUnknown
	}

	label := strings.Trim(fsys.Label(), " \x00")
	for l, format := range formatByLabel {
		if strings.EqualFold(label, l) {
			return format
		}
	}
	return configdrive.FormatUnknown
}

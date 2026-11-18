package configdrive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

type Format uint8

const (
	FormatUnknown Format = iota
	FormatNoCloud
	FormatOpenStack
)

func (f Format) String() string {
	switch f {
	case FormatNoCloud:
		return "NoCloud"
	case FormatOpenStack:
		return "OpenStack"
	default:
		return "Unknown"
	}
}

const (
	VolumeLabelNoCloud   = "cidata"
	VolumeLabelOpenStack = "config-2"

	OpenStackMetadataPath    = "/openstack/latest/meta_data.json"
	OpenStackUserDataPath    = "/openstack/latest/user_data"
	OpenStackNetworkDataPath = "/ironcore/latest/network_data.json"

	NoCloudMetadataPath    = "/meta-data"
	NoCloudUserDataPath    = "/user-data"
	NoCloudNetworkDataPath = "/network-data"
)

// volumeLabels maps a config drive format to the volume label written to the
// image, following cloud-init / OpenStack conventions. Written verbatim:
// readers (blkid, cloud-init, bootlet's config loader) compare them verbatim
// or case-folded.
var volumeLabels = map[Format]string{
	FormatNoCloud:   VolumeLabelNoCloud,
	FormatOpenStack: VolumeLabelOpenStack,
}

var writeFilesFunctions = map[Format]func(filesystem.FileSystem, Args) error{
	FormatNoCloud:   writeNoCloud,
	FormatOpenStack: writeOpenstack,
}

type Metadata struct {
	ServerID string `json:"serverID"`
	Hostname string `json:"hostname"`
}

type Args struct {
	Metadata    Metadata
	UserData    io.Reader
	NetworkData io.Reader
}

type Builder struct {
	volumeLabel string
	writeFiles  func(filesystem.FileSystem, Args) error
}

type BuilderOptions struct {
	Format Format
}

type BuilderOption func(*BuilderOptions)

func WithFormat(format Format) BuilderOption {
	return func(o *BuilderOptions) {
		o.Format = format
	}
}

func NewBuilder(opts ...BuilderOption) (*Builder, error) {
	bOpts := &BuilderOptions{}
	for _, opt := range opts {
		opt(bOpts)
	}
	if bOpts.Format == FormatUnknown {
		bOpts.Format = FormatNoCloud
	}

	label, ok := volumeLabels[bOpts.Format]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", bOpts.Format)
	}

	writeFilesFunc, ok := writeFilesFunctions[bOpts.Format]
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", bOpts.Format)
	}

	return &Builder{
		volumeLabel: label,
		writeFiles:  writeFilesFunc,
	}, nil
}

func (b *Builder) VolumeLabel() string {
	return b.volumeLabel
}

func (b *Builder) WriteFiles(fs filesystem.FileSystem, args Args) error {
	return b.writeFiles(fs, args)
}

func (b *Builder) Build(w io.Writer, args Args) error {
	f, err := os.CreateTemp("", "configdrive-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if err := f.Close(); err != nil {
		return err
	}

	if err := b.BuildFile(f.Name(), args); err != nil {
		return err
	}

	rd, err := os.Open(f.Name())
	if err != nil {
		return err
	}
	defer func() { _ = rd.Close() }()

	_, err = io.Copy(w, rd)
	return err
}

// BuildFile builds an ISO9660 config drive image at filename. The image is
// sized to its content: ISO9660 images are staged in a workspace and only
// materialized on Finalize, so no up-front volume sizing is needed.
func (b *Builder) BuildFile(filename string, args Args) (retErr error) {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(filename)
		}
	}()

	bf := file.New(f, false)
	defer func() { _ = bf.Close() }()

	// Size zero means unbounded: the image grows to whatever Finalize writes.
	fsys, err := iso9660.Create(bf, 0, 0, 0, "")
	if err != nil {
		return fmt.Errorf("creating file system: %w", err)
	}
	defer func() { _ = fsys.Close() }() // cleans up the staging workspace

	if err := b.writeFiles(fsys, args); err != nil {
		return err
	}

	if err := fsys.Finalize(iso9660.FinalizeOptions{
		RockRidge:        true,
		Joliet:           true,
		VolumeIdentifier: b.volumeLabel,
	}); err != nil {
		return fmt.Errorf("finalizing ISO9660 image: %w", err)
	}

	return nil
}

func writeOpenstack(fsys filesystem.FileSystem, args Args) error {
	metadata := &openStackMetadata{
		UUID:     args.Metadata.ServerID,
		Hostname: args.Metadata.Hostname,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshalling openstack metadata: %w", err)
	}

	for _, part := range []struct {
		path string
		rd   io.Reader
	}{
		{path: OpenStackMetadataPath, rd: bytes.NewBuffer(metadataJSON)},
		{path: OpenStackUserDataPath, rd: args.UserData},
		{path: OpenStackNetworkDataPath, rd: args.NetworkData},
	} {
		if part.rd == nil {
			continue
		}

		if err := writeFile(fsys, part.path, part.rd); err != nil {
			return fmt.Errorf("writing file %s: %w", part.path, err)
		}
	}

	return nil
}

type openStackMetadata struct {
	UUID     string `json:"uuid"`
	Hostname string `json:"hostname,omitempty"`
}

type noCloudMetadata struct {
	InstanceID    string `json:"instance-id"`
	LocalHostname string `json:"local-hostname,omitempty"`
}

func writeNoCloud(fsys filesystem.FileSystem, args Args) error {
	metadata := &noCloudMetadata{
		InstanceID:    args.Metadata.ServerID,
		LocalHostname: args.Metadata.Hostname,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshalling nocloud metadata: %w", err)
	}

	userData := args.UserData
	if userData == nil {
		userData = bytes.NewBuffer([]byte{})
	}

	for _, part := range []struct {
		path string
		rd   io.Reader
	}{
		{path: NoCloudMetadataPath, rd: bytes.NewBuffer(metadataJSON)},
		{path: NoCloudUserDataPath, rd: userData},
		{path: NoCloudNetworkDataPath, rd: args.NetworkData},
	} {
		if part.rd == nil {
			continue
		}
		if err := writeFile(fsys, part.path, part.rd); err != nil {
			return fmt.Errorf("writing file %s: %w", part.path, err)
		}
	}
	return nil
}

func writeFile(fs filesystem.FileSystem, path string, r io.Reader) error {
	if dir := filepath.Dir(path); dir != "/" && dir != "." {
		if err := fs.Mkdir(dir); err != nil { // mkdir -p semantics
			return err
		}
	}

	f, err := fs.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

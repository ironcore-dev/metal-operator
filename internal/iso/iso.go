package iso

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/ironcore-dev/metal-operator/internal/blob"
	"github.com/ironcore-dev/metal-operator/internal/configdrive"
	"github.com/ironcore-dev/metal-operator/internal/initrd"
	"github.com/ironcore-dev/metal-operator/internal/uki"
)

const limineConfTemplate = `timeout: 0
   serial: yes

   /Linux
      protocol: linux
      path:        boot():/boot/vmlinuz
      module_path: boot():/boot/initrd.img
      cmdline: %s
   `

type Builder struct {
	biosSys    blob.Blob
	biosCD     blob.Blob
	ukiBuilder *uki.Builder

	workDir string
}

type BuilderOpts struct {
	BIOSSys    blob.Blob
	BIOSCD     blob.Blob
	UKIBuilder *uki.Builder
	WorkDir    string
}

func NewBuilder(opts BuilderOpts) (*Builder, error) {
	if opts.BIOSSys == nil {
		return nil, fmt.Errorf("must specify BIOSSys")
	}
	if opts.BIOSCD == nil {
		return nil, fmt.Errorf("must specify BIOSCD")
	}
	if opts.UKIBuilder == nil {
		return nil, fmt.Errorf("must specify UKIBuilder")
	}
	return &Builder{
		biosSys:    opts.BIOSSys,
		biosCD:     opts.BIOSCD,
		ukiBuilder: opts.UKIBuilder,
		workDir:    opts.WorkDir,
	}, nil
}

type BuildArgs struct {
	Architecture string
	Initrds      []blob.Sized
	Kernel       blob.Sized
	Cmdline      string
	OSRelease    string
	ConfigDrive  *configdrive.Args
}

var espBootNameByArch = map[string]string{
	"amd64": "BOOTX64.EFI",
	"arm64": "BOOTAA64.EFI",
}

// espSlack keeps the FAT data area big enough for firmware to classify the
// volume as FAT32: below 260 MiB go-diskfs uses 512-byte clusters, and edk2
// requires >= 65525 clusters (~32 MiB) to detect FAT32 rather than FAT16.
const espSlack = 33 << 20

// buildESPImage creates a FAT32 EFI system partition image at filename
// containing the UKI as the architecture-specific UEFI fallback bootloader
// (\EFI\BOOT\BOOT<arch>.EFI). The image is referenced from the ISO's El
// Torito EFI boot entry so firmware loads the UKI directly.
func (b *Builder) buildESPImage(arch, ukiFilename, filename string, ukiSize int64) error {
	bootName, ok := espBootNameByArch[arch]
	if !ok {
		return fmt.Errorf("unsupported architecture for UEFI boot: %q", arch)
	}

	size := (ukiSize + espSlack + 511) / 512 * 512
	d, err := createDisk(filename, size, diskfs.WithSectorSize(diskfs.SectorSize512))
	if err != nil {
		return fmt.Errorf("creating ESP image: %w", err)
	}
	defer func() { _ = d.Close() }()

	fs, err := d.CreateFilesystem(disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeFat32,
		VolumeLabel: "ESP",
	})
	if err != nil {
		return fmt.Errorf("creating FAT32 filesystem: %w", err)
	}

	if err := fs.Mkdir("/EFI/BOOT"); err != nil { // mkdir -p semantics
		return fmt.Errorf("creating /EFI/BOOT: %w", err)
	}

	ukiFile, err := os.Open(ukiFilename)
	if err != nil {
		return fmt.Errorf("opening %q: %w", ukiFilename, err)
	}
	defer func() { _ = ukiFile.Close() }()

	dstFilename := "/EFI/BOOT/" + bootName
	if err := writeFile(fs, dstFilename, ukiFile); err != nil {
		return fmt.Errorf("writing %q: %w", dstFilename, err)
	}
	return nil
}

func (b *Builder) buildUKI(ctx context.Context, filename string, args BuildArgs) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating uki file %q: %w", filename, err)
	}

	if err := b.ukiBuilder.Build(ctx, f, uki.BuildArgs{
		Architecture: args.Architecture,
		Initrds:      args.Initrds,
		Kernel:       args.Kernel,
		Cmdline:      args.Cmdline,
		OSRelease:    args.OSRelease,
	}); err != nil {
		_ = f.Close()
		return fmt.Errorf("building uki: %w", err)
	}

	return f.Close()
}

func (b *Builder) Build(ctx context.Context, filename string, args BuildArgs) (retErr error) {
	tempDir, err := os.MkdirTemp(b.workDir, "iso-build-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ukiFilename := filepath.Join(tempDir, "uki.efi")
	if err := b.buildUKI(ctx, ukiFilename, args); err != nil {
		return fmt.Errorf("building uki: %w", err)
	}

	ukiStat, err := os.Stat(ukiFilename)
	if err != nil {
		return fmt.Errorf("stating %q: %w", ukiFilename, err)
	}

	ukiSize := ukiStat.Size()
	espFilename := filepath.Join(tempDir, "esp.fat32")
	if err := b.buildESPImage(args.Architecture, ukiFilename, espFilename, ukiSize); err != nil {
		return fmt.Errorf("building ESP image: %w", err)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating %q: %w", filename, err)
	}
	backend := file.New(f, false)
	defer func() { retErr = errors.Join(retErr, backend.Close()) }()

	fs, err := iso9660.Create(backend, 0, 0, 0, "")
	if err != nil {
		return fmt.Errorf("creating ISO9660 filesystem: %w", err)
	}
	defer func() { _ = fs.Close() }()

	writeFile := func(path string, open func() (io.ReadCloser, error)) error {
		rc, err := open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()

		return writeFile(fs, path, rc)
	}

	initrds := make([]blob.Blob, 0, len(args.Initrds))
	for _, initrd := range args.Initrds {
		initrds = append(initrds, initrd)
	}

	files := map[string]func() (io.ReadCloser, error){
		"/boot/vmlinuz":    func() (io.ReadCloser, error) { return args.Kernel.Open(ctx) },
		"/boot/initrd.img": func() (io.ReadCloser, error) { return io.NopCloser(initrd.MergeReader(ctx, initrds)), nil },
		"/boot/limine/limine.conf": func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(fmt.Sprintf(limineConfTemplate, args.Cmdline))), nil
		},
		"/boot/limine/limine-bios.sys":    func() (io.ReadCloser, error) { return b.biosSys.Open(ctx) },
		"/boot/limine/limine-bios-cd.bin": func() (io.ReadCloser, error) { return b.biosCD.Open(ctx) },
		"/boot/efi.img":                   func() (io.ReadCloser, error) { return os.Open(espFilename) },
	}
	for path, data := range files {
		if err := writeFile(path, data); err != nil {
			return err
		}
	}

	// Limine's BIOS CD stage-1 locates itself via the El Torito boot info
	// table (bi_boot_LBA). BootTable: true makes go-diskfs patch that table
	// into the boot file in place — the equivalent of mkisofs
	// -boot-info-table — so no post-build "limine bios-install" is needed
	// for CD/virtual-media boot.
	biosEntry := &iso9660.ElToritoEntry{
		Platform:  iso9660.BIOS,
		Emulation: iso9660.NoEmulation,
		BootFile:  "/boot/limine/limine-bios-cd.bin",
		BootTable: true,
	}
	biosEntry.SetLoadSize(4) // returns nothing in go-diskfs v1.9

	if err := fs.Finalize(iso9660.FinalizeOptions{
		VolumeIdentifier: "BOOT",
		RockRidge:        true, // keeps lowercase names intact for Limine
		Joliet:           true,
		ElTorito: &iso9660.ElTorito{
			BootCatalog: "/boot.catalog",
			Entries: []*iso9660.ElToritoEntry{
				biosEntry, // legacy BIOS boot via Limine
				// UEFI boot: firmware mounts efi.img and loads the UKI inside it
				{Platform: iso9660.EFI, Emulation: iso9660.NoEmulation, BootFile: "/boot/efi.img"},
			},
		},
	}); err != nil {
		return err
	}
	return nil
}

func createDisk(filename string, size int64, opts ...diskfs.OpenOpt) (d *disk.Disk, retErr error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(filename)
		}
	}()
	if err := os.Truncate(filename, size); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("truncating file: %w", err)
	}

	b := file.New(f, false)

	d, err = diskfs.OpenBackend(b, opts...)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return d, nil
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

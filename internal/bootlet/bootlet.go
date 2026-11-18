package bootlet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/partition/gpt"
	diskimg "github.com/ironcore-dev/ironcore-image/v2/image/disk"
	"github.com/ironcore-dev/metal-operator/internal/bootlet/agent"
	"github.com/ironcore-dev/metal-operator/internal/bootlet/block"
	"github.com/ironcore-dev/metal-operator/internal/bootlet/config"
	"github.com/lima-vm/go-qcow2reader/image/qcow2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Bootlet struct {
	imageRepo *remote.Repository
	imageRef  string

	configDriveURL   string
	configDriveToken string

	agentReporter *agent.Client

	explicitDevice    string
	configDriveDevice string
	minGB             int64
	hints             *config.RootDeviceHints
}

func newRepo(imageRef string, cred auth.Credential) (*remote.Repository, error) {
	repo, err := remote.NewRepository(imageRef)
	if err != nil {
		return nil, fmt.Errorf("creating image repository client: %w", err)
	}
	repo.Client = &auth.Client{
		Credential: auth.StaticCredential(repo.Reference.Registry, cred),
	}
	// TODO: As soon as we have certificates on our registry, we must remove this.
	repo.PlainHTTP = true
	return repo, nil
}

func New(cfg *config.Config) (*Bootlet, error) {
	registryCredential := auth.Credential{
		AccessToken: cfg.Token,
	}

	imageRepo, err := newRepo(cfg.ImageRef, registryCredential)
	if err != nil {
		return nil, fmt.Errorf("creating image repository client: %w", err)
	}

	return &Bootlet{
		imageRepo:         imageRepo,
		imageRef:          cfg.ImageRef,
		configDriveURL:    cfg.ConfigDriveURL,
		configDriveToken:  cfg.Token,
		explicitDevice:    cfg.Device,
		configDriveDevice: cfg.SourceDevice,
		minGB:             cfg.MinGB,
		hints:             cfg.Hints,
	}, nil
}

func RootDeviceHintMatcher(h *config.RootDeviceHints) func(block.Device) bool {
	return func(d block.Device) bool {
		contains := func(hay, needle string) bool {
			return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
		}
		switch {
		case h.Name != "" && d.Name != h.Name:
			return false
		case h.Model != "" && !contains(d.Model, h.Model):
			return false
		case h.Serial != "" && !contains(d.Serial, h.Serial):
			return false
		case h.WWN != "" && !contains(d.WWN, h.WWN):
			return false
		case h.Rotational != nil && d.Rotational != *h.Rotational:
			return false
		case h.SizeGB != 0 && !withinTolerance(d.SizeBytes/gb, h.SizeGB):
			return false
		case h.MinSizeGB != 0 && d.SizeBytes < h.MinSizeGB*gb:
			return false
		}
		return true
	}
}

const gb = 1 << 30

func withinTolerance(got, want int64) bool { // Ironic's fuzzy == on size
	return got > want-want/50-1 && got < want+want/50+1
}

func (b *Bootlet) getImage(ctx context.Context) (*diskimg.Image, error) {
	desc, err := b.imageRepo.Resolve(ctx, b.imageRef)
	if err != nil {
		return nil, err
	}

	aImg, err := diskimg.Decode(ctx, b.imageRepo, desc)
	if err != nil {
		return nil, err
	}

	img, ok := aImg.(*diskimg.Image)
	if !ok {
		return nil, fmt.Errorf("expected disk.Image but got %T", img)
	}
	return img, nil
}

func (b *Bootlet) ResolveDevice(ctx context.Context) (string, error) {
	log := logf.FromContext(ctx)

	if b.explicitDevice != "" {
		return b.explicitDevice, nil
	}

	log.Info("Device not specified, trying to select device based on hints / minGB", "hints", b.hints, "minGB", b.minGB)
	var pred func(block.Device) bool
	if !b.hints.IsZero() {
		pred = RootDeviceHintMatcher(b.hints)
	}
	d, err := block.SelectDisk(pred, b.minGB, b.configDriveDevice)
	if err != nil {
		return "", fmt.Errorf("selecting disk: %w", err)
	}

	log.Info("Selected disk", "disk", d.Path)
	return d.Path, nil
}

func (b *Bootlet) ProvisionImage(ctx context.Context, dev string) error {
	log := logf.FromContext(ctx)

	log.Info("Looking up image in registry", "imageRef", b.imageRef)
	img, err := b.getImage(ctx)
	if err != nil {
		return fmt.Errorf("getting image: %w", err)
	}

	log.Info("Writing image")
	if err := b.writeImage(ctx, dev, img); err != nil {
		return fmt.Errorf("writing image: %w", err)
	}

	return nil
}

func (b *Bootlet) openConfigDrive(ctx context.Context) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.configDriveURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating config drive request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+b.configDriveToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching config drive: %w", err)
	}

	contentLengthS := resp.Header.Get("Content-Length")
	contentLength, err := strconv.ParseInt(contentLengthS, 10, 64)
	if err != nil {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("parsing config drive content-length: %w", err)
	}

	return resp.Body, contentLength, nil
}

func (b *Bootlet) ProvisionConfigDrive(ctx context.Context, dev string) error {
	log := logf.FromContext(ctx)

	log.Info("Inserting config drive")
	if err := b.insertConfigDrive(ctx, dev); err != nil {
		return fmt.Errorf("inserting config drive: %w", err)
	}

	return nil
}

type qcow2Chain struct {
	size    int64
	dir     string
	readers []*qcow2.Qcow2
}

func openChain(ctx context.Context, src content.Fetcher, img *diskimg.Image) (res *qcow2Chain, retErr error) {
	dir, err := os.MkdirTemp("", "bootlet-chain-*")
	if err != nil {
		return nil, fmt.Errorf("creating chain temp dir: %w", err)
	}

	c := &qcow2Chain{dir: dir}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, c.Close())
		}
	}()

	for i, chain := range img.Chain {
		rc, err := src.Fetch(ctx, chain)
		if err != nil {
			return nil, fmt.Errorf("fetching layer %d: %w", i, err)
		}

		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("layer-%d", i)))
		if err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("creating layer %d spool file: %w", i, err)
		}

		if _, err := io.Copy(f, rc); err != nil {
			_ = f.Close()
			_ = rc.Close()
			return nil, fmt.Errorf("saving layer %d: %w", i, err)
		}
		_ = rc.Close()

		qr, err := qcow2.Open(f, nil)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("opening layer %d: %w", i, err)
		}

		// Take the virtual size from the qcow2 header. chain.Size is the
		// (compressed) blob size and only a fraction of the virtual size.
		c.size = max(c.size, qr.Size())

		c.readers = append(c.readers, qr)
	}
	return c, nil
}

// readAt resolves a single byte range by walking the chain top-down.
// For each layer, ask Extent: if it has data, read from there; otherwise
// fall through to the next layer. If nothing in the chain has data,
// the range is zero.
func (c *qcow2Chain) readAt(p []byte, off int64) error {
	return c.readLayerFrom(p, off, 0)
}

// readLayerFrom resolves p starting at the given layer, iterating same-status
// extents within a layer until the whole range is covered. Extent only reports
// the run-up to the next status change (often a single cluster), so the range
// remainder must keep querying the same layer; only holes delegate deeper.
func (c *qcow2Chain) readLayerFrom(p []byte, off int64, layerIdx int) error {
	if len(p) == 0 {
		return nil
	}
	if layerIdx >= len(c.readers) {
		// Bottom of the chain: nothing has data here, range is zero.
		clear(p)
		return nil
	}

	ext, err := c.readers[layerIdx].Extent(off, int64(len(p)))
	if err != nil {
		return err
	}

	// Clamp to what this extent covers from `off`.
	n := ext.Start + ext.Length - off
	if n > int64(len(p)) {
		n = int64(len(p))
	}
	if n <= 0 {
		return fmt.Errorf("empty extent at offset %d", off)
	}

	switch {
	case ext.Zero:
		// Explicit zero cluster — authoritative, stop here.
		clear(p[:n])
	case !ext.Allocated:
		// Hole: this layer has nothing here. Try the next layer for this
		// segment, then continue with this layer for the remainder.
		if err := c.readLayerFrom(p[:n], off, layerIdx+1); err != nil {
			return err
		}
	default:
		rn, err := c.readers[layerIdx].ReadAt(p[:n], off)
		if int64(rn) != n {
			return fmt.Errorf("short read at offset %d: read %d of %d bytes: %w", off, rn, n, err)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			// go-qcow2reader returns io.EOF together with a full read when the
			// range ends exactly at the virtual size.
			return err
		}
	}

	return c.readLayerFrom(p[n:], off+n, layerIdx)
}

func (c *qcow2Chain) Close() error {
	var errs []error
	for _, reader := range c.readers {
		errs = append(errs, reader.Close())
	}
	errs = append(errs, os.RemoveAll(c.dir))
	return errors.Join(errs...)
}

func (b *Bootlet) writeImage(ctx context.Context, dev string, img *diskimg.Image) error {
	c, err := openChain(ctx, b.imageRepo, img)
	if err != nil {
		return fmt.Errorf("opening chain: %w", err)
	}
	defer func() { _ = c.Close() }()

	dst, err := os.OpenFile(dev, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = dst.Close() }()

	const chunk = 2 << 20
	buf := make([]byte, chunk)
	for off := int64(0); off < c.size; off += int64(chunk) {
		n := int64(chunk)
		if off+n > c.size {
			n = c.size - off
		}

		if err := c.readAt(buf[:n], off); err != nil {
			return fmt.Errorf("read at offset %d: %w", off, err)
		}

		if _, err := dst.Write(buf[:n]); err != nil {
			return fmt.Errorf("write data (offset %d): %w", off, err)
		}
	}
	if err := dst.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}

const (
	configDriveSectorSize    = 512
	configDriveSize          = 64 << 20
	configDriveSectors       = configDriveSize / configDriveSectorSize
	configDrivePartitionName = "configdrive"
	gptTailSectors           = 34  // backup header (1) + backup entries (32) + slack (1)
	gptMaxPartitionEntries   = 128 // minimum partition array size mandated by the GPT spec
)

func (b *Bootlet) insertConfigDrive(ctx context.Context, devPath string) error {
	log := logf.FromContext(ctx)

	rc, size, err := b.openConfigDrive(ctx)
	if err != nil {
		return fmt.Errorf("opening config drive: %w", err)
	}
	defer func() { _ = rc.Close() }()

	d, err := diskfs.Open(devPath)
	if err != nil {
		return fmt.Errorf("open device %s: %w", devPath, err)
	}
	defer func() { _ = d.Close() }()

	tbl, err := d.GetPartitionTable()
	if err != nil {
		return fmt.Errorf("get partition table: %w", err)
	}

	gptTbl, ok := tbl.(*gpt.Table)
	if !ok {
		return fmt.Errorf("expected gpt.Table as partition table, got %T", tbl)
	}

	lastUsable := uint64(d.Size/configDriveSectorSize) - gptTailSectors

	// Copy the existing partitions (all gpt.Partition fields we care about
	// are exported) to rebuild the table from scratch further down.
	partitions := make([]*gpt.Partition, 0, len(gptTbl.Partitions)+1)
	var cdPart *gpt.Partition
	for _, p := range gptTbl.Partitions {
		cp := *p
		if cp.Name == configDrivePartitionName && cdPart == nil {
			// Retry-safe: a previous (partially failed) run already created it.
			cdPart = &cp
		}
		partitions = append(partitions, &cp)
	}

	if cdPart == nil {
		start := lastUsable - configDriveSectors + 1
		if err := ensureTailFree(gptTbl, start); err != nil {
			return err
		}

		idx, err := firstFreeGPTIndex(gptTbl)
		if err != nil {
			return err
		}

		cdPart = &gpt.Partition{
			Index: idx, // go-diskfs requires a free 1-based slot index when writing
			Start: start,
			End:   lastUsable,
			Size:  configDriveSize,
			Type:  gpt.LinuxFilesystem,
			Name:  configDrivePartitionName,
		}
		partitions = append(partitions, cdPart)
	} else {
		log.Info("Reusing existing config drive partition", "index", cdPart.Index)
	}

	extent := int64((cdPart.End - cdPart.Start + 1) * configDriveSectorSize)
	if size > extent { // your 64 MiB bound, enforced again
		return fmt.Errorf("configdrive %d B exceeds partition extent %d B", size, extent)
	}

	// Rebuild the table from scratch instead of mutating the parsed one: the
	// image's table keeps its geometry (backup GPT location, last usable LBA)
	// pointing at the image's end of disk, not the physical one, and those
	// fields are unexported in go-diskfs. A fresh (uninitialized) table is
	// initialized against the real disk size on write, relocating the backup
	// GPT to the true end of the disk and declaring the whole disk usable.
	freshTbl := &gpt.Table{
		GUID:          gptTbl.GUID, // keep the disk GUID stable
		ProtectiveMBR: gptTbl.ProtectiveMBR,
		Partitions:    partitions,
	}
	if err := d.Partition(freshTbl); err != nil {
		return fmt.Errorf("partitioning: %w", err)
	}

	w, err := d.Backend.Writable()
	if err != nil {
		return fmt.Errorf("getting writable backend: %w", err)
	}
	defer func() { _ = w.Close() }()

	offset := int64(cdPart.Start * configDriveSectorSize)

	// Zero the partition extent first to get rid of stale data on reused disks.
	zero := make([]byte, 2<<20)
	zw := io.NewOffsetWriter(w, offset)
	for remaining := extent; remaining > 0; {
		n := min(int64(len(zero)), remaining)
		if _, err := zw.Write(zero[:n]); err != nil {
			return fmt.Errorf("zeroing config drive partition: %w", err)
		}
		remaining -= n
	}

	if _, err := io.Copy(io.NewOffsetWriter(w, offset), rc); err != nil {
		return fmt.Errorf("writing config drive: %w", err)
	}
	if s, ok := w.(interface{ Sync() error }); ok {
		if err := s.Sync(); err != nil {
			return fmt.Errorf("syncing config drive: %w", err)
		}
	}

	return nil
}

// firstFreeGPTIndex returns the lowest unused 1-based GPT partition slot.
func firstFreeGPTIndex(tbl *gpt.Table) (int, error) {
	used := make(map[int]bool, len(tbl.Partitions))
	for _, p := range tbl.Partitions {
		used[p.Index] = true
	}
	for i := 1; i <= gptMaxPartitionEntries; i++ {
		if !used[i] {
			return i, nil
		}
	}
	return 0, fmt.Errorf("no free GPT partition slot (all %d entries in use)", gptMaxPartitionEntries)
}

// ensureTailFree makes sure the last `configDriveSize` bytes of usable disk
// (ending before the GPT backup header) are not claimed by any partition.
func ensureTailFree(tbl *gpt.Table, start uint64) error {
	for _, p := range tbl.Partitions {
		if p == nil || (p.Start == 0 && p.End == 0) {
			continue // unused GPT entry
		}
		if p.End >= start {
			return fmt.Errorf(
				"partition %q (LBA %d–%d) reaches into config-drive tail starting at LBA %d",
				p.Name, p.Start, p.End, start,
			)
		}
	}
	return nil
}

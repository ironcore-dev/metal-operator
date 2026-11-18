package artifact

import (
	"context"
	"fmt"
	"io"
	"os"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/blob"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/ironcore-dev/metal-operator/internal/image"
	"github.com/ironcore-dev/metal-operator/internal/iso"
	isobuilder "github.com/ironcore-dev/metal-operator/internal/iso"
	"github.com/opencontainers/go-digest"
)

const (
	ISOBuilderName = "ISO"

	contentTypeISO = "application/vnd.efi-iso"
)

type ISOBuilder struct {
	ISOBuilder         *iso.Builder
	RepositoryProvider image.RepositoryProvider
}

func (b *ISOBuilder) Name() string { return ISOBuilderName }

func (b *ISOBuilder) buildAndPushISO(
	ctx context.Context,
	store content.Ingester,
	cfg *resolvedDirectImageConfig,
) (dgst digest.Digest, size int64, err error) {
	return content.IngestBuild(ctx, store, func(w io.Writer) error {
		tmpFile, err := os.CreateTemp("", "iso-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("closing temp file: %w", err)
		}

		initrds := make([]blob.Sized, 0, len(cfg.Image.Initrds))
		for _, initrd := range cfg.Image.Initrds {
			initrds = append(initrds, blob.FromDescriptor(cfg.Repo, initrd))
		}

		if err := b.ISOBuilder.Build(ctx, tmpFile.Name(), isobuilder.BuildArgs{
			Architecture: cfg.Image.Config.Architecture,
			Initrds:      initrds,
			Kernel:       blob.FromDescriptor(cfg.Repo, cfg.Image.Kernel),
			Cmdline:      image.JoinCmdline(cfg.Cmdline, cfg.Args),
			OSRelease:    cfg.OSRelease,
		}); err != nil {
			return fmt.Errorf("building iso: %w", err)
		}

		f, err := os.Open(tmpFile.Name())
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		_, err = io.Copy(w, f)
		return err
	})
}

func (b *ISOBuilder) Build(
	ctx context.Context,
	store content.Ingester,
	cfg BuilderConfig[metalv1alpha1.DirectArtifactConfig],
) (*Bundle, error) {
	resolved, err := resolveDirectImageConfig(ctx, b.RepositoryProvider, cfg.Config)
	if err != nil {
		return nil, err
	}

	isoDgst, isoSize, err := b.buildAndPushISO(ctx, store, resolved)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Entry: "boot.iso",
		Files: map[string]*File{
			"boot.iso": {
				ContentType: contentTypeISO,
				Size:        isoSize,
				Digest:      isoDgst,
			},
		},
	}, nil
}

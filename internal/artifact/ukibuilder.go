package artifact

import (
	"context"
	"fmt"
	"io"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/blob"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/ironcore-dev/metal-operator/internal/image"
	"github.com/ironcore-dev/metal-operator/internal/uki"
	ukibuilder "github.com/ironcore-dev/metal-operator/internal/uki"
	"github.com/opencontainers/go-digest"
)

const UKIBuilderName = "UKI"

type UKIBuilder struct {
	ukiBuilder         *uki.Builder
	repositoryProvider image.RepositoryProvider
}

func (b *UKIBuilder) Name() string { return UKIBuilderName }

func (b *UKIBuilder) buildAndPushUKI(
	ctx context.Context,
	store content.Ingester,
	cfg *resolvedDirectImageConfig,
) (dgst digest.Digest, size int64, err error) {
	return content.IngestBuild(ctx, store, func(w io.Writer) error {
		initrds := make([]blob.Sized, 0, len(cfg.Image.Initrds))
		for _, initrd := range cfg.Image.Initrds {
			initrds = append(initrds, blob.FromDescriptor(cfg.Repo, initrd))
		}

		if err := b.ukiBuilder.Build(ctx, w, ukibuilder.BuildArgs{
			Architecture: cfg.Image.Config.Architecture,
			Initrds:      initrds,
			Kernel:       blob.FromDescriptor(cfg.Repo, cfg.Image.Kernel),
			Cmdline:      image.JoinCmdline(cfg.Cmdline, cfg.Args),
			OSRelease:    cfg.OSRelease,
		}); err != nil {
			return fmt.Errorf("building uki: %w", err)
		}
		return nil
	})
}

func (b *UKIBuilder) Build(
	ctx context.Context,
	store content.Ingester,
	cfg BuilderConfig[metalv1alpha1.DirectArtifactConfig],
) (*Bundle, error) {
	resolved, err := resolveDirectImageConfig(ctx, b.repositoryProvider, cfg.Config)
	if err != nil {
		return nil, err
	}

	ukiDgst, ukiSize, err := b.buildAndPushUKI(ctx, store, resolved)
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Entry: "uki.efi",
		Files: map[string]*File{
			"uki.efi": {
				Digest:      ukiDgst,
				Size:        ukiSize,
				ContentType: "application/efi",
			},
		},
	}, nil
}

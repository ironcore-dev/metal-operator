package artifact

import (
	"cmp"
	"context"
	"fmt"

	"github.com/ironcore-dev/ironcore-image/v2/image/direct"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/image"
	"oras.land/oras-go/v2"
)

type resolvedDirectImageConfig struct {
	Repo      oras.ReadOnlyTarget
	Image     *direct.Image
	Cmdline   string
	Args      string
	OSRelease string
}

func resolveDirectImageConfig(
	ctx context.Context,
	repoProvider image.RepositoryProvider,
	config metalv1alpha1.DirectArtifactConfig,
) (*resolvedDirectImageConfig, error) {
	repo, err := repoProvider.RepositoryFor(config.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("getting repository for %s: %w", config.ImageRef, err)
	}

	desc, err := repo.Resolve(ctx, config.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", config.ImageRef, err)
	}

	img, err := direct.DecodeTyped(ctx, repo, desc)
	if err != nil {
		return nil, err
	}

	cmdline := cmp.Or(img.Config.Cmdline, config.Cmdline)
	args := cmp.Or(img.Config.Args, config.Args)
	osRelease := cmp.Or(img.Config.OSRelease, config.OSRelease)

	return &resolvedDirectImageConfig{
		Repo:      repo,
		Image:     img,
		Cmdline:   cmdline,
		Args:      args,
		OSRelease: osRelease,
	}, nil
}

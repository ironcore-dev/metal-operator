package image

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/ironcore-dev/ironcore-image/v2/image"
	"github.com/ironcore-dev/ironcore-image/v2/image/direct"
	"github.com/ironcore-dev/ironcore-image/v2/image/disk"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry"
)

var allArtifactTypes = []string{
	disk.ArtifactType,
	direct.ArtifactType,
}

func matchArtifactTypes(artifactTypes ...string) func(gotArtifactType string) bool {
	return func(gotArtifactType string) bool {
		return slices.Contains(artifactTypes, gotArtifactType)
	}
}

func compareDescriptors(d1, d2 ocispec.Descriptor) int {
	i1, i2 := slices.Index(allArtifactTypes, d1.ArtifactType), slices.Index(allArtifactTypes, d2.ArtifactType)
	return cmp.Compare(i1, i2)
}

func matchPlatform(architecture string) func(got *ocispec.Platform) bool {
	return func(got *ocispec.Platform) bool {
		if got == nil {
			return false
		}
		return architecture == got.Architecture
	}
}

type ResolveOptions struct {
	ArtifactTypes []string
}

type ResolveOption func(*ResolveOptions)

func Resolve(ctx context.Context, src oras.ReadOnlyTarget, img, architecture string, opts ...ResolveOption) (imageRef string, imageKind metalv1alpha1.ImageKind, err error) {
	rOpts := &ResolveOptions{}
	for _, opt := range opts {
		opt(rOpts)
	}

	if len(rOpts.ArtifactTypes) == 0 {
		rOpts.ArtifactTypes = allArtifactTypes
	}

	ref, err := registry.ParseReference(img)
	if err != nil {
		return "", "", fmt.Errorf("parsing %q: %w", img, err)
	}

	desc, err := image.Resolve(ctx, src, img, image.ResolveOptions{
		MatchArtifactType:  matchArtifactTypes(rOpts.ArtifactTypes...),
		MatchPlatform:      matchPlatform(architecture),
		CompareDescriptors: compareDescriptors,
		OnIterateError:     image.IgnoreImageOrIndexNotFound,
	})
	if err != nil {
		return "", "", fmt.Errorf("resolving %q: %w", img, err)
	}

	imageRef, err = ToRef(img, desc.Digest)
	if err != nil {
		return "", "", err
	}

	return imageRef, imageKind, nil
}

func ToRef(img string, dgst digest.Digest) (string, error) {
	ref, err := registry.ParseReference(img)
	if err != nil {
		return "", fmt.Errorf("parsing %q: %w", img, err)
	}

	ref.Reference = dgst.String()
	return ref.String(), nil
}

func ResolveImageRef(ctx context.Context, src oras.ReadOnlyTarget, imageRef string) (ocispec.Descriptor, error) {
	ref, err := registry.ParseReference(imageRef)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing image ref %q: %w", imageRef, err)
	}

	digest, err := ref.Digest()
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("image ref %q digest error: %w", imageRef, err)
	}

	desc, err := src.Resolve(ctx, digest.String())
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("resolving image %s: %w", digest, err)
	}

	return desc, nil
}

func artifactTypeToImageKind(artifactType string) (metalv1alpha1.ImageKind, error) {
	switch artifactType {
	case direct.ArtifactType:
		return metalv1alpha1.ImageKindDirect, nil
	case disk.ArtifactType:
		return metalv1alpha1.ImageKindDisk, nil
	default:
		return "", fmt.Errorf("unknown artifact type %q", artifactType)
	}
}

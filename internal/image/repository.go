package image

import (
	"slices"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

type RepositoryProvider interface {
	RepositoryFor(reference string) (oras.ReadOnlyTarget, error)
}

type repositoryProvider struct {
	insecureRegistries []string
}

func (r *repositoryProvider) RepositoryFor(reference string) (oras.ReadOnlyTarget, error) {
	ref, err := registry.ParseReference(reference)
	if err != nil {
		return nil, err
	}

	insecure := slices.Contains(r.insecureRegistries, ref.Host())
	repo := &remote.Repository{
		Reference: ref,
		PlainHTTP: insecure,
	}
	return repo, nil
}

type RepositoryProviderOption func(*repositoryProvider)

func WithInsecureRegistries(registries ...string) RepositoryProviderOption {
	return func(r *repositoryProvider) {
		r.insecureRegistries = append(r.insecureRegistries, registries...)
	}
}

func NewRepositoryProvider(opts ...RepositoryProviderOption) RepositoryProvider {
	p := &repositoryProvider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var DefaultRepositoryProvider RepositoryProvider = &repositoryProvider{}

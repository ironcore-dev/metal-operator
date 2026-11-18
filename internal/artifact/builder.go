package artifact

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/opencontainers/go-digest"
)

type Bundle struct {
	Entry string
	Files map[string]*File
}

type File struct {
	Digest      digest.Digest
	Size        int64
	ContentType string
}

type BuilderConfig[config any] struct {
	ArtifactNamespace string
	ArtifactName      string
	Config            config
}

type GenericBuilderConfig = BuilderConfig[any]

type Builder[config any] interface {
	Name() string
	Build(ctx context.Context, store content.Ingester, cfg BuilderConfig[config]) (*Bundle, error)
}

type GenericBuilder interface {
	Name() string
	Build(ctx context.Context, store content.Ingester, cfg GenericBuilderConfig) (*Bundle, error)
}

type genericBuilderAdapter[config any] struct {
	builder Builder[config]
}

func (b *genericBuilderAdapter[config]) Name() string {
	return b.builder.Name()
}

func (b *genericBuilderAdapter[config]) Build(ctx context.Context, store content.Ingester, cfg BuilderConfig[any]) (*Bundle, error) {
	typedCfg, ok := cfg.Config.(config)
	if !ok {
		return nil, fmt.Errorf("expected config to be of type %T, got %T", typedCfg, cfg.Config)
	}

	return b.builder.Build(ctx, store, BuilderConfig[config]{
		ArtifactNamespace: cfg.ArtifactNamespace,
		ArtifactName:      cfg.ArtifactName,
		Config:            typedCfg,
	})
}

type Builders struct {
	byName map[string]GenericBuilder
}

func NewBuilders() *Builders {
	return &Builders{
		byName: make(map[string]GenericBuilder),
	}
}

func (b *Builders) Register(name string, builder GenericBuilder) {
	if _, ok := b.byName[name]; ok {
		panic(fmt.Sprintf("builder %q already registered", name))
	}
	b.byName[name] = builder
}

func (b *Builders) RegisterTyped[config any](name string, builder Builder[config]) {
	b.Register(name, &genericBuilderAdapter[config]{builder})
}

func (b *Builders) Has(name string) bool {
	_, ok := b.byName[name]
	return ok
}

func (b *Builders) Get(name string) (GenericBuilder, bool) {
	bldr, ok := b.byName[name]
	return bldr, ok
}

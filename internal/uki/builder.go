package uki

import (
	"context"
	"fmt"
	"io"
	"maps"

	ironcoreimageblob "github.com/ironcore-dev/ironcore-image/v2/blob"
	"github.com/ironcore-dev/ironcore-image/v2/ukify"
	"github.com/ironcore-dev/metal-operator/internal/blob"
)

type Builder struct {
	stubs map[string]blob.Sized
}

func NewBuilder(stubs map[string]blob.Sized) *Builder {
	return &Builder{
		stubs: maps.Clone(stubs),
	}
}

type BuildArgs struct {
	Architecture string
	Kernel       blob.Sized
	Initrds      []blob.Sized
	Cmdline      string
	OSRelease    string
}

func (b *Builder) Build(ctx context.Context, w io.Writer, args BuildArgs) error {
	if args.Architecture == "" {
		return fmt.Errorf("must specify Architecture")
	}
	if args.Kernel == nil {
		return fmt.Errorf("must specify Kernel")
	}

	stub, ok := b.stubs[args.Architecture]
	if !ok {
		return fmt.Errorf("no stub for architecture %q", args.Architecture)
	}

	initrds := make([]ironcoreimageblob.Sized, 0, len(args.Initrds))
	for _, initrd := range args.Initrds {
		initrds = append(initrds, initrd)
	}

	if err := ukify.Build(ctx, w, ukify.BuildOptions{
		Stub:      stub,
		Kernel:    args.Kernel,
		Initrds:   initrds,
		Cmdline:   args.Cmdline,
		OSRelease: args.OSRelease,
	}); err != nil {
		return fmt.Errorf("ukifying: %w", err)
	}
	return nil
}

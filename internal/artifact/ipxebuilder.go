package artifact

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"io"
	"text/template"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/blob"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/ironcore-dev/metal-operator/internal/image"
	"github.com/ironcore-dev/metal-operator/internal/initrd"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"
)

const (
	IPXEName = "IPXE"
)

//go:embed default-ipxe-script.tpl
var defaultIPXEScriptText string

var defaultIPXEScript *template.Template

func init() {
	defaultIPXEScript = template.Must(template.New("ipxe").Parse(defaultIPXEScriptText))
}

type IPXE struct {
	RepositoryProvider image.RepositoryProvider
	ScriptTemplate     *template.Template
}

type ScriptTemplateValues struct {
	KernelURL string
	InitrdURL string
	Cmdline   string
}

type Config struct {
	KernelDigest  string   `json:"kernelDigest"`
	InitrdDigests []string `json:"initrdDigests"`
	Cmdline       string   `json:"cmdline"`
	OSRelease     string   `json:"osRelease"`
}

func (b *IPXE) buildScript(ctx context.Context, store content.Ingester, cmdline string) (dgst digest.Digest, size int64, err error) {
	return content.IngestBuild(ctx, store, func(w io.Writer) error {
		scriptTemplate := cmp.Or(b.ScriptTemplate, defaultIPXEScript)
		if err := scriptTemplate.Execute(w, &ScriptTemplateValues{
			KernelURL: "./kernel",
			InitrdURL: "./initrd",
			Cmdline:   cmdline,
		}); err != nil {
			return fmt.Errorf("building ipxe script: %w", err)
		}
		return nil
	})
}

func (b *IPXE) buildInitrd(ctx context.Context, store content.Ingester, initrdDescriptors []ocispec.Descriptor) (dgst digest.Digest, size int64, err error) {
	return content.IngestBuild(ctx, store, func(w io.Writer) error {
		initrds := make([]blob.Blob, 0, len(initrdDescriptors))
		for _, i := range initrds {
			initrds = append(initrds, i)
		}
		return initrd.Merge(ctx, w, initrds)
	})
}

func (b *IPXE) copyKernel(ctx context.Context, store content.Ingester, repo oras.ReadOnlyTarget, kernelDesc ocispec.Descriptor) error {
	return content.IngestKnown(ctx, store, kernelDesc.Digest, kernelDesc.Size, func() (io.ReadCloser, error) {
		return repo.Fetch(ctx, kernelDesc)
	})
}

func (b *IPXE) Name() string { return IPXEName }

func (b *IPXE) Build(
	ctx context.Context,
	store content.Store,
	cfg BuilderConfig[metalv1alpha1.DirectArtifactConfig],
) (*Bundle, error) {
	resolved, err := resolveDirectImageConfig(ctx, b.RepositoryProvider, cfg.Config)
	if err != nil {
		return nil, err
	}

	var (
		grp, grpCtx = errgroup.WithContext(ctx)

		scriptDgst digest.Digest
		scriptSize int64

		initrdDgst digest.Digest
		initrdSize int64
	)
	grp.Go(func() error {
		var err error
		scriptDgst, scriptSize, err = b.buildScript(grpCtx, store, image.JoinCmdline(resolved.Cmdline, resolved.Args))
		return err
	})
	grp.Go(func() error {
		var err error
		initrdDgst, initrdSize, err = b.buildInitrd(grpCtx, store, resolved.Image.Initrds)
		return err
	})
	grp.Go(func() error {
		return b.copyKernel(ctx, store, resolved.Repo, resolved.Image.Kernel)
	})
	if err := grp.Wait(); err != nil {
		return nil, err
	}

	return &Bundle{
		Entry: "ipxe.script",
		Files: map[string]*File{
			"ipxe.script": {
				ContentType: "text/plain",
				Digest:      scriptDgst,
				Size:        scriptSize,
			},
			"kernel": {
				ContentType: "application/octet-stream",
				Digest:      resolved.Image.Kernel.Digest,
				Size:        resolved.Image.Kernel.Size,
			},
			"initrd": {
				ContentType: "application/octet-stream",
				Digest:      initrdDgst,
				Size:        initrdSize,
			},
		},
	}, nil
}

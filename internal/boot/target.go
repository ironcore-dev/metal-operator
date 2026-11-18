package boot

import (
	"context"
	"fmt"
	"net/url"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
)

type Target interface {
	target()
}

type ServerClaimMetadata struct {
	// Namespace is the namespace of the server claim.
	Namespace string
	// Name is the name of the server claim.
	Name string
	// UID is the UID of the server claim.
	UID types.UID
}

type TargetConfig struct {
	Metadata  ServerClaimMetadata
	ImageRef  string
	Cmdline   string
	Args      string
	OSRelease string
	// EmbedConfigDrive may be specified if the config drive should be embedded into the target.
	EmbedConfigDrive *ConfigDriveConfig
}

type TargetProvider interface {
	ProvisionTarget(ctx context.Context, server *metalv1alpha1.Server, config TargetConfig) (Target, task.Result, error)
	CleanupTarget(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error)
}

type ISOImage struct {
	URI *url.URL
}

func (*ISOImage) target() {}

type LocalDiskTarget struct {
}

func (*LocalDiskTarget) target() {}

type EFIImage struct {
	URI *url.URL
}

func (*EFIImage) target() {}

type PXE struct{}

func (*PXE) target() {}

type TargetDeleter interface {
	// DeleteTarget deletes any boot target from the node, setting it to a neutral and safe state.
	DeleteTarget(ctx context.Context, server *metalv1alpha1.Server) (task.Result, error)
}

type TargetControl interface {
	TargetDeleter
	// ApplyTarget applies the given target. If the target gets updated internally, mutate must be called if non-nil.
	// errdef.ErrUnsupported can be returned to indicate an unsupported target.
	ApplyTarget(ctx context.Context, server *metalv1alpha1.Server, target Target, mutate func() error) (task.Result, error)
}

type ImperativeTargetControl interface {
	TargetDeleter
	// HasTarget reports whether the targeted node has given target configured.
	HasTarget(ctx context.Context, server *metalv1alpha1.Server, target Target) (bool, error)
	SetTarget(ctx context.Context, server *metalv1alpha1.Server, target Target) (task.Result, error)
}

type imperativeTargetControlAdapter struct {
	ImperativeTargetControl
}

func (a *imperativeTargetControlAdapter) ApplyTarget(
	ctx context.Context,
	server *metalv1alpha1.Server,
	target Target,
	mutate func() error,
) (task.Result, error) {
	hasTarget, err := a.HasTarget(ctx, server, target)
	if err != nil {
		return task.Result{}, fmt.Errorf("checking has target: %w", err)
	}
	if hasTarget {
		return task.Result{}, nil
	}
	if mutate != nil {
		if err := mutate(); err != nil {
			return task.Result{}, err
		}
	}
	return a.SetTarget(ctx, server, target)
}

func AsTargetControl(imperative ImperativeTargetControl) TargetControl {
	return &imperativeTargetControlAdapter{imperative}
}

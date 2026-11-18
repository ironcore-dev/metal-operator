package boot

import (
	"context"
	"fmt"
	"net/url"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
)

type ConfigDrive interface {
	configDrive()
}

type ConfigDriveConfig struct {
	Metadata ServerClaimMetadata
	Hostname string
	UserData []byte
	Network  Network
}

type ConfigDriveProvider interface {
	ProvisionConfigDrive(ctx context.Context, server *metalv1alpha1.Server, cfg ConfigDriveConfig) (ConfigDrive, task.Result, error)
	CleanupConfigDrive(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error)
}

type ConfigDriveImage struct {
	URI *url.URL
}

func (*ConfigDriveImage) configDrive() {}

type ConfigDriveSupporter interface {
	// SupportsConfigDrive reports whether a server supports a config drive.
	SupportsConfigDrive(ctx context.Context, server *metalv1alpha1.Server) (bool, error)
}

type ConfigDriveDeleter interface {
	// DeleteConfigDrive deletes the config drive from the target server, if any.
	DeleteConfigDrive(ctx context.Context, server *metalv1alpha1.Server) (task.Result, error)
}

type ConfigDriveControl interface {
	ConfigDriveSupporter
	ConfigDriveDeleter
	// ApplyConfigDrive applies the config drive on the target server to the given one.
	// Mutate must be called if non-nil and the config drive is updated internally.
	ApplyConfigDrive(ctx context.Context, server *metalv1alpha1.Server, configDrive ConfigDrive, mutate func() error) (task.Result, error)
}

type ImperativeConfigDriveControl interface {
	ConfigDriveSupporter
	ConfigDriveDeleter
	// HasConfigDrive checks whether the target node has the specified config drive.
	HasConfigDrive(ctx context.Context, server *metalv1alpha1.Server, configDrive ConfigDrive) (bool, error)
	// SetConfigDrive sets the config drive on the target node to the given one.
	SetConfigDrive(ctx context.Context, server *metalv1alpha1.Server, configDrive ConfigDrive) (task.Result, error)
}

type imperativeConfigDriveControlAdapter struct {
	ImperativeConfigDriveControl
}

func (a *imperativeConfigDriveControlAdapter) ApplyConfigDrive(ctx context.Context, server *metalv1alpha1.Server, configDrive ConfigDrive, mutate func() error) (task.Result, error) {
	ok, err := a.HasConfigDrive(ctx, server, configDrive)
	if err != nil {
		return task.Result{}, fmt.Errorf("checking config drive: %w", err)
	}
	if !ok {
		if mutate != nil {
			if err := mutate(); err != nil {
				return task.Result{}, err
			}
		}
	}
	return a.SetConfigDrive(ctx, server, configDrive)
}

func AsConfigDriveControl(imperativeControl ImperativeConfigDriveControl) ConfigDriveControl {
	return &imperativeConfigDriveControlAdapter{
		imperativeControl,
	}
}

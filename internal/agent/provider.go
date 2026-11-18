package agent

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
)

type Config struct {
	Metadata    boot.ServerClaimMetadata
	ImageRef    string
	ConfigDrive *ConfigDriveConfig
}

type ConfigDriveConfig struct {
	Hostname string
	UserData []byte
	Network  boot.Network
}

type Spec struct {
	Handle   any
	ImageRef string
	Hostname string
	UserData []byte
}

type Provider interface {
	Provision(ctx context.Context, server *metalv1alpha1.Server, spec Config) (*Spec, task.Result, error)
	CheckCompleted(ctx context.Context, handle any) (task.Result, error)
	Cleanup(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error)
}

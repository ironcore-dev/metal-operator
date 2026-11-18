package boot

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
)

type NetworkConfig struct {
	Metadata ServerClaimMetadata
}

type NetworkProvider interface {
	ProvisionNetwork(ctx context.Context, server *metalv1alpha1.Server, config NetworkConfig) (Network, task.Result, error)
	CleanupNetwork(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error)
}

type Network interface {
	APIVersion() string
	Kind() string
}

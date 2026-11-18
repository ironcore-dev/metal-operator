package boot

import (
	"fmt"
	"maps"
	"slices"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TargetProviderResolver interface {
	TargetProviderFor(server *metalv1alpha1.Server) (TargetProvider, error)
}

type targetProviderResolver struct {
	defaultBootMode          metalv1alpha1.BootMode
	targetProviderByBootMode map[metalv1alpha1.BootMode]TargetProvider
}

func NewTargetProviderResolver(
	defaultBootMode metalv1alpha1.BootMode,
	targetProviderByBootMode map[metalv1alpha1.BootMode]TargetProvider,
) TargetProviderResolver {
	return &targetProviderResolver{
		defaultBootMode:          defaultBootMode,
		targetProviderByBootMode: maps.Clone(targetProviderByBootMode),
	}
}

func (b *targetProviderResolver) TargetProviderFor(server *metalv1alpha1.Server) (TargetProvider, error) {
	if bootMode := server.Spec.BootMode; bootMode != nil {
		if bt, ok := b.targetProviderByBootMode[*bootMode]; ok {
			return bt, nil
		}
		return nil, reconcile.TerminalError(fmt.Errorf("no target provider for server boot mode %q", *bootMode))
	}

	bootMode := b.defaultBootMode
	bt, ok := b.targetProviderByBootMode[bootMode]
	if !ok {
		return nil, reconcile.TerminalError(fmt.Errorf("no target provider for default boot mode %q", b.defaultBootMode))
	}

	if slices.Contains(server.Spec.SupportedBootModes, bootMode) {
		return bt, nil
	}
	return nil, reconcile.TerminalError(fmt.Errorf("server does not support boot mode %q (supported %v)",
		bootMode, server.Spec.SupportedBootModes))
}

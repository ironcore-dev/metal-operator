package deploy

import (
	"context"
	"errors"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
	"oras.land/oras-go/v2/errdef"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type DirectDeployer struct {
	networkProvider boot.NetworkProvider

	targetProviderResolver boot.TargetProviderResolver
	targetControl          boot.TargetControl
	powerControl           boot.PowerControl

	configDriveProvider boot.ConfigDriveProvider
	configDriveControl  boot.ConfigDriveControl

	canEmbedConfigDrive bool
}

type DirectDeployerOptions struct {
	NetworkProvider boot.NetworkProvider

	TargetProviderResolver boot.TargetProviderResolver
	TargetControl          boot.TargetControl
	PowerControl           boot.PowerControl

	ConfigDriveProvider boot.ConfigDriveProvider
	ConfigDriveControl  boot.ConfigDriveControl

	CanEmbedConfigDrive bool
}

func NewDirectDeployer(opts DirectDeployerOptions) *DirectDeployer {
	return &DirectDeployer{
		networkProvider:        opts.NetworkProvider,
		targetProviderResolver: opts.TargetProviderResolver,
		targetControl:          opts.TargetControl,
		powerControl:           opts.PowerControl,
		configDriveProvider:    opts.ConfigDriveProvider,
		configDriveControl:     opts.ConfigDriveControl,
		canEmbedConfigDrive:    opts.CanEmbedConfigDrive,
	}
}

func (b *DirectDeployer) WithCanEmbedConfigDrive() *DirectDeployer {
	res := *b
	res.canEmbedConfigDrive = true
	return &res
}

type DirectConfig struct {
	Metadata  boot.ServerClaimMetadata
	ImageRef  string
	UserData  []byte
	Hostname  string
	Cmdline   string
	Args      string
	OSRelease string
}

func (b *DirectDeployer) Deploy(
	ctx context.Context,
	server *metalv1alpha1.Server,
	markers Markers,
	cfg DirectConfig,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	var network boot.Network
	if b.networkProvider != nil {
		log.V(5).Info("Provisioning network")
		networkConfig := boot.NetworkConfig{Metadata: cfg.Metadata}
		provisioned, res, err := b.networkProvider.ProvisionNetwork(ctx, server, networkConfig)
		if err != nil {
			return task.Result{}, fmt.Errorf("provisioning network: %w", err)
		}
		if res.Pending {
			return res, nil
		}

		network = provisioned
	}

	log.V(5).Info("Configuring boot")
	configDriveConfig := boot.ConfigDriveConfig{
		Metadata: cfg.Metadata,
		Hostname: cfg.Hostname,
		UserData: cfg.UserData,
		Network:  network,
	}
	configured, res, err := b.configure(ctx, server, configDriveConfig)
	if err != nil {
		return res, fmt.Errorf("configuring boot: %w", err)
	}
	if res.Pending {
		return res, nil
	}

	log.V(5).Info("Provisioning boot target")
	targetConfig := boot.TargetConfig{
		Metadata:  cfg.Metadata,
		ImageRef:  cfg.ImageRef,
		Cmdline:   cfg.Cmdline,
		Args:      cfg.Args,
		OSRelease: cfg.OSRelease,
	}
	if !configured && b.canEmbedConfigDrive {
		targetConfig.EmbedConfigDrive = &configDriveConfig
	}
	res, err = b.setBootTarget(ctx, server, markers, targetConfig)
	if err != nil {
		return task.Result{}, fmt.Errorf("setting boot target: %w", err)
	}
	if res.Pending {
		return res, nil
	}

	didReboot := markers.MarkerExists(metalv1alpha1.MarkerBooted)
	if !didReboot {
		// We don't mind about the task result of reboot here as simply issuing reboot is enough.
		log.V(1).Info("Issuing reboot")
		if err := b.powerControl.Reboot(ctx, server); err != nil {
			return task.Result{}, fmt.Errorf("rebooting server: %w", err)
		}

		if err := markers.UpdateMarkers(ctx, AddMarker(metalv1alpha1.MarkerBooted)); err != nil {
			return task.Result{}, fmt.Errorf("setting server rebooted: %w", err)
		}
	}
	return task.Result{}, nil
}

func (b *DirectDeployer) configure(
	ctx context.Context,
	node *metalv1alpha1.Server,
	config boot.ConfigDriveConfig,
) (cfgDriveSupported bool, res task.Result, err error) {
	log := logf.FromContext(ctx)

	configDriveSupported, err := b.configDriveControl.SupportsConfigDrive(ctx, node)
	if err != nil {
		return false, task.Result{}, fmt.Errorf("checking config drive support: %w", err)
	}
	if !configDriveSupported {
		log.V(5).Info("Config drive not supported")
		return false, task.Result{}, nil
	}
	res, err = b.setConfigDrive(ctx, node, config)
	if err != nil {
		return false, task.Result{}, fmt.Errorf("setting config drive: %w", err)
	}
	return true, res, nil
}

func (b *DirectDeployer) setConfigDrive(
	ctx context.Context,
	server *metalv1alpha1.Server,
	configDriveConfig boot.ConfigDriveConfig,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	cfgDrive, res, err := b.configDriveProvider.ProvisionConfigDrive(ctx, server, configDriveConfig)
	if err != nil {
		return res, fmt.Errorf("provisioning config drive: %w", err)
	}

	return b.configDriveControl.ApplyConfigDrive(ctx, server, cfgDrive, func() error {
		log.V(1).Info("Updating config drive")
		return nil
	})
}

func (b *DirectDeployer) setBootTarget(
	ctx context.Context,
	server *metalv1alpha1.Server,
	markers Markers,
	bootCfg boot.TargetConfig,
) (task.Result, error) {
	targetProvider, err := b.targetProviderResolver.TargetProviderFor(server)
	if err != nil {
		return task.Result{}, fmt.Errorf("resolving target provider for server %s: %w", server.Name, err)
	}

	target, res, err := targetProvider.ProvisionTarget(ctx, server, bootCfg)
	if err != nil {
		return task.Result{}, fmt.Errorf("provisioning kernel boot target: %w", err)
	}
	if res.Pending {
		return res, nil
	}

	return b.targetControl.ApplyTarget(ctx, server, target, func() error {
		return markers.UpdateMarkers(ctx, DeleteMarker(metalv1alpha1.MarkerBooted))
	})
}

func (b *DirectDeployer) cleanupConfigDrive(
	ctx context.Context,
	node *metalv1alpha1.Server,
	namespacedName types.NamespacedName,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	configDriveSupported, err := b.configDriveControl.SupportsConfigDrive(ctx, node)
	if err != nil {
		return task.Result{}, fmt.Errorf("checking config drive support: %w", err)
	}
	if configDriveSupported {
		log.V(5).Info("Deleting config drive")
		res, err := b.configDriveControl.DeleteConfigDrive(ctx, node)
		if err != nil {
			return task.Result{}, fmt.Errorf("deleting config drive: %w", err)
		}
		if res.Pending {
			return res, nil
		}

		log.V(5).Info("Cleaning up config drive")
		res, err = b.configDriveProvider.CleanupConfigDrive(ctx, node, namespacedName)
		if err != nil {
			return task.Result{}, fmt.Errorf("cleaning up config drive: %w", err)
		}
		if res.Pending {
			return res, nil
		}
	} else {
		log.V(5).Info("Config drive was not supported, nothing to clean up")
	}
	return task.Result{}, nil
}

func (b *DirectDeployer) Cleanup(ctx context.Context, server *metalv1alpha1.Server, namespacedName types.NamespacedName) (task.Result, error) {
	log := logf.FromContext(ctx)

	log.V(5).Info("Cleaning up boot config")
	res, err := b.cleanupConfigDrive(ctx, server, namespacedName)
	if err != nil {
		return task.Result{}, fmt.Errorf("cleaning up boot config: %w", err)
	}
	if res.Pending {
		return res, nil
	}

	log.V(5).Info("Cleaning up boot target")
	targetProvider, err := b.targetProviderResolver.TargetProviderFor(server)
	if err != nil && !errors.Is(err, errdef.ErrUnsupported) {
		return task.Result{}, fmt.Errorf("resolving target provider for server %s: %w", server.Name, err)
	}
	if err == nil {
		res, err := targetProvider.CleanupTarget(ctx, server, namespacedName)
		if err != nil {
			return task.Result{}, fmt.Errorf("cleaning up target: %w", err)
		}
		if res.Pending {
			return res, nil
		}
	}

	res, err = b.targetControl.DeleteTarget(ctx, server)
	if err != nil {
		return task.Result{}, fmt.Errorf("deleting target: %w", err)
	}
	if res.Pending {
		return res, nil
	}

	log.V(5).Info("Cleanup successful")
	return task.Result{}, nil
}

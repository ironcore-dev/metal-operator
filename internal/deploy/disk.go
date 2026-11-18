package deploy

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/agent"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type DiskConfig struct {
	Metadata boot.ServerClaimMetadata
	ImageRef string
	Hostname string
	UserData []byte
}

type DiskDeployer struct {
	networkProvider boot.NetworkProvider

	targetControl boot.TargetControl
	powerControl  boot.PowerControl

	agentDeployer *DirectDeployer
	agentProvider agent.Provider
}

type DiskDeployerOptions struct {
	NetworkProvider boot.NetworkProvider

	TargetControl boot.TargetControl
	PowerControl  boot.PowerControl

	AgentDeployer *DirectDeployer
	AgentProvider agent.Provider
}

func NewDiskDeployer(opts DiskDeployerOptions) *DiskDeployer {
	return &DiskDeployer{
		networkProvider: opts.NetworkProvider,
		targetControl:   opts.TargetControl,
		powerControl:    opts.PowerControl,
		agentDeployer:   opts.AgentDeployer,
		agentProvider:   opts.AgentProvider,
	}
}

func (d *DiskDeployer) provision(
	ctx context.Context,
	server *metalv1alpha1.Server,
	markers Markers,
	cfg DiskConfig,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	var network boot.Network
	if d.networkProvider != nil {
		log.V(5).Info("Provisioning network")
		networkConfig := boot.NetworkConfig{Metadata: cfg.Metadata}
		provisioned, res, err := d.networkProvider.ProvisionNetwork(ctx, server, networkConfig)
		if err != nil {
			return task.Result{}, fmt.Errorf("provisioning network: %w", err)
		}
		if !res.Done() {
			return res, nil
		}

		network = provisioned
	}

	log.V(5).Info("Provisioning agent")
	agentSpec, res, err := d.agentProvider.Provision(ctx, server, agent.Config{
		ImageRef: cfg.ImageRef,
		ConfigDrive: &agent.ConfigDriveConfig{
			Hostname: cfg.Hostname,
			UserData: cfg.UserData,
			Network:  network,
		},
	})
	if err != nil {
		return task.Result{}, fmt.Errorf("provisioning agent: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	log.V(5).Info("Deploying agent")
	res, err = d.agentDeployer.Deploy(ctx, server, markers, DirectConfig{
		ImageRef: agentSpec.ImageRef,
		Hostname: agentSpec.Hostname,
		UserData: agentSpec.UserData,
	})
	if err != nil {
		return res, fmt.Errorf("deploying agent: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	res, err = d.powerControl.ApplyPower(ctx, server, true, func() error {
		log.V(1).Info("Issued server power-on")
		return nil
	})
	if err != nil {
		return task.Result{}, fmt.Errorf("powering on server: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	log.V(5).Info("Checking agent completion")
	return d.agentProvider.CheckCompleted(ctx, agentSpec.Handle)
}

func (d *DiskDeployer) cleanupAgent(
	ctx context.Context,
	server *metalv1alpha1.Server,
	namespacedName types.NamespacedName,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	log.V(1).Info("Cleaning up agent deployment")
	res, err := d.agentDeployer.Cleanup(ctx, server, namespacedName)
	if err != nil {
		return task.Result{}, fmt.Errorf("cleaning up agent deployment: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	log.V(1).Info("Cleaning up agent provider")
	res, err = d.agentProvider.Cleanup(ctx, server, namespacedName)
	if err != nil {
		return task.Result{}, fmt.Errorf("cleaning up agent provider: %w", err)
	}
	return res, nil
}

func (d *DiskDeployer) Deploy(
	ctx context.Context,
	server *metalv1alpha1.Server,
	markers Markers,
	cfg DiskConfig,
) (task.Result, error) {
	log := logf.FromContext(ctx)

	provisioned := markers.MarkerExists(metalv1alpha1.MarkerProvisioned)
	if !provisioned {
		res, err := d.provision(ctx, server, markers, cfg)
		if err != nil {
			return task.Result{}, fmt.Errorf("provisioning: %w", err)
		}
		if !res.Done() {
			return res, nil
		}

		log.V(1).Info("Marking reboot required & server provisioned")
		if err := markers.UpdateMarkers(ctx,
			DeleteMarker(metalv1alpha1.MarkerBooted),
			AddMarker(metalv1alpha1.MarkerProvisioned),
		); err != nil {
			return task.Result{}, fmt.Errorf("updating markers: %w", err)
		}
	}
	res, err := d.cleanupAgent(ctx, server, types.NamespacedName{
		Namespace: cfg.Metadata.Namespace,
		Name:      cfg.Metadata.Name,
	})
	if err != nil {
		return task.Result{}, fmt.Errorf("cleaning up agent: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	res, err = d.targetControl.ApplyTarget(ctx, server, &boot.LocalDiskTarget{}, func() error {
		log.V(1).Info("Updated server target")
		return nil
	})
	if err != nil {
		return task.Result{}, fmt.Errorf("applying target: %w", err)
	}
	if !res.Done() {
		return res, nil
	}

	needsReboot := markers.MarkerExists(metalv1alpha1.MarkerDiskRebooted)
	if needsReboot {
		log.V(1).Info("Issuing reboot")
		if err := d.powerControl.Reboot(ctx, server); err != nil {
			return task.Result{}, fmt.Errorf("rebooting server: %w", err)
		}

		if err := markers.UpdateMarkers(ctx, AddMarker(metalv1alpha1.MarkerDiskRebooted)); err != nil {
			return task.Result{}, fmt.Errorf("updating markers: %w", err)
		}
	}
	return task.Result{}, nil
}

func (d *DiskDeployer) Cleanup(
	ctx context.Context,
	server *metalv1alpha1.Server,
	namespacedName types.NamespacedName,
) (task.Result, error) {
	res, err := d.cleanupAgent(ctx, server, namespacedName)
	if err != nil {
		return task.Result{}, fmt.Errorf("cleaning up agent: %w", err)
	}
	if !res.Done() {
		return res, nil
	}
	if d.networkProvider != nil {
		res, err = d.networkProvider.CleanupNetwork(ctx, server, namespacedName)
		if err != nil {
			return task.Result{}, fmt.Errorf("cleaning up network: %w", err)
		}
		if !res.Done() {
			return res, nil
		}
	}
	return task.Result{}, nil
}

package redfish

import (
	"context"
	"fmt"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"github.com/stmcginnis/gofish/schemas"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func isInPowerState(p schemas.PowerState, powerState bool) bool {
	if powerState {
		return p == schemas.OnPowerState
	}
	return p == schemas.OffPowerState
}

func isTransitioningToPowerState(p schemas.PowerState, powerState bool) bool {
	if powerState {
		return p == schemas.PoweringOnPowerState
	}
	return p == schemas.PoweringOffPowerState
}

func powerStateResetType(powerState bool) schemas.ResetType {
	if powerState {
		return schemas.OnResetType
	}
	return schemas.GracefulShutdownResetType
}

type PowerControl struct {
	client       k8sclient.Client
	dialer       Dialer
	pollDuration time.Duration
}

func NewPowerControl(client k8sclient.Client, dialer Dialer) *PowerControl {
	return &PowerControl{
		client:       client,
		dialer:       dialer,
		pollDuration: 5 * time.Second,
	}
}

func (c *PowerControl) ApplyPower(
	ctx context.Context,
	node *metalv1alpha1.Server,
	powerState bool,
	mutate func() error,
) (task.Result, error) {
	addr, opts, err := DialDataFromServer(ctx, c.client, node)
	if err != nil {
		return task.Result{}, err
	}

	bmcClient, err := c.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return task.Result{}, err
	}
	defer func() { _ = bmcClient.Close() }()

	system, err := SystemForNode(ctx, bmcClient, node)
	if err != nil {
		return task.Result{}, err
	}

	if isInPowerState(system.PowerState, powerState) {
		return task.Result{}, nil
	}
	if isTransitioningToPowerState(system.PowerState, powerState) {
		return task.Result{RetryAfter: c.pollDuration}, nil
	}

	if mutate != nil {
		if err := mutate(); err != nil {
			return task.Result{}, err
		}
	}

	mon, err := system.Reset(powerStateResetType(powerState))
	if err != nil {
		return task.Result{}, fmt.Errorf("resetting system: %w", err)
	}
	return MonitorToTaskResult(mon, c.pollDuration), nil
}

func (c *PowerControl) State(ctx context.Context, node *metalv1alpha1.Server) (boot.PowerState, error) {
	addr, opts, err := DialDataFromServer(ctx, c.client, node)
	if err != nil {
		return "", err
	}

	bmcClient, err := c.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return "", err
	}
	defer func() { _ = bmcClient.Close() }()

	system, err := SystemForNode(ctx, bmcClient, node)
	if err != nil {
		return "", err
	}

	switch system.PowerState {
	case schemas.OnPowerState:
		return boot.PowerStateOn, nil
	case schemas.PoweringOnPowerState:
		return boot.PowerStatePoweringOn, nil
	case schemas.OffPowerState:
		return boot.PowerStateOff, nil
	case schemas.PoweringOffPowerState:
		return boot.PowerStatePoweringOff, nil
	default:
		return boot.PowerStateUnknown, nil
	}
}

func (c *PowerControl) Reboot(ctx context.Context, node *metalv1alpha1.Server) error {
	addr, opts, err := DialDataFromServer(ctx, c.client, node)
	if err != nil {
		return err
	}

	bmcClient, err := c.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return err
	}
	defer func() { _ = bmcClient.Close() }()

	system, err := SystemForNode(ctx, bmcClient, node)
	if err != nil {
		return err
	}

	_, err = system.Reset(schemas.ForceRestartResetType)
	if err != nil {
		return fmt.Errorf("resetting system: %w", err)
	}
	return nil
}

package redfish

import (
	"context"
	"fmt"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/boot"
	"github.com/ironcore-dev/metal-operator/internal/ptrs"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"github.com/stmcginnis/gofish/schemas"
	"oras.land/oras-go/v2/errdef"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigDriveControl struct {
	client       k8sclient.Client
	dialer       Dialer
	pollDuration time.Duration
}

func NewConfigDriveControl(client k8sclient.Client, dialer Dialer) *ConfigDriveControl {
	return &ConfigDriveControl{
		client:       client,
		dialer:       dialer,
		pollDuration: 5 * time.Second,
	}
}

var _ boot.ImperativeConfigDriveControl = (*ConfigDriveControl)(nil)

var configDriveMediaTypes = []schemas.VirtualMediaType{
	schemas.USBStickVirtualMediaType,
	schemas.FloppyVirtualMediaType,
}

func (c *ConfigDriveControl) SupportsConfigDrive(ctx context.Context, node *metalv1alpha1.Server) (bool, error) {
	addr, opts, err := DialDataFromServer(ctx, c.client, node)
	if err != nil {
		return false, err
	}

	bmcClient, err := c.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return false, err
	}
	defer func() { _ = bmcClient.Close() }()

	system, err := SystemForNode(ctx, bmcClient, node)
	if err != nil {
		return false, err
	}

	vm, err := FindVirtualMediaByType(ctx, system, configDriveMediaTypes...)
	if err != nil {
		return false, fmt.Errorf("finding config drive virtual media: %w", err)
	}
	return vm != nil, nil
}

func (c *ConfigDriveControl) HasConfigDrive(ctx context.Context, node *metalv1alpha1.Server, configDrive boot.ConfigDrive) (bool, error) {
	virtualMediaRemovable, ok := configDrive.(*boot.ConfigDriveImage)
	if !ok {
		return false, fmt.Errorf("config drive %T: %w", configDrive, errdef.ErrUnsupported)
	}

	addr, opts, err := DialDataFromServer(ctx, c.client, node)
	if err != nil {
		return false, err
	}

	bmcClient, err := c.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return false, err
	}
	defer func() { _ = bmcClient.Close() }()

	system, err := SystemForNode(ctx, bmcClient, node)
	if err != nil {
		return false, err
	}

	vm, err := FindVirtualMediaByType(ctx, system, configDriveMediaTypes...)
	if err != nil {
		return false, fmt.Errorf("finding config drive virtual media: %w", err)
	}
	if vm == nil {
		return false, fmt.Errorf("no config drive virtual media found")
	}

	if !ptrs.DerefOrZero(vm.Inserted) || ptrs.DerefOrZero(vm.WriteProtected) {
		return false, nil
	}

	return vm.Image == virtualMediaRemovable.URI.String(), nil
}

func (c *ConfigDriveControl) SetConfigDrive(
	ctx context.Context,
	node *metalv1alpha1.Server,
	configDrive boot.ConfigDrive,
) (task.Result, error) {
	virtualMedia, ok := configDrive.(*boot.ConfigDriveImage)
	if !ok {
		return task.Result{}, fmt.Errorf("config drive %T: %w", configDrive, errdef.ErrUnsupported)
	}

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

	vm, err := FindVirtualMediaByType(ctx, system, configDriveMediaTypes...)
	if err != nil {
		return task.Result{}, fmt.Errorf("finding config drive virtual media: %w", err)
	}

	return SetVirtualMedia(ctx, vm, virtualMedia.URI.String(), c.pollDuration)
}

func (c *ConfigDriveControl) DeleteConfigDrive(
	ctx context.Context,
	node *metalv1alpha1.Server,
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

	vm, err := FindVirtualMediaByType(ctx, system, configDriveMediaTypes...)
	if err != nil {
		return task.Result{}, fmt.Errorf("finding config drive virtual media: %w", err)
	}

	return EjectVirtualMedia(ctx, vm, c.pollDuration)
}

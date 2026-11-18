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

type BootControl struct {
	client       k8sclient.Client
	pollDuration time.Duration
	dialer       Dialer
}

func NewBootControl(client k8sclient.Client, dialer Dialer) *BootControl {
	return &BootControl{
		client: client,
		dialer: dialer,
	}
}

var _ boot.ImperativeTargetControl = (*BootControl)(nil)

func (r *BootControl) HasTarget(ctx context.Context, node *metalv1alpha1.Server, target boot.Target) (bool, error) {
	addr, opts, err := DialDataFromServer(ctx, r.client, node)
	if err != nil {
		return false, err
	}

	c, err := r.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return false, err
	}
	defer func() { _ = c.Close() }()

	system, err := SystemForNode(ctx, c, node)
	if err != nil {
		return false, err
	}

	return r.hasTarget(ctx, system, target)
}

func (r *BootControl) hasTarget(ctx context.Context, system *schemas.ComputerSystem, target boot.Target) (bool, error) {
	switch target := target.(type) {
	case *boot.ISOImage:
		return r.hasVirtualMediaCDTarget(ctx, system, target)
	case *boot.EFIImage:
		return r.hasHTTPTarget(ctx, system, target)
	case *boot.PXE:
		return r.hasPXETarget(ctx, system, target)
	case *boot.LocalDiskTarget:
		return r.hasLocalDiskTarget(ctx, system, target)
	default:
		return false, fmt.Errorf("target type %T: %w", target, errdef.ErrUnsupported)
	}
}

func isContinuousCDBoot(boot *schemas.Boot) bool {
	return boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
		boot.BootSourceOverrideTarget == schemas.CdBootSource
}

func (r *BootControl) hasVirtualMediaCDTarget(ctx context.Context, system *schemas.ComputerSystem, target *boot.ISOImage) (bool, error) {
	if !isContinuousCDBoot(&system.Boot) {
		return false, nil
	}

	vm, err := FindVirtualMediaByType(ctx, system, schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType)
	if err != nil {
		return false, fmt.Errorf("finding virtual media: %w", err)
	}
	if vm == nil {
		return false, fmt.Errorf("no cd / dvd virtual media found")
	}

	image := target.URI.String()
	return vm.Image == image && ptrs.DerefOrZero(vm.Inserted) && !ptrs.DerefOrZero(vm.WriteProtected), nil
}

func isContinuousHTTPBoot(boot *schemas.Boot, httpBootURI string) bool {
	return boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
		boot.BootSourceOverrideTarget == schemas.UefiHTTPBootSource &&
		boot.HTTPBootURI != nil && *boot.HTTPBootURI == httpBootURI
}

func (r *BootControl) hasHTTPTarget(ctx context.Context, system *schemas.ComputerSystem, target *boot.EFIImage) (bool, error) {
	httpBootURI := target.URI.String()
	return isContinuousHTTPBoot(&system.Boot, httpBootURI), nil
}

func isContinuousPXEBoot(boot *schemas.Boot) bool {
	return boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
		boot.BootSourceOverrideTarget == schemas.PxeBootSource
}

func (r *BootControl) hasPXETarget(ctx context.Context, system *schemas.ComputerSystem, target *boot.PXE) (bool, error) {
	return isContinuousPXEBoot(&system.Boot), nil
}

func isContinuousUEFIShellBoot(boot *schemas.Boot) bool {
	return boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
		boot.BootSourceOverrideTarget == schemas.UefiShellBootSource
}

func isContinuousHDDBoot(boot *schemas.Boot) bool {
	return boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
		boot.BootSourceOverrideTarget == schemas.HddBootSource
}

func (r *BootControl) hasLocalDiskTarget(_ context.Context, system *schemas.ComputerSystem, _ *boot.LocalDiskTarget) (bool, error) {
	return isContinuousHDDBoot(&system.Boot), nil
}

func (r *BootControl) setLocalDiskTarget(_ context.Context, system *schemas.ComputerSystem, _ *boot.LocalDiskTarget) (task.Result, error) {
	if isContinuousHDDBoot(&system.Boot) {
		return task.Result{}, nil
	}
	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.HddBootSource,
	}); err != nil {
		return task.Result{}, fmt.Errorf("setting boot: %w", err)
	}
	return task.Result{}, nil
}

func (r *BootControl) setISOImageTarget(
	ctx context.Context,
	system *schemas.ComputerSystem,
	target *boot.ISOImage,
) (task.Result, error) {
	vm, err := FindVirtualMediaByType(ctx, system, schemas.CDVirtualMediaType, schemas.DVDVirtualMediaType)
	if err != nil {
		return task.Result{}, fmt.Errorf("finding virtual media: %w", err)
	}
	if vm == nil {
		return task.Result{}, fmt.Errorf("no cd / dvd virtual media found")
	}

	res, err := SetVirtualMedia(ctx, vm, target.URI.String(), r.pollDuration)
	if err != nil {
		return task.Result{}, fmt.Errorf("setting virtual media: %w", err)
	}
	if res.Pending {
		return res, nil
	}
	if isContinuousCDBoot(&system.Boot) {
		return task.Result{}, nil
	}

	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.CdBootSource,
	}); err != nil {
		return task.Result{}, fmt.Errorf("setting boot: %w", err)
	}
	return task.Result{}, nil
}

func (r *BootControl) setEFIImageTarget(ctx context.Context, system *schemas.ComputerSystem, target *boot.EFIImage) (task.Result, error) {
	httpBootURI := target.URI.String()

	if isContinuousHTTPBoot(&system.Boot, httpBootURI) {
		return task.Result{}, nil
	}

	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.UefiHTTPBootSource,
		HTTPBootURI:               &httpBootURI,
	}); err != nil {
		return task.Result{}, fmt.Errorf("setting boot: %w", err)
	}
	return task.Result{}, nil
}

func (r *BootControl) setPXETarget(ctx context.Context, system *schemas.ComputerSystem, target *boot.PXE) (task.Result, error) {
	if isContinuousPXEBoot(&system.Boot) {
		return task.Result{}, nil
	}
	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.PxeBootSource,
	}); err != nil {
		return task.Result{}, fmt.Errorf("setting boot: %w", err)
	}
	return task.Result{}, nil
}

func (r *BootControl) setBootUEFIShell(_ context.Context, system *schemas.ComputerSystem) (task.Result, error) {
	if isContinuousUEFIShellBoot(&system.Boot) {
		return task.Result{}, nil
	}
	if err := system.SetBoot(&schemas.Boot{
		BootSourceOverrideEnabled: schemas.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.UefiShellBootSource, // TODO: Also handle non-UEFI here
	}); err != nil {
		return task.Result{}, fmt.Errorf("setting boot: %w", err)
	}
	return task.Result{}, nil
}

func (r *BootControl) setTarget(
	ctx context.Context,
	system *schemas.ComputerSystem,
	target boot.Target,
) (task.Result, error) {
	switch target := target.(type) {
	case *boot.ISOImage:
		return r.setISOImageTarget(ctx, system, target)
	case *boot.EFIImage:
		return r.setEFIImageTarget(ctx, system, target)
	case *boot.PXE:
		return r.setPXETarget(ctx, system, target)
	case *boot.LocalDiskTarget:
		return r.setLocalDiskTarget(ctx, system, target)
	default:
		return task.Result{}, fmt.Errorf("%w target type: %T", errdef.ErrUnsupported, target)
	}
}

func (r *BootControl) SetTarget(
	ctx context.Context,
	node *metalv1alpha1.Server,
	target boot.Target,
) (task.Result, error) {
	addr, opts, err := DialDataFromServer(ctx, r.client, node)
	if err != nil {
		return task.Result{}, err
	}

	c, err := r.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return task.Result{}, err
	}
	defer func() { _ = c.Close() }()

	system, err := SystemForNode(ctx, c, node)
	if err != nil {
		return task.Result{}, err
	}

	res, err := r.setTarget(ctx, system, target)
	if err != nil {
		return task.Result{}, err
	}
	return res, nil
}

func (r *BootControl) DeleteTarget(ctx context.Context, node *metalv1alpha1.Server) (task.Result, error) {
	addr, opts, err := DialDataFromServer(ctx, r.client, node)
	if err != nil {
		return task.Result{}, err
	}

	c, err := r.dialer.Dial(ctx, addr, opts)
	if err != nil {
		return task.Result{}, err
	}
	defer func() { _ = c.Close() }()

	system, err := SystemForNode(ctx, c, node)
	if err != nil {
		return task.Result{}, err
	}

	return r.setBootUEFIShell(ctx, system)
}

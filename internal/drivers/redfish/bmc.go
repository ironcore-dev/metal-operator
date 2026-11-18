package redfish

import (
	"context"
	"fmt"
	"net"
	"slices"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/ptrs"
	"github.com/ironcore-dev/metal-operator/internal/task"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
	"oras.land/oras-go/v2/errdef"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BmcSecretUsernameKey = "username"
	BmcSecretPasswordKey = "password"
)

func decodeBMCSecretCredentials(secret *metalv1alpha1.BMCSecret) (username, password string, err error) {
	// TODO: use constants for secret keys
	usernameData, ok := secret.Data[BmcSecretUsernameKey]
	if !ok {
		return "", "", fmt.Errorf("bmc secret %s has no username at %s", secret.Name, BmcSecretUsernameKey)
	}

	passwordData, ok := secret.Data[BmcSecretPasswordKey]
	if !ok {
		return "", "", fmt.Errorf("bmc secret %s has no password at %s", secret.Name, BmcSecretPasswordKey)
	}

	return string(usernameData), string(passwordData), nil
}

func getBMCCredentials(ctx context.Context, c k8sclient.Client, bmcSecretName string) (username, password string, err error) {
	secret := &metalv1alpha1.BMCSecret{}
	secretKey := k8sclient.ObjectKey{Name: bmcSecretName}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		return "", "", fmt.Errorf("getting bmc secret: %w", err)
	}
	return decodeBMCSecretCredentials(secret)
}

func resolveDialData(ctx context.Context, c k8sclient.Client, host string, protocol metalv1alpha1.Protocol, bmcSecretName string) (addr string, opts DialOptions, err error) {
	username, password, err := getBMCCredentials(ctx, c, bmcSecretName)
	if err != nil {
		return "", DialOptions{}, fmt.Errorf("getting bmc credentials: %w", err)
	}

	address := fmt.Sprintf("%s://%s", protocol.Scheme, net.JoinHostPort(host, fmt.Sprintf("%d", protocol.Port)))
	return address, DialOptions{
		Username: username,
		Password: password,
	}, nil
}

func DialDataFromServer(ctx context.Context, c k8sclient.Client, server *metalv1alpha1.Server) (addr string, opts DialOptions, err error) {
	switch {
	case server.Spec.BMC != nil:
		bmc := server.Spec.BMC
		if protocolName := bmc.Protocol.Name; protocolName != metalv1alpha1.ProtocolNameRedfish {
			return "", DialOptions{}, fmt.Errorf("protocol %q is not supported", protocolName)
		}

		return resolveDialData(ctx, c, bmc.Address, bmc.Protocol, bmc.BMCSecretRef.Name)
	case server.Spec.BMCRef != nil:
		bmc := &metalv1alpha1.BMC{}
		bmcKey := k8sclient.ObjectKey{Name: server.Spec.BMCRef.Name}
		if err := c.Get(ctx, bmcKey, bmc); err != nil {
			return "", DialOptions{}, fmt.Errorf("getting bmc %s: %w", server.Spec.BMCRef.Name, err)
		}

		var host string
		switch {
		case bmc.Spec.Endpoint != nil:
			host = bmc.Spec.Endpoint.IP.String()
		case bmc.Spec.EndpointRef != nil:
			endpoint := &metalv1alpha1.Endpoint{}
			endpointKey := k8sclient.ObjectKey{Name: bmc.Spec.EndpointRef.Name}
			if err := c.Get(ctx, endpointKey, endpoint); err != nil {
				return "", DialOptions{}, fmt.Errorf("getting bmc endpoint %s: %w", bmc.Spec.EndpointRef.Name, err)
			}
			host = endpoint.Spec.IP.String()
		default:
			return "", DialOptions{}, fmt.Errorf("bmc %s has no endpoint", server.Spec.BMCRef.Name)
		}

		return resolveDialData(ctx, c, host, bmc.Spec.Protocol, bmc.Spec.BMCSecretRef.Name)
	default:
		return "", DialOptions{}, fmt.Errorf("not a bmc node: %w", errdef.ErrUnsupported)
	}
}

func SystemForNode(ctx context.Context, c Client, node *metalv1alpha1.Server) (*schemas.ComputerSystem, error) {
	systems, err := c.Systems(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting systems: %w", err)
	}

	for _, s := range systems {
		if s.ID == node.Spec.SystemUUID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("system %q not found", node.Spec.SystemUUID)
}

func SetEntityClientContext(ctx context.Context, obj schemas.SchemaObject) {
	c := obj.GetClient()
	if apiClient, ok := c.(*gofish.APIClient); ok {
		obj.SetClient(apiClient.WithContext(ctx))
	}
}

func FindVirtualMediaByType(
	ctx context.Context,
	system *schemas.ComputerSystem,
	acceptMediaTypes ...schemas.VirtualMediaType,
) (*schemas.VirtualMedia, error) {
	SetEntityClientContext(ctx, system)

	vms, err := system.VirtualMedia()
	if err != nil {
		return nil, fmt.Errorf("getting virtual media list: %w", err)
	}
	for _, v := range vms {
		for _, mt := range v.MediaTypes {
			if slices.Contains(acceptMediaTypes, mt) {
				return v, nil
			}
		}
	}
	return nil, nil
}

func MonitorToTaskResult(mon *schemas.TaskMonitorInfo, defaultPollDuration time.Duration) task.Result {
	switch {
	// According to redfish spec, a result w/o monitor indicates the action has happened successfully.
	case mon == nil:
		return task.Result{}
	// If the monitor specifies a mark to retry after, use that.
	case !mon.RetryAfter.IsZero():
		return task.Result{RetryAfter: time.Until(mon.RetryAfter)}
	// Otherwise, stick to polling with a given default duration.
	default:
		return task.Result{RetryAfter: defaultPollDuration}
	}
}

func SetVirtualMedia(ctx context.Context, vm *schemas.VirtualMedia, image string, pollDuration time.Duration) (task.Result, error) {
	SetEntityClientContext(ctx, vm)
	if vm.Image == image && ptrs.DerefOrZero(vm.Inserted) && !ptrs.DerefOrZero(vm.WriteProtected) {
		return task.Result{}, nil
	}

	if ptrs.DerefOrZero(vm.Inserted) {
		mon, err := vm.EjectMedia()
		if err != nil {
			return task.Result{}, fmt.Errorf("ejecting media: %w", err)
		}
		if mon != nil {
			return MonitorToTaskResult(mon, pollDuration), nil
		}
	}

	mon, err := vm.InsertMedia(&schemas.VirtualMediaInsertMediaParameters{
		Image:          image,
		Inserted:       new(true),
		WriteProtected: new(false),
	})
	if err != nil {
		return task.Result{}, fmt.Errorf("inserting media: %w", err)
	}
	return MonitorToTaskResult(mon, pollDuration), nil
}

func EjectVirtualMedia(ctx context.Context, vm *schemas.VirtualMedia, pollDuration time.Duration) (task.Result, error) {
	if !ptrs.DerefOrZero(vm.Inserted) {
		return task.Result{}, nil
	}

	mon, err := vm.EjectMedia()
	if err != nil {
		return task.Result{}, fmt.Errorf("ejecting media: %w", err)
	}
	return MonitorToTaskResult(mon, pollDuration), nil
}

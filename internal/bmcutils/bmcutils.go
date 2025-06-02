// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmcutils

import (
	"context"
	"fmt"
	"net"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetProtocolScheme(scheme metalv1alpha1.ProtocolScheme, insecure bool) metalv1alpha1.ProtocolScheme {
	if scheme != "" {
		return scheme
	}
	if insecure {
		return metalv1alpha1.HTTPProtocolScheme
	}
	return metalv1alpha1.HTTPSProtocolScheme
}

func GetBMCCredentialsFromSecret(secret *metalv1alpha1.BMCSecret) (string, string, error) {
	// TODO: use constants for secret keys
	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("no username found in the BMC secret")
	}
	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("no password found in the BMC secret")
	}
	return string(username), string(password), nil
}

func GetBMCFromBMCName(ctx context.Context, c client.Client, bmcName string) (*metalv1alpha1.BMC, error) {
	bmcObj := &metalv1alpha1.BMC{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcName}, bmcObj); err != nil {
		return nil, fmt.Errorf("failed to get bmc %q: %w", bmcName, err)
	}
	return bmcObj, nil
}

func GetBMCCredentialsForBMCSecretName(ctx context.Context, c client.Client, bmcSecretName string) (string, string, error) {
	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcSecretName}, bmcSecret); err != nil {
		return "", "", fmt.Errorf("failed to get bmc secret: %w", err)
	}
	return GetBMCCredentialsFromSecret(bmcSecret)
}

func GetBMCAddressForBMC(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC) (string, error) {
	var address string

	if bmcObj.Spec.EndpointRef != nil {
		endpoint := &metalv1alpha1.Endpoint{}
		if err := c.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
			return "", fmt.Errorf("failed to get Endpoints for BMC: %w", err)
		}
		return endpoint.Spec.IP.String(), nil
	}

	if bmcObj.Spec.Endpoint != nil {
		return bmcObj.Spec.Endpoint.IP.String(), nil
	}

	return address, nil
}

const DefaultKubeNamespace = "default"

func GetBMCClientForServer(ctx context.Context, c client.Client, server *metalv1alpha1.Server, insecure bool, options bmc.Options) (bmc.BMC, error) {
	if server.Spec.BMCRef != nil {
		b := &metalv1alpha1.BMC{}
		bmcName := server.Spec.BMCRef.Name
		if err := c.Get(ctx, client.ObjectKey{Name: bmcName}, b); err != nil {
			return nil, fmt.Errorf("failed to get BMC: %w", err)
		}

		return GetBMCClientFromBMC(ctx, c, b, insecure, options)
	}

	if server.Spec.BMC != nil {
		bmcSecret := &metalv1alpha1.BMCSecret{}
		if err := c.Get(ctx, client.ObjectKey{Name: server.Spec.BMC.BMCSecretRef.Name}, bmcSecret); err != nil {
			return nil, fmt.Errorf("failed to get BMC secret: %w", err)
		}

		protocolScheme := GetProtocolScheme(server.Spec.BMC.Protocol.Scheme, insecure)

		return CreateBMCClient(
			ctx,
			c,
			protocolScheme,
			server.Spec.BMC.Protocol.Name,
			server.Spec.BMC.Address,
			server.Spec.BMC.Protocol.Port,
			bmcSecret,
			options,
		)
	}

	return nil, fmt.Errorf("server %s has neither a BMCRef nor a BMC configured", server.Name)
}

func GetBMCClientFromBMC(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC, insecure bool, options bmc.Options) (bmc.BMC, error) {
	var address string

	if bmcObj.Spec.EndpointRef != nil {
		endpoint := &metalv1alpha1.Endpoint{}
		if err := c.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
			return nil, fmt.Errorf("failed to get Endpoints for BMC: %w", err)
		}
		address = endpoint.Spec.IP.String()
	}

	if bmcObj.Spec.Endpoint != nil {
		address = bmcObj.Spec.Endpoint.IP.String()
	}

	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.BMCSecretRef.Name}, bmcSecret); err != nil {
		return nil, fmt.Errorf("failed to get BMC secret: %w", err)
	}

	protocolScheme := GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, insecure)

	return CreateBMCClient(ctx, c, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, bmcSecret, options)
}

func CreateBMCClient(
	ctx context.Context,
	c client.Client,
	protocolScheme metalv1alpha1.ProtocolScheme,
	bmcProtocol metalv1alpha1.ProtocolName,
	address string,
	port int32,
	bmcSecret *metalv1alpha1.BMCSecret,
	bmcOptions bmc.Options,
) (bmc.BMC, error) {
	var bmcClient bmc.BMC
	var err error

	bmcOptions.Endpoint = fmt.Sprintf("%s://%s", protocolScheme, net.JoinHostPort(address, fmt.Sprintf("%d", port)))
	bmcOptions.Username, bmcOptions.Password, err = GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials from BMC secret: %w", err)
	}

	switch bmcProtocol {
	case metalv1alpha1.ProtocolRedfish:
		bmcClient, err = bmc.NewRedfishBMCClient(ctx, bmcOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create Redfish client: %w", err)
		}
	case metalv1alpha1.ProtocolRedfishLocal:
		bmcClient, err = bmc.NewRedfishLocalBMCClient(ctx, bmcOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to create Redfish client: %w", err)
		}
	case metalv1alpha1.ProtocolRedfishKube:
		bmcClient, err = bmc.NewRedfishKubeBMCClient(ctx, bmcOptions, c, DefaultKubeNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to create Redfish client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported BMC protocol %s", bmcProtocol)
	}
	return bmcClient, nil
}

func GetServerNameFromBMCandIndex(index int, bmc *metalv1alpha1.BMC) string {
	return fmt.Sprintf("%s-%s-%d", bmc.Name, "system", index)
}

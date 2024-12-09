// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package console

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
	BMCAddress string
	Username   string
	Password   string
}

func GetConfigForServerName(ctx context.Context, c client.Client, serverName string) (*Config, error) {
	server := &v1alpha1.Server{}
	if err := c.Get(ctx, client.ObjectKey{Name: serverName}, server); err != nil {
		return nil, fmt.Errorf("failed to get server %q: %w", serverName, err)
	}

	// Inline BMC configuration
	if server.Spec.BMC != nil {
		username, password, err := bmcutils.GetBMCCredentialsForBMCSecretName(ctx, c, server.Spec.BMC.BMCSecretRef.Name)
		if err != nil {
			return nil, err
		}
		return &Config{
			BMCAddress: server.Spec.BMC.Address,
			Username:   username,
			Password:   password,
		}, nil
	}

	// BMC by reference
	if server.Spec.BMCRef != nil {
		bmc, err := bmcutils.GetBMCFromBMCName(ctx, c, server.Spec.BMCRef.Name)
		if err != nil {
			return nil, err
		}
		username, password, err := bmcutils.GetBMCCredentialsForBMCSecretName(ctx, c, bmc.Spec.BMCSecretRef.Name)
		if err != nil {
			return nil, err
		}
		address, err := bmcutils.GetBMCAddressForBMC(ctx, c, bmc)
		if err != nil {
			return nil, err
		}

		return &Config{
			BMCAddress: address,
			Username:   username,
			Password:   password,
		}, nil
	}

	return nil, nil
}

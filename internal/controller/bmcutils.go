/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/base64"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/afritzler/metal-operator/bmc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetBMCClientFromBMCName(ctx context.Context, c client.Client, bmcName string, insecure bool) (bmc.BMC, error) {
	bmc := &metalv1alpha1.BMC{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcName}, bmc); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("BMC %q not found", bmcName)
		}
		return nil, err
	}
	return GetBMCClientFromBMC(ctx, c, bmc, insecure)
}

func GetBMCClientFromBMC(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC, insecure bool) (bmc.BMC, error) {
	endpoint := &metalv1alpha1.Endpoint{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.EndpointRef.Name}, endpoint); err != nil {
		return nil, fmt.Errorf("failed to get Endpoints for BMC: %w", err)
	}

	bmcSecret := &metalv1alpha1.BMCSecret{}
	if err := c.Get(ctx, client.ObjectKey{Name: bmcObj.Spec.BMCSecretRef.Name}, bmcSecret); err != nil {
		return nil, fmt.Errorf("failed to get BMC secret: %w", err)
	}

	protocol := "https"
	if insecure {
		protocol = "http"
	}

	var bmcClient bmc.BMC
	switch bmcObj.Spec.Protocol.Name {
	case ProtocolRedfish:
		bmcAddress := fmt.Sprintf("%s://%s:%d", protocol, endpoint.Spec.IP, bmcObj.Spec.Protocol.Port)
		username, password, err := GetBMCCredentialsFromSecret(bmcSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials from BMC secret: %w", err)
		}
		bmcClient, err = bmc.NewRedfishBMCClient(ctx, bmcAddress, username, password, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create Redfish client: %w", err)
		}
	case ProtocolRedfishLocal:
		bmcAddress := fmt.Sprintf("%s://%s:%d", protocol, endpoint.Spec.IP, bmcObj.Spec.Protocol.Port)
		username, password, err := GetBMCCredentialsFromSecret(bmcSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials from BMC secret: %w", err)
		}
		bmcClient, err = bmc.NewRedfishLocalBMCClient(ctx, bmcAddress, username, password, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create Redfish client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported BMC protocol %s", bmcObj.Spec.Protocol.Name)
	}
	return bmcClient, nil
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
	decodedUsername, err := base64.StdEncoding.DecodeString(string(username))
	if err != nil {
		return "", "", fmt.Errorf("error decoding username: %w", err)
	}
	decodedPassword, err := base64.StdEncoding.DecodeString(string(password))
	if err != nil {
		return "", "", fmt.Errorf("error decoding password: %w", err)
	}

	return string(decodedUsername), string(decodedPassword), nil
}

func GetServerNameFromBMCandIndex(index int, bmc *metalv1alpha1.BMC) string {
	return fmt.Sprintf("compute-%d-%s", index, bmc.Name)
}

func GetBMCNameFromEndpoint(endpoint *metalv1alpha1.Endpoint) string {
	return fmt.Sprintf("bmc-%s", endpoint.Name)
}

func GetBMCSecretNameFromEndpoint(endpoint *metalv1alpha1.Endpoint) string {
	return fmt.Sprintf("bmc-%s", endpoint.Name)
}

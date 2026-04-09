// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmcutils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"golang.org/x/crypto/ssh"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BmcSecretUsernameKey = "username"
	BmcSecretPasswordKey = "password"
)

type BMCUnAvailableError struct {
	Message string
}

func (e BMCUnAvailableError) Error() string {
	return e.Message
}

type BMCClientOptions int

const (
	BMCConnectivityCheckOption BMCClientOptions = 1
)

// CreateBMCClientOption is a functional option for CreateBMCClient and GetBMCClientForServer.
type CreateBMCClientOption func(*createBMCClientConfig)

type createBMCClientConfig struct {
	registryURL string
}

// WithRegistryURL configures the BMC client to POST dummy registration data to
// the given base URL after SetPXEBootOnce. Used with ProtocolRedfishWithRegistryPatch to
// simulate probe boot registration without a real K8s Job.
func WithRegistryURL(url string) CreateBMCClientOption {
	return func(c *createBMCClientConfig) {
		c.registryURL = url
	}
}

func GetProtocolScheme(scheme metalv1alpha1.ProtocolScheme, defaultScheme metalv1alpha1.ProtocolScheme) metalv1alpha1.ProtocolScheme {
	if scheme != "" {
		return scheme
	}
	return defaultScheme
}

func GetBMCCredentialsFromSecret(secret *metalv1alpha1.BMCSecret) (string, string, error) {
	// TODO: use constants for secret keys
	username, err := getValueFromSecret(secret, BmcSecretUsernameKey)
	if err != nil {
		return "", "", err
	}
	password, err := getValueFromSecret(secret, BmcSecretPasswordKey)
	if err != nil {
		return "", "", err
	}
	return username, password, nil
}

func getValueFromSecret(secret *metalv1alpha1.BMCSecret, key string) (string, error) {
	if secret == nil {
		return "", errors.New("secret cannot be nil")
	}
	value, ok := secret.Data[key]
	if ok {
		return string(value), nil
	}
	valueStr, ok := secret.StringData[key]
	if ok {
		return valueStr, nil
	}
	return "", fmt.Errorf("cannot find value in BMCSecret '%s' for key '%s' in data nor in stringData", secret.Name, key)
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

func GetBMCClientForServer(ctx context.Context, c client.Client, server *metalv1alpha1.Server, defaultProtocol metalv1alpha1.ProtocolScheme, skipCertValidation bool, options bmc.Options, opts ...CreateBMCClientOption) (bmc.BMC, error) {
	if server.Spec.BMCRef != nil {
		b := &metalv1alpha1.BMC{}
		bmcName := server.Spec.BMCRef.Name
		if err := c.Get(ctx, client.ObjectKey{Name: bmcName}, b); err != nil {
			return nil, err
		}

		anyOpts := make([]any, len(opts))
		for i, o := range opts {
			anyOpts[i] = o
		}
		return GetBMCClientFromBMC(ctx, c, b, defaultProtocol, skipCertValidation, options, anyOpts...)
	}

	if server.Spec.BMC != nil {
		bmcSecret := &metalv1alpha1.BMCSecret{}
		if err := c.Get(ctx, client.ObjectKey{Name: server.Spec.BMC.BMCSecretRef.Name}, bmcSecret); err != nil {
			return nil, err
		}

		protocolScheme := GetProtocolScheme(server.Spec.BMC.Protocol.Scheme, defaultProtocol)

		return CreateBMCClient(
			ctx,
			c,
			protocolScheme,
			server.Spec.BMC.Protocol.Name,
			server.Spec.BMC.Address,
			server.Spec.BMC.Protocol.Port,
			bmcSecret,
			options,
			skipCertValidation,
			opts...,
		)
	}

	return nil, fmt.Errorf("server %s has neither a BMCRef nor a BMC configured", server.Name)
}

func GetBMCClientFromBMC(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC, defaultProtocol metalv1alpha1.ProtocolScheme, skipCertValidation bool, options bmc.Options, opts ...any) (bmc.BMC, error) {
	var address string
	var bmcClientOpts []BMCClientOptions
	var createOpts []CreateBMCClientOption
	for _, o := range opts {
		switch v := o.(type) {
		case BMCClientOptions:
			bmcClientOpts = append(bmcClientOpts, v)
		case CreateBMCClientOption:
			createOpts = append(createOpts, v)
		}
	}

	if !slices.Contains(bmcClientOpts, BMCConnectivityCheckOption) {
		if bmcObj.Status.State != metalv1alpha1.BMCStateEnabled && bmcObj.Status.State != "" {
			return nil, BMCUnAvailableError{Message: fmt.Sprintf("BMC %s is not in enabled state: current state: %s", bmcObj.Name, bmcObj.Status.State)}
		}
	}

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

	protocolScheme := GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, defaultProtocol)
	bmcClient, err := CreateBMCClient(ctx, c, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, bmcSecret, options, skipCertValidation, createOpts...)
	return bmcClient, err
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
	skipCertValidation bool,
	opts ...CreateBMCClientOption,
) (bmc.BMC, error) {
	var bmcClient bmc.BMC
	var err error

	cfg := &createBMCClientConfig{}
	for _, o := range opts {
		o(cfg)
	}

	bmcOptions.Endpoint = fmt.Sprintf("%s://%s", protocolScheme, net.JoinHostPort(address, fmt.Sprintf("%d", port)))
	bmcOptions.Username, bmcOptions.Password, err = GetBMCCredentialsFromSecret(bmcSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials from BMC secret: %w", err)
	}
	bmcOptions.InsecureTLS = skipCertValidation

	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Creating BMC client", "Protocol", bmcProtocol, "Address", bmcOptions.Endpoint, "Username", bmcOptions.Username, "cfg", cfg)

	switch bmcProtocol {
	case metalv1alpha1.ProtocolRedfish:
		bmcClient, err = bmc.NewRedfishBMCClient(ctx, bmcOptions)
		if err != nil {
			return nil, err
		}
	case metalv1alpha1.ProtocolRedfishLocal:
		bmcClient, err = bmc.NewRedfishLocalBMCClient(ctx, bmcOptions)
		if err != nil {
			return nil, err
		}
	case metalv1alpha1.ProtocolRedfishWithRegistryPatch:
		bmcClient, err = bmc.NewRedfishLocalBMCClientWithRegistry(ctx, bmcOptions, cfg.registryURL)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported BMC protocol %s", bmcProtocol)
	}
	return bmcClient, nil
}

func GetServerNameFromBMCandIndex(index int, bmcObj *metalv1alpha1.BMC) string {
	return fmt.Sprintf("%s-%s-%d", bmcObj.Name, "system", index)
}

func SSHResetBMC(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
	// If Redfish reset fails, try SSH-based reset for known manufacturers
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		Timeout: timeout,
	}
	resetCMD := ""
	switch manufacturer {
	case string(bmc.ManufacturerDell):
		resetCMD = "racreset"
	case string(bmc.ManufacturerHPE):
		resetCMD = "cd /map1 && reset"
	case string(bmc.ManufacturerLenovo):
		resetCMD = "resetsp"
	default:
		return fmt.Errorf("unsupported BMC manufacturer %s for bmc reset", manufacturer)
	}
	sshClient, err := ssh.Dial("tcp", net.JoinHostPort(ip, "22"), config)
	if err != nil {
		return fmt.Errorf("failed to dial ssh: %w", err)
	}
	defer func() {
		_ = sshClient.Close()
	}()

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()
	// cancel reset cmd after 5 minutes
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- session.Run(resetCMD)
	}()
	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for BMC reset command to complete: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to run reset command: %w", err)
		}
	}
	return nil
}

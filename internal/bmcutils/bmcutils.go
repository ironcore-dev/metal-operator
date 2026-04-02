// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmcutils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/bmc"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/api/core/v1"
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

func GetBMCClientForServer(ctx context.Context, c client.Client, server *metalv1alpha1.Server, insecure bool, options bmc.Options) (bmc.BMC, error) {
	if server.Spec.BMCRef != nil {
		b := &metalv1alpha1.BMC{}
		bmcName := server.Spec.BMCRef.Name
		if err := c.Get(ctx, client.ObjectKey{Name: bmcName}, b); err != nil {
			return nil, err
		}

		return GetBMCClientFromBMC(ctx, c, b, insecure, options)
	}

	if server.Spec.BMC != nil {
		bmcSecret := &metalv1alpha1.BMCSecret{}
		if err := c.Get(ctx, client.ObjectKey{Name: server.Spec.BMC.BMCSecretRef.Name}, bmcSecret); err != nil {
			return nil, err
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

func GetBMCClientFromBMC(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC, insecure bool, options bmc.Options, opts ...BMCClientOptions) (bmc.BMC, error) {
	var address string

	if !slices.Contains(opts, BMCConnectivityCheckOption) {
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

	// Load certificate for secure connection if available
	if !insecure && bmcObj.Status.CertificateSecretRef != nil {
		tlsConfig, err := loadTLSConfigFromSecret(ctx, c, bmcObj)
		if err != nil {
			// Log warning but continue with insecure connection
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Info("Failed to load certificate, using insecure connection", "error", err)
		} else {
			options.TLSConfig = tlsConfig
		}
	}

	protocolScheme := GetProtocolScheme(bmcObj.Spec.Protocol.Scheme, insecure)
	bmcClient, err := CreateBMCClient(ctx, c, protocolScheme, bmcObj.Spec.Protocol.Name, address, bmcObj.Spec.Protocol.Port, bmcSecret, options)
	return bmcClient, err
}

// loadTLSConfigFromSecret loads TLS configuration from a certificate secret
func loadTLSConfigFromSecret(ctx context.Context, c client.Client, bmcObj *metalv1alpha1.BMC) (*tls.Config, error) {
	secret := &v1.Secret{}
	secretKey := client.ObjectKey{
		Name:      bmcObj.Status.CertificateSecretRef.Name,
		Namespace: getManagerNamespace(), // Use manager namespace for cluster-scoped BMC
	}

	if err := c.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get certificate secret: %w", err)
	}

	// Load CA certificate
	caCertPEM := secret.Data["ca.crt"]
	if len(caCertPEM) == 0 {
		return nil, fmt.Errorf("ca.crt not found in certificate secret")
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	// Add client certificate if present (for operator-generated CSR case)
	if certPEM, hasCert := secret.Data["tls.crt"]; hasCert {
		if keyPEM, hasKey := secret.Data["tls.key"]; hasKey {
			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	return tlsConfig, nil
}

// getManagerNamespace returns the namespace where the metal-operator is running
func getManagerNamespace() string {
	// Try to read from environment variable first (set by Kubernetes)
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	// Default namespace
	return "metal-operator-system"
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
			return nil, err
		}
	case metalv1alpha1.ProtocolRedfishLocal:
		bmcClient, err = bmc.NewRedfishLocalBMCClient(ctx, bmcOptions)
		if err != nil {
			return nil, err
		}
	case metalv1alpha1.ProtocolRedfishKube:
		bmcClient, err = bmc.NewRedfishKubeBMCClient(ctx, bmcOptions, c, DefaultKubeNamespace)
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

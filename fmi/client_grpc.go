// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package fmi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	commonv1alpha1 "github.com/ironcore-dev/metal-operator/api/gen/common/v1alpha1"
	"github.com/ironcore-dev/metal-operator/api/gen/serverbios/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientGRPC struct {
	address        string
	requestTimeout time.Duration
	tlsCredentials credentials.TransportCredentials
}

func NewClientGRPC(config ClientConfig) (*ClientGRPC, error) {
	tlsCredentials := insecure.NewCredentials()

	if config.CertFile != "" && config.KeyFile != "" && config.CAFile != "" {
		pemServerCA, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, err
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(pemServerCA) {
			return nil, fmt.Errorf("failed to add server CA's certificate")
		}

		clientCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, err
		}

		clientTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      certPool,
		}
		tlsCredentials = credentials.NewTLS(clientTLSConfig)
	}

	return &ClientGRPC{
		address:        config.ServerURL,
		requestTimeout: config.RequestTimeout,
		tlsCredentials: tlsCredentials,
	}, nil
}

func (c *ClientGRPC) Scan(ctx context.Context, serverBIOSRef string) (ScanResult, error) {
	conn, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.tlsCredentials))
	if err != nil {
		return ScanResult{}, err
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	client := v1alpha1.NewServerBIOSServiceClient(conn)
	req := &v1alpha1.BIOSScanRequest{
		ServerBiosRef: serverBIOSRef,
	}
	resp, err := client.BIOSScan(ctx, req)
	if err != nil {
		return ScanResult{}, err
	}
	if resp.Result != commonv1alpha1.RequestResult_REQUEST_RESULT_SUCCESS {
		return ScanResult{}, fmt.Errorf("unexpected result: %s", resp.Result)
	}
	return ScanResult{
		Version:  resp.Version,
		Settings: resp.Settings,
	}, nil
}

func (c *ClientGRPC) SettingsApply(ctx context.Context, serverBIOSRef string) (SettingsApplyResult, error) {
	conn, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.tlsCredentials))
	if err != nil {
		return SettingsApplyResult{}, err
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	client := v1alpha1.NewServerBIOSServiceClient(conn)
	req := &v1alpha1.BIOSSettingsApplyRequest{
		ServerBiosRef: serverBIOSRef,
	}

	resp, err := client.BIOSSettingsApply(ctx, req)
	if err != nil {
		return SettingsApplyResult{}, err
	}
	if resp.Result != commonv1alpha1.RequestResult_REQUEST_RESULT_SUCCESS {
		return SettingsApplyResult{}, fmt.Errorf("unexpected result: %s", resp.Result)
	}
	return SettingsApplyResult{RebootRequired: resp.RebootRequired}, nil
}

func (c *ClientGRPC) VersionUpdate(ctx context.Context, serverBIOSRef string) error {
	conn, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(c.tlsCredentials))
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	client := v1alpha1.NewServerBIOSServiceClient(conn)
	req := &v1alpha1.BIOSVersionUpdateRequest{
		ServerBiosRef: serverBIOSRef,
	}

	resp, err := client.BIOSVersionUpdate(ctx, req)
	if err != nil {
		return err
	}
	if resp.Result != commonv1alpha1.RequestResult_REQUEST_RESULT_SUCCESS {
		return fmt.Errorf("unexpected result: %s", resp.Result)
	}
	return nil
}

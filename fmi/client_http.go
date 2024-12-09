// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package fmi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type ClientHTTP struct {
	*http.Client
	serverURL string
}

func NewClientHTTP(config ClientConfig) (*ClientHTTP, error) {
	tlsConfig := &tls.Config{}
	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if config.CAFile != "" {
		caCert, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	tlsConfig.InsecureSkipVerify = config.InsecureSkipVerify

	transport := &http.Transport{TLSClientConfig: tlsConfig}
	httpClient := &http.Client{Transport: transport}
	return &ClientHTTP{
		Client:    httpClient,
		serverURL: config.ServerURL,
	}, nil
}

func (c *ClientHTTP) Scan(_ context.Context, serverBIOSRef string) (ScanResult, error) {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return ScanResult{}, err
	}

	resp, err := c.Post(fmt.Sprintf("%s/%s", c.serverURL, "scan"), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return ScanResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ScanResult{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result ScanResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ScanResult{}, err
	}
	return result, nil
}

func (c *ClientHTTP) SettingsApply(_ context.Context, serverBIOSRef string) (SettingsApplyResult, error) {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return SettingsApplyResult{}, err
	}

	resp, err := c.Post(
		fmt.Sprintf("%s/%s", c.serverURL, "settings-apply"), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return SettingsApplyResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return SettingsApplyResult{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return SettingsApplyResult{RebootRequired: true}, nil
}

func (c *ClientHTTP) VersionUpdate(_ context.Context, serverBIOSRef string) error {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return err
	}

	resp, err := c.Post(
		fmt.Sprintf("%s/%s", c.serverURL, "version-update"), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return nil
}

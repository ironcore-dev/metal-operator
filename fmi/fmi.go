package fmi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
)

// ClientConfig contains the configuration for a task runner client.
type ClientConfig struct {
	// ServerURL is the URL of the task runner server.
	ServerURL string

	// ScanEndpoint is the endpoint for the scan task.
	ScanEndpoint string

	// SettingsApplyEndpoint is the endpoint for the settings apply task.
	SettingsApplyEndpoint string

	// VersionUpdateEndpoint is the endpoint for the version update task.
	VersionUpdateEndpoint string

	// CAFile is the path to the CA file for TLS authentication.
	CAFile string

	// CertFile is the path to the client certificate file for TLS authentication.
	CertFile string

	// KeyFile is the path to the client key file for TLS authentication.
	KeyFile string

	// InsecureSkipVerify skips TLS verification.
	InsecureSkipVerify bool
}

// TaskPayload represents the payload for a task.
type TaskPayload struct {
	// ServerBIOSRef is the reference to the ServerBIOS object.
	ServerBIOSRef string `json:"serverBIOSRef"`
}

// ScanResult represents the result of a scan.
type ScanResult struct {
	// Version is the BIOS version of the server.
	Version string `json:"version"`

	// Settings is a map of BIOS settings and their values.
	Settings map[string]string `json:"settings"`
}

// TaskRunnerClient is the interface for a task runner client.
type TaskRunnerClient interface {
	// Scan requests a scan task.
	Scan(ctx context.Context, serverBIOSRef string) (ScanResult, error)

	// SettingsApply requests a settings apply task.
	SettingsApply(ctx context.Context, serverBIOSRef string) error

	// VersionUpdate requests a version update task.
	VersionUpdate(ctx context.Context, serverBIOSRef string) error
}

// TaskRunner is the interface for a task runner.
type TaskRunner interface {
	// ExecuteScan executes a scan task.
	ExecuteScan(ctx context.Context, serverBIOSRef string) (ScanResult, error)

	// ExecuteSettingsApply executes a settings apply task.
	ExecuteSettingsApply(ctx context.Context, serverBIOSRef string) error

	// ExecuteVersionUpdate executes a version update task.
	ExecuteVersionUpdate(ctx context.Context, serverBIOSRef string) error
}

// NewClientForConfig returns the client for the server for given config.
func NewClientForConfig(config ClientConfig) (TaskRunnerClient, error) {
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

	return &Client{
		Client:                httpClient,
		serverURL:             config.ServerURL,
		scanEndpoint:          config.ScanEndpoint,
		settingsApplyEndpoint: config.SettingsApplyEndpoint,
		versionUpdateEndpoint: config.VersionUpdateEndpoint,
	}, nil
}

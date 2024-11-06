package fmi

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClientTypeHTTP = "http"
	ClientTypeGRPC = "grpc"
)

const (
	TaskRunnerTypeDefault = "default"
	TaskRunnerTypeFake    = "fake"
)

// ServerConfig contains the configuration for a task runner server.
type ServerConfig struct {
	// Hostname is the hostname of the task runner server.
	Hostname string

	// Port is the port of the task runner server.
	Port int

	// ShutdownTimeout is the timeout to wait for the server to shut down.
	ShutdownTimeout time.Duration

	// CAFile is the path to the CA file for TLS authentication.
	CAFile string

	// CertFile is the path to the server certificate file for TLS authentication.
	CertFile string

	// KeyFile is the path to the server key file for TLS authentication.
	KeyFile string

	// KubeClient is the Kubernetes client.
	KubeClient client.Client

	// TaskRunnerType is the type of task runner to use.
	TaskRunnerType string
}

// ClientConfig contains the configuration for a task runner client.
type ClientConfig struct {
	// ServerURL is the URL of the task runner server.
	ServerURL string

	// CAFile is the path to the CA file for TLS authentication.
	CAFile string

	// CertFile is the path to the client certificate file for TLS authentication.
	CertFile string

	// KeyFile is the path to the client key file for TLS authentication.
	KeyFile string

	// InsecureSkipVerify skips TLS verification.
	InsecureSkipVerify bool

	// RequestTimeout is the timeout for requests to the task runner server.
	RequestTimeout time.Duration
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

// SettingsApplyResult represents the result of a settings apply.
type SettingsApplyResult struct {
	// RebootRequired indicates whether a reboot is required.
	RebootRequired bool `json:"success"`
}

// TaskRunnerClient is the interface for a task runner client.
type TaskRunnerClient interface {
	// Scan requests a scan task.
	Scan(ctx context.Context, serverBIOSRef string) (ScanResult, error)

	// SettingsApply requests a settings apply task.
	SettingsApply(ctx context.Context, serverBIOSRef string) (SettingsApplyResult, error)

	// VersionUpdate requests a version update task.
	VersionUpdate(ctx context.Context, serverBIOSRef string) error
}

// TaskRunner is the interface for a task runner.
type TaskRunner interface {
	// ExecuteScan executes a scan task.
	ExecuteScan(ctx context.Context, serverBIOSRef string) (ScanResult, error)

	// ExecuteSettingsApply executes a settings apply task.
	ExecuteSettingsApply(ctx context.Context, serverBIOSRef string) (SettingsApplyResult, error)

	// ExecuteVersionUpdate executes a version update task.
	ExecuteVersionUpdate(ctx context.Context, serverBIOSRef string) error
}

// NewClientForConfig returns the client for the server for given config.
func NewClientForConfig(clientType string, config ClientConfig) (TaskRunnerClient, error) {
	switch clientType {
	case ClientTypeHTTP:
		return NewClientHTTP(config)
	case ClientTypeGRPC:
		return NewClientGRPC(config)
	default:
		return nil, fmt.Errorf("unknown client type: %s", clientType)
	}
}

func NewTaskRunner(runnerType string, kubeClient client.Client, insecureBMC bool) (TaskRunner, error) {
	switch runnerType {
	case TaskRunnerTypeDefault:
		return NewDefaultTaskRunner(kubeClient, insecureBMC), nil
	case TaskRunnerTypeFake:
		return NewFakeTaskRunner(kubeClient, insecureBMC), nil
	default:
		return nil, fmt.Errorf("unknown task runner type: %s", runnerType)
	}
}

package fmi

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeTaskRunner is the fake implementation of the TaskRunner interface.
type FakeTaskRunner struct {
	client.Client
	insecure bool
}

// NewFakeTaskRunner creates a new FakeTaskRunner.
func NewFakeTaskRunner(client client.Client, insecure bool) *FakeTaskRunner {
	return &FakeTaskRunner{
		Client:   client,
		insecure: insecure,
	}
}

// ExecuteScan executes a scan task.
func (s *FakeTaskRunner) ExecuteScan(_ context.Context, _ string) (ScanResult, error) {
	return ScanResult{
		Version:  "1.0.0-fake",
		Settings: map[string]string{},
	}, nil
}

// ExecuteSettingsApply applies the settings to the server.
func (s *FakeTaskRunner) ExecuteSettingsApply(_ context.Context, _ string) (SettingsApplyResult, error) {
	return SettingsApplyResult{
		RebootRequired: true,
	}, nil
}

// todo: remove nolint

// ExecuteVersionUpdate updates the BIOS version of the server.
// nolint:unparam
func (s *FakeTaskRunner) ExecuteVersionUpdate(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

// getObjects returns the ServerBIOS and Server objects for the given serverBIOSRef.
// nolint: unused
func (s *FakeTaskRunner) getObjects(
	ctx context.Context,
	serverBIOSRef string,
) (*metalv1alpha1.ServerBIOS, *metalv1alpha1.Server, error) {
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := s.Get(ctx, client.ObjectKey{Name: serverBIOSRef}, serverBIOS); err != nil {
		return nil, nil, err
	}
	server := &metalv1alpha1.Server{}
	if err := s.Get(ctx, client.ObjectKey{Name: serverBIOS.Spec.ServerRef.Name}, server); err != nil {
		return nil, nil, err
	}
	return serverBIOS, server, nil
}

package fmi

import (
	"context"
	"fmt"
	"sync"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultTaskRunner is the default implementation of the TaskRunner interface.
type DefaultTaskRunner struct {
	client.Client
	insecure bool
	tasks    *sync.Map
}

// NewDefaultTaskRunner creates a new DefaultTaskRunner.
func NewDefaultTaskRunner(client client.Client, insecure bool) *DefaultTaskRunner {
	return &DefaultTaskRunner{
		Client:   client,
		insecure: insecure,
		tasks:    &sync.Map{},
	}
}

// ExecuteScan executes a scan task.
func (s *DefaultTaskRunner) ExecuteScan(ctx context.Context, serverBIOSRef string) (ScanResult, error) {
	serverBIOS, server, err := s.getObjects(ctx, serverBIOSRef)
	if err != nil {
		return ScanResult{}, err
	}
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, s.Client, server, s.insecure)
	if err != nil {
		return ScanResult{}, err
	}
	currentBIOSVersion, err := bmcClient.GetBiosVersion(server.Spec.UUID)
	if err != nil {
		return ScanResult{}, err
	}
	attributes := make([]string, 0)
	for k := range serverBIOS.Spec.BIOS.Settings {
		attributes = append(attributes, k)
	}
	currentSettings, err := bmcClient.GetBiosAttributeValues(server.Spec.UUID, attributes)
	if err != nil {
		return ScanResult{}, err
	}
	return ScanResult{
		Version:  currentBIOSVersion,
		Settings: currentSettings,
	}, nil
}

// ExecuteSettingsApply applies the settings to the server.
func (s *DefaultTaskRunner) ExecuteSettingsApply(ctx context.Context, serverBIOSRef string) error {
	inProgress, err := s.isTaskInProgress(ctx, serverBIOSRef)
	if err != nil {
		return err
	}
	if inProgress {
		return nil
	}

	serverBIOS, server, err := s.getObjects(ctx, serverBIOSRef)
	if err != nil {
		return err
	}
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, s.Client, server, s.insecure)
	if err != nil {
		return err
	}
	defer bmcClient.Logout()

	diff := make(map[string]string)
	for k, v := range serverBIOS.Spec.BIOS.Settings {
		if vv, ok := serverBIOS.Status.BIOS.Settings[k]; ok && vv == v {
			continue
		}
		diff[k] = v
	}
	reset, err := bmcClient.SetBiosAttributes(server.Spec.UUID, diff)
	if err != nil {
		return err
	}
	if reset {
		if err = s.patchServerCondition(ctx, server); err != nil {
			return fmt.Errorf("failed to patch Server status: %w", err)
		}
	}
	return nil
}

// todo: remove nolint

// ExecuteVersionUpdate updates the BIOS version of the server.
// nolint:unparam
func (s *DefaultTaskRunner) ExecuteVersionUpdate(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

// isTaskInProgress checks if a task is in progress.
// nolint:unparam
func (s *DefaultTaskRunner) isTaskInProgress(_ context.Context, serverBIOSRef string) (bool, error) {
	_, ok := s.tasks.Load(serverBIOSRef)
	return ok, nil
}

// storeTask stores a task for the given serverBIOSRef in the tasks map.
// nolint:unused
func (s *DefaultTaskRunner) storeTask(serverBIOSRef string) {
	s.tasks.Store(serverBIOSRef, struct{}{})
}

// dropTask drops a task for the given serverBIOSRef from the tasks map.
// nolint:unused
func (s *DefaultTaskRunner) dropTask(serverBIOSRef string) {
	s.tasks.Delete(serverBIOSRef)
}

// getObjects returns the ServerBIOS and Server objects for the given serverBIOSRef.
func (s *DefaultTaskRunner) getObjects(
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

// patchServerCondition patches the Server status with the given condition.
func (s *DefaultTaskRunner) patchServerCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	changed := meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type: "RebootRequired",
	})
	if changed {
		return s.Status().Patch(ctx, serverBase, client.MergeFrom(server))

	}
	return nil
}

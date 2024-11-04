package executor

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DefaultBIOSExecutor struct {
	client.Client
	Insecure bool
}

func New(client client.Client, insecure bool) *DefaultBIOSExecutor {
	return &DefaultBIOSExecutor{
		Client:   client,
		Insecure: insecure,
	}
}

func (e *DefaultBIOSExecutor) ExecuteScan(ctx context.Context, log logr.Logger, serverBIOSRef string) (string, map[string]string, error) {
	serverBIOS, server, err := e.getObjects(ctx, serverBIOSRef)
	if err != nil {
		return "", nil, err
	}
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, e.Client, server, e.Insecure)
	if err != nil {
		return "", nil, err
	}
	currentBIOSVersion, err := bmcClient.GetBiosVersion(server.Spec.UUID)
	if err != nil {
		return "", nil, err
	}
	attributes := make([]string, 0)
	for k := range serverBIOS.Spec.BIOS.Settings {
		attributes = append(attributes, k)
	}
	currentSettings, err := bmcClient.GetBiosAttributeValues(server.Spec.UUID, attributes)
	if err != nil {
		return "", nil, err
	}
	return currentBIOSVersion, currentSettings, nil
}

func (e *DefaultBIOSExecutor) ExecuteSettingsApply(ctx context.Context, log logr.Logger, serverBIOSRef string) error {
	serverBIOS, server, err := e.getObjects(ctx, serverBIOSRef)
	if err != nil {
		return err
	}
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, e.Client, server, e.Insecure)
	if err != nil {
		return err
	}
	defer bmcClient.Logout()

	diff := make(map[string]string)
	for k, v := range serverBIOS.Spec.BIOS.Settings {
		if v == serverBIOS.Status.BIOS.Settings[k] {
			continue
		}
		diff[k] = v
	}
	reset, err := bmcClient.SetBiosAttributes(server.Spec.UUID, diff)
	if err != nil {
		return err
	}
	if reset {
		if err = e.patchServerCondition(ctx, server); err != nil {
			return fmt.Errorf("failed to patch Server status: %w", err)
		}
	}
	return nil
}

func (e *DefaultBIOSExecutor) ExecuteVersionUpdate(ctx context.Context, log logr.Logger, serverBIOSRef string) error {
	// TODO implement me
	log.V(1).Info("BIOS version update is not implemented")
	return nil
}

func (e *DefaultBIOSExecutor) getObjects(ctx context.Context, serverBIOSRef string) (*metalv1alpha1.ServerBIOS, *metalv1alpha1.Server, error) {
	serverBIOS := &metalv1alpha1.ServerBIOS{}
	if err := e.Get(ctx, client.ObjectKey{Name: serverBIOSRef}, serverBIOS); err != nil {
		return nil, nil, err
	}
	server := &metalv1alpha1.Server{}
	if err := e.Get(ctx, client.ObjectKey{Name: serverBIOS.Spec.ServerRef.Name}, server); err != nil {
		return nil, nil, err
	}
	return serverBIOS, server, nil
}

func (e *DefaultBIOSExecutor) IsTaskInProgress(ctx context.Context, log logr.Logger, serverBIOSRef string) (bool, error) {
	// TODO implement task state tracking
	return false, nil
}

func (e *DefaultBIOSExecutor) patchServerCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	changed := meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type: "RebootRequired",
	})
	if changed {
		return e.Status().Patch(ctx, serverBase, client.MergeFrom(server))

	}
	return nil
}

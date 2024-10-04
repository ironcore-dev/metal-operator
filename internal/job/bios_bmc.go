package job

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BmcBIOSExecutor struct {
	client.Client
	Insecure bool
}

func New(c client.Client) *BmcBIOSExecutor {
	return &BmcBIOSExecutor{
		Client: c,
	}
}

func (e *BmcBIOSExecutor) Run(ctx context.Context, jobTypeString, serverBIOSRef string) error {
	jobType := metalv1alpha1.JobType(jobTypeString)
	switch jobType {
	case metalv1alpha1.UpdateBIOSVersionJobType:
		return e.UpdateBIOSVersion(ctx, serverBIOSRef)
	case metalv1alpha1.ScanBIOSVersionJobType:
		return e.ScanBIOSVersionSettings(ctx, serverBIOSRef)
	case metalv1alpha1.ApplyBIOSSettingsJobType:
		return e.ApplyBIOSSettings(ctx, serverBIOSRef)
	default:
		return fmt.Errorf("unknown job type: %s", jobTypeString)
	}
}

func (e *BmcBIOSExecutor) ApplyBIOSSettings(ctx context.Context, serverBIOSRef string) error {
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

	serverBIOSBase := serverBIOS.DeepCopy()
	serverBIOS.Status.BIOS.Settings = serverBIOS.Spec.BIOS.Settings
	serverBIOS.Status.RunningJob = metalv1alpha1.RunningJobRef{}
	return e.Patch(ctx, serverBIOSBase, client.MergeFrom(serverBIOS))
}

func (e *BmcBIOSExecutor) ScanBIOSVersionSettings(ctx context.Context, serverBIOSRef string) error {
	serverBIOS, server, err := e.getObjects(ctx, serverBIOSRef)
	if err != nil {
		return err
	}
	bmcClient, err := bmcutils.GetBMCClientForServer(ctx, e.Client, server, e.Insecure)
	if err != nil {
		return err
	}
	currentBIOSVersion, err := bmcClient.GetBiosVersion(server.Spec.UUID)
	if err != nil {
		return err
	}
	attributes := make([]string, 0)
	for k := range serverBIOS.Spec.BIOS.Settings {
		attributes = append(attributes, k)
	}
	currentSettings, err := bmcClient.GetBiosAttributeValues(server.Spec.UUID, attributes)
	if err != nil {
		return err
	}
	serverBIOSBase := serverBIOS.DeepCopy()
	serverBIOS.Status.BIOS.Version = currentBIOSVersion
	serverBIOS.Status.BIOS.Settings = currentSettings
	serverBIOS.Status.LastScanTime = metav1.Now()
	serverBIOS.Status.RunningJob = metalv1alpha1.RunningJobRef{}
	return e.Patch(ctx, serverBIOSBase, client.MergeFrom(serverBIOS))
}

func (e *BmcBIOSExecutor) UpdateBIOSVersion(ctx context.Context, serverBIOSRef string) error {
	// todo: implement me
	return nil
}

func (e *BmcBIOSExecutor) getObjects(ctx context.Context, serverBIOSRef string) (*metalv1alpha1.ServerBIOS, *metalv1alpha1.Server, error) {
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

func (e *BmcBIOSExecutor) patchServerCondition(ctx context.Context, server *metalv1alpha1.Server) error {
	serverBase := server.DeepCopy()
	changed := meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type: "RebootNeeded",
	})
	if changed {
		return e.Status().Patch(ctx, serverBase, client.MergeFrom(server))

	}
	return nil
}

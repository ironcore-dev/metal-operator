// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/ironcore-dev/metal-operator/bmc/common"
	"github.com/stmcginnis/gofish/schemas"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	metalJobLabel = "kube.ironcore.dev/job"
	registryURL   = "http://metal-registry.metal-operator-system.svc.cluster.local:30000/register"
)

var _ BMC = (*RedfishKubeBMC)(nil)

type KubeClient struct {
	kclient   client.Client
	namespace string
}

// RedfishKubeBMC is an implementation of the BMC interface for Redfish.
type RedfishKubeBMC struct {
	*RedfishBMC
	*KubeClient
}

// NewRedfishKubeBMCClient creates a new RedfishKubeBMC with the given connection details.
func NewRedfishKubeBMCClient(
	ctx context.Context,
	options Options,
	c client.Client,
	ns string,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	if UnitTestMockUps == nil {
		InitMockUp()
	}
	redfishKubeBMC := &RedfishKubeBMC{
		RedfishBMC: bmc,
		KubeClient: &KubeClient{
			kclient:   c,
			namespace: ns,
		},
	}

	return redfishKubeBMC, nil
}

// setSystemPowerState updates the power state of a system.
func (r *RedfishKubeBMC) setSystemPowerState(ctx context.Context, systemURI string, state schemas.PowerState) error {
	// Apply a 150ms delay before performing the power state change.
	time.Sleep(150 * time.Millisecond)

	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}

	system.PowerState = state
	system.RawData = nil
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to patch system to %s: %w", state, err)
	}
	return nil
}

// PowerOn powers on the system asynchronously.
func (r *RedfishKubeBMC) PowerOn(ctx context.Context, systemURI string) error {
	go func() {
		if err := r.setSystemPowerState(ctx, systemURI, schemas.OnPowerState); err != nil {
			log.FromContext(ctx).Error(err, "PowerOn failed", "systemURI", systemURI)
			return
		}

		// Apply pending BIOS settings after a delay (mock for testing).
		if len(UnitTestMockUps.PendingBIOSSetting) > 0 {
			time.Sleep(50 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBIOSSetting {
				if _, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
					UnitTestMockUps.BIOSSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBIOSSetting()
		}
	}()
	return nil
}

// PowerOff powers off the system asynchronously.
func (r *RedfishKubeBMC) PowerOff(ctx context.Context, systemURI string) error {
	go func() {
		if err := r.setSystemPowerState(ctx, systemURI, schemas.OffPowerState); err != nil {
			log.FromContext(ctx).Error(err, "PowerOff failed", "systemURI", systemURI)
		}
	}()
	return nil
}

// GetBiosPendingAttributeValues returns pending BIOS attribute values.
func (r *RedfishKubeBMC) GetBiosPendingAttributeValues(ctx context.Context, systemUUID string) (schemas.SettingsAttributes, error) {
	pending := UnitTestMockUps.PendingBIOSSetting
	if len(pending) == 0 {
		return schemas.SettingsAttributes{}, nil
	}

	result := make(schemas.SettingsAttributes, len(pending))
	for key, data := range pending {
		result[key] = data["value"]
	}
	return result, nil
}

// SetBiosAttributesOnReset sets BIOS attributes, applying them immediately or on next reset.
func (r *RedfishKubeBMC) SetBiosAttributesOnReset(ctx context.Context, systemUUID string, attributes schemas.SettingsAttributes) error {
	UnitTestMockUps.ResetPendingBIOSSetting()
	for key, value := range attributes {
		if attrData, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
			if reboot, ok := attrData["reboot"].(bool); ok && !reboot {
				attrData["value"] = value
			} else {
				UnitTestMockUps.PendingBIOSSetting[key] = map[string]any{
					"type":   attrData["type"],
					"reboot": attrData["reboot"],
					"value":  value,
				}
			}
		}
	}
	return nil
}

// GetBiosAttributeValues retrieves specific BIOS attribute values.
func (r *RedfishKubeBMC) GetBiosAttributeValues(ctx context.Context, systemUUID string, attributes []string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BIOS attributes: %w", err)
	}

	result := make(schemas.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if attrData, ok := UnitTestMockUps.BIOSSettingAttr[name]; ok && filtered[name].AttributeName != "" {
			result[name] = attrData["value"]
		}
	}
	return result, nil
}

// getFilteredBiosRegistryAttributes returns filtered BIOS registry attributes.
func (r *RedfishKubeBMC) getFilteredBiosRegistryAttributes(readOnly, immutable bool) (map[string]RegistryEntryAttributes, error) {
	if len(UnitTestMockUps.BIOSSettingAttr) == 0 {
		return nil, fmt.Errorf("no BIOS setting attributes found")
	}

	filtered := make(map[string]RegistryEntryAttributes)
	for name, attrData := range UnitTestMockUps.BIOSSettingAttr {
		resetRequired := attrData["reboot"].(bool)
		filtered[name] = RegistryEntryAttributes{
			AttributeName: name,
			Immutable:     immutable,
			ReadOnly:      readOnly,
			Type:          attrData["type"].(string),
			ResetRequired: &resetRequired,
		}
	}
	return filtered, nil
}

// CheckBiosAttributes validates BIOS attributes.
func (r *RedfishKubeBMC) CheckBiosAttributes(attrs schemas.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return r.checkAttributes(attrs, filtered)
}

// GetBiosVersion retrieves the BIOS version.
func (r *RedfishKubeBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BIOSVersion == "" {
		var err error
		UnitTestMockUps.BIOSVersion, err = r.RedfishBMC.GetBiosVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BIOS version: %w", err)
		}
	}
	return UnitTestMockUps.BIOSVersion, nil
}

// UpgradeBiosVersion initiates a BIOS upgrade.
func (r *RedfishKubeBMC) UpgradeBiosVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BIOSUpgradeTaskIndex = 0
	UnitTestMockUps.BIOSUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BIOSUpgradeTaskIndex < len(UnitTestMockUps.BIOSUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BIOSUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBiosUpgradeTask retrieves the status of a BIOS upgrade task.
func (r *RedfishKubeBMC) GetBiosUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	index := UnitTestMockUps.BIOSUpgradeTaskIndex
	if index >= len(UnitTestMockUps.BIOSUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BIOSUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BIOSUpgradeTaskStatus[index]
	if task.TaskState == schemas.CompletedTaskState {
		UnitTestMockUps.BIOSVersion = UnitTestMockUps.BIOSUpgradingVersion
	}
	return task, nil
}

// ResetManager resets the BMC with a delay for pending settings.
func (r *RedfishKubeBMC) ResetManager(ctx context.Context, UUID string, resetType schemas.ResetType) error {
	go func() {
		if len(UnitTestMockUps.PendingBMCSetting) > 0 {
			time.Sleep(150 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBMCSetting {
				if _, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
					UnitTestMockUps.BMCSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBMCSetting()
		}
	}()
	return nil
}

// SetBMCAttributesImmediately sets BMC attributes, applying them immediately or on reset.
func (r *RedfishKubeBMC) SetBMCAttributesImmediately(ctx context.Context, UUID string, attributes schemas.SettingsAttributes) error {
	for key, value := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok {
			if reboot, ok := attrData["reboot"].(bool); ok && !reboot {
				attrData["value"] = value
			} else {
				UnitTestMockUps.PendingBMCSetting[key] = map[string]any{
					"type":   attrData["type"],
					"reboot": attrData["reboot"],
					"value":  value,
				}
			}
		}
	}
	return nil
}

// GetBMCAttributeValues retrieves specific BMC attribute values.
func (r *RedfishKubeBMC) GetBMCAttributeValues(ctx context.Context, UUID string, attributes map[string]string) (schemas.SettingsAttributes, error) {
	if len(attributes) == 0 {
		return nil, nil
	}

	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get filtered BMC attributes: %w", err)
	}

	result := make(schemas.SettingsAttributes, len(attributes))
	for key := range attributes {
		if attrData, ok := UnitTestMockUps.BMCSettingAttr[key]; ok && filtered[key].AttributeName != "" {
			result[key] = attrData["value"]
		}
	}
	return result, nil
}

// GetBMCPendingAttributeValues returns pending BMC attribute values.
func (r *RedfishKubeBMC) GetBMCPendingAttributeValues(ctx context.Context, systemUUID string) (schemas.SettingsAttributes, error) {
	pending := UnitTestMockUps.PendingBMCSetting
	if len(pending) == 0 {
		return schemas.SettingsAttributes{}, nil
	}

	result := make(schemas.SettingsAttributes, len(pending))
	for key, data := range pending {
		result[key] = data["value"]
	}
	return result, nil
}

// getFilteredBMCRegistryAttributes returns filtered BMC registry attributes.
func (r *RedfishKubeBMC) getFilteredBMCRegistryAttributes(readOnly, immutable bool) (map[string]schemas.Attributes, error) {
	if len(UnitTestMockUps.BMCSettingAttr) == 0 {
		return nil, fmt.Errorf("no BMC setting attributes found")
	}

	filtered := make(map[string]schemas.Attributes)
	for name, attrData := range UnitTestMockUps.BMCSettingAttr {
		filtered[name] = schemas.Attributes{
			AttributeName: name,
			Immutable:     immutable,
			ReadOnly:      readOnly,
			Type:          attrData["type"].(schemas.AttributeType),
			ResetRequired: attrData["reboot"].(bool),
		}
	}
	return filtered, nil
}

// CheckBMCAttributes validates BMC attributes.
func (r *RedfishKubeBMC) CheckBMCAttributes(ctx context.Context, UUID string, attrs schemas.SettingsAttributes) (bool, error) {
	filtered, err := r.getFilteredBMCRegistryAttributes(false, false)
	if err != nil || len(filtered) == 0 {
		return false, err
	}
	return common.CheckAttribues(attrs, filtered)
}

// GetBMCVersion retrieves the BMC version.
func (r *RedfishKubeBMC) GetBMCVersion(ctx context.Context, systemUUID string) (string, error) {
	if UnitTestMockUps.BMCVersion == "" {
		var err error
		UnitTestMockUps.BMCVersion, err = r.RedfishBMC.GetBMCVersion(ctx, systemUUID)
		if err != nil {
			return "", fmt.Errorf("failed to get BMC version: %w", err)
		}
	}
	return UnitTestMockUps.BMCVersion, nil
}

// UpgradeBMCVersion initiates a BMC upgrade.
func (r *RedfishKubeBMC) UpgradeBMCVersion(ctx context.Context, manufacturer string, params *schemas.UpdateServiceSimpleUpdateParameters) (string, bool, error) {
	UnitTestMockUps.BMCUpgradeTaskIndex = 0
	UnitTestMockUps.BMCUpgradingVersion = params.ImageURI
	go func() {
		time.Sleep(20 * time.Millisecond)
		for UnitTestMockUps.BMCUpgradeTaskIndex < len(UnitTestMockUps.BMCUpgradeTaskStatus)-1 {
			time.Sleep(5 * time.Millisecond)
			UnitTestMockUps.BMCUpgradeTaskIndex++
		}
	}()
	return DummyMockTaskForUpgrade, false, nil
}

// GetBMCUpgradeTask retrieves the status of a BMC upgrade task.
func (r *RedfishKubeBMC) GetBMCUpgradeTask(ctx context.Context, manufacturer, taskURI string) (*schemas.Task, error) {
	index := UnitTestMockUps.BMCUpgradeTaskIndex
	if index >= len(UnitTestMockUps.BMCUpgradeTaskStatus) {
		index = len(UnitTestMockUps.BMCUpgradeTaskStatus) - 1
	}
	task := &UnitTestMockUps.BMCUpgradeTaskStatus[index]
	if task.TaskState == schemas.CompletedTaskState {
		UnitTestMockUps.BMCVersion = UnitTestMockUps.BMCUpgradingVersion
	}
	return task, nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishKubeBMC) SetPXEBootOnce(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	var setBoot schemas.Boot
	// TODO: cover setting BootSourceOverrideMode with BIOS settings profile
	// Only skip setting BootSourceOverrideMode for older BMCs that don't report it
	if system.Boot.BootSourceOverrideMode != "" {
		setBoot = pxeBootWithSettingUEFIBootMode
	} else {
		setBoot = pxeBootWithoutSettingUEFIBootMode
	}
	if err := system.SetBoot(&setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	netData := `{"networkInterfaces":[{"name":"dummy0","ipAddresses":["127.0.0.2"],"macAddress":"aa:bb:cc:dd:ee:ff"}]`
	curlCmd := fmt.Sprintf(
		`apk add curl && curl -H 'Content-Type: application/json' \
-d '{"SystemUUID":"%s","data":%s}}' \
-X POST %s`,
		system.UUID, netData, registryURL)
	cmd := []string{
		"/bin/sh",
		"-c",
		curlCmd,
	}
	if err := r.createJob(ctx, r.kclient, cmd, r.namespace, system.UUID); err != nil {
		return fmt.Errorf("failed to create job for system %s: %w", system.UUID, err)
	}
	return nil
}

func (r *RedfishKubeBMC) createJob(
	ctx context.Context,
	c client.Client,
	cmd []string,
	namespace,
	systemUUID string,
) error {
	// Check if a job with the same label already exists
	jobList := &v1.JobList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{metalJobLabel: systemUUID},
	}
	if err := c.List(ctx, jobList, listOpts...); err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}
	if len(jobList.Items) > 0 {
		return nil // Job already exists, do not create a new one
	}

	job := &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("register-%s-", systemUUID),
			Namespace:    namespace,
			Labels: map[string]string{
				metalJobLabel: systemUUID,
			},
		},
		Spec: v1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						metalJobLabel: systemUUID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "registry-job",
							Image:   "alpine:latest",
							Command: cmd,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
			TTLSecondsAfterFinished: ptr.To(int32(30)),
		},
	}
	if err := c.Create(ctx, job); err != nil {
		return err
	}
	return nil
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stmcginnis/gofish/redfish"
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
	client    client.Client
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
	bmcOptions Options,
	c client.Client,
	ns string,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, bmcOptions)
	if err != nil {
		return nil, err
	}
	redfishKubeBMC := &RedfishKubeBMC{
		RedfishBMC: bmc,
		KubeClient: &KubeClient{
			client:    c,
			namespace: ns,
		},
	}

	return redfishKubeBMC, nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishKubeBMC) SetPXEBootOnce(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	var setBoot redfish.Boot
	// TODO: cover setting BootSourceOverrideMode with BIOS settings profile
	if system.Boot.BootSourceOverrideMode != redfish.UEFIBootSourceOverrideMode {
		setBoot = pxeBootWithSettingUEFIBootMode
	} else {
		setBoot = pxeBootWithoutSettingUEFIBootMode
	}
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	netData := `{"networkInterfaces":[{"name":"dummy0","ipAddress":"127.0.0.2","macAddress":"aa:bb:cc:dd:ee:ff"}]`
	curlCmd := fmt.Sprintf(
		`apk add curl && curl -H 'Content-Type: application/json' \
-d '{"SystemUUID":"%s","data":%s}}' \
-X POST %s`,
		systemUUID, netData, registryURL)
	cmd := []string{
		"/bin/sh",
		"-c",
		curlCmd,
	}
	if err := r.createJob(context.TODO(), r.KubeClient.client, cmd, r.KubeClient.namespace, systemUUID); err != nil {
		return fmt.Errorf("failed to create job for system %s: %w", systemUUID, err)
	}
	return nil
}

func (r RedfishKubeBMC) createJob(
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

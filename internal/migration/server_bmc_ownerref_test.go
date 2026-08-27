// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

func TestServerBMCOwnerReferenceMigration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := metalv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	legacy := &metalv1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         metalv1alpha1.GroupVersion.String(),
				Kind:               "BMC",
				Name:               "some-bmc",
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			}},
		},
		Spec: metalv1alpha1.ServerSpec{
			BMCRef: &corev1.LocalObjectReference{Name: "some-bmc"},
		},
	}
	clean := &metalv1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: "clean", Namespace: "default"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy, clean).Build()
	m := &ServerBMCOwnerReferenceMigration{Client: c}

	if err := m.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	got := &metalv1alpha1.Server{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(legacy), got); err != nil {
		t.Fatal(err)
	}
	if len(got.OwnerReferences) != 0 {
		t.Errorf("legacy Server still has owner references: %v", got.OwnerReferences)
	}
	if got.Spec.BMCRef == nil || got.Spec.BMCRef.Name != "some-bmc" {
		t.Errorf("legacy Server BMCRef was modified: %v", got.Spec.BMCRef)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(clean), got); err != nil {
		t.Fatal(err)
	}
	if len(got.OwnerReferences) != 0 {
		t.Errorf("clean Server gained owner references: %v", got.OwnerReferences)
	}
}

// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

func TestEndpointOwnerReferenceMigration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := metalv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         metalv1alpha1.GroupVersion.String(),
		Kind:               "Endpoint",
		Name:               "some-endpoint",
		Controller:         new(true),
		BlockOwnerDeletion: new(true),
	}
	legacyBMC := &metalv1alpha1.BMC{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-bmc", OwnerReferences: []metav1.OwnerReference{ownerRef}},
	}
	legacySecret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-secret", OwnerReferences: []metav1.OwnerReference{ownerRef}},
	}
	cleanBMC := &metalv1alpha1.BMC{
		ObjectMeta: metav1.ObjectMeta{Name: "clean-bmc"},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacyBMC, legacySecret, cleanBMC).Build()
	m := &EndpointOwnerReferenceMigration{Client: c}

	if err := m.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	for _, obj := range []client.Object{
		&metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{Name: "legacy-bmc"}},
		&metalv1alpha1.BMCSecret{ObjectMeta: metav1.ObjectMeta{Name: "legacy-secret"}},
		&metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{Name: "clean-bmc"}},
	} {
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatal(err)
		}
		if len(obj.GetOwnerReferences()) != 0 {
			t.Errorf("%T %s has owner references: %v", obj, obj.GetName(), obj.GetOwnerReferences())
		}
	}
}

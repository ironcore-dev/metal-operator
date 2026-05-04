// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerMaintenance Webhook", func() {

	var (
		ctx       context.Context
		server    *metalv1alpha1.Server
		maint     *metalv1alpha1.ServerMaintenance
		validator ServerMaintenanceValidator
	)

	BeforeEach(func() {
		ctx = context.Background()

		// create a valid Server first
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-server",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "1234",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		validator = ServerMaintenanceValidator{Client: k8sClient}
		komega.SetClient(k8sClient)

		maint = &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-maintenance",
				Namespace: "default",
			},
			Spec: metalv1alpha1.ServerMaintenanceSpec{
				ServerRef: &v1.LocalObjectReference{
					Name: "test-server",
				},
			},
		}
	})

	AfterEach(func(ctx context.Context) {
		_ = k8sClient.DeleteAllOf(ctx, &metalv1alpha1.Server{})
		_ = k8sClient.DeleteAllOf(ctx, &metalv1alpha1.ServerMaintenance{})
	})

	Context("ValidateCreate", func() {

		It("should allow creation when Server exists", func() {
			_, err := validator.ValidateCreate(ctx, maint)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject creation when Server does NOT exist", func() {
			maint.Spec.ServerRef.Name = "non-existent-server"

			_, err := validator.ValidateCreate(ctx, maint)
			Expect(err).To(HaveOccurred())
		})

		It("should reject creation when ServerRef is empty", func() {
			maint.Spec.ServerRef = &v1.LocalObjectReference{
				Name: "",
			}

			_, err := validator.ValidateCreate(ctx, maint)
			Expect(err).To(HaveOccurred())
		})

		It("should reject creation when ServerRef is nil", func() {
			maint.Spec.ServerRef = nil

			_, err := validator.ValidateCreate(ctx, maint)
			Expect(err).To(HaveOccurred())
		})
	})
})

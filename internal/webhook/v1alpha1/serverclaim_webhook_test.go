// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerClaim Webhook", func() {
	var (
		serverClaim *metalv1alpha1.ServerClaim
		validator   ServerClaimCustomValidator
	)

	BeforeEach(func() {
		serverClaim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "default",
				GenerateName: "test-claim-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power: metalv1alpha1.PowerOff,
				ServerRef: &corev1.LocalObjectReference{
					Name: "test-server",
				},
				Image: "foo:latest",
			},
		}
		validator = ServerClaimCustomValidator{Client: k8sClient}
	})

	Context("When creating a ServerClaim under Validating Webhook", func() {
		It("should reject a ServerClaim with the deprecated approval annotation", func() {
			serverClaim.Annotations = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			Expect(validator.ValidateCreate(ctx, serverClaim)).Error().To(HaveOccurred())
		})

		It("should reject a ServerClaim with the deprecated approval label", func() {
			serverClaim.Labels = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			Expect(validator.ValidateCreate(ctx, serverClaim)).Error().To(HaveOccurred())
		})

		It("should reject a ServerClaim with the deprecated key in both annotations and labels", func() {
			serverClaim.Annotations = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			serverClaim.Labels = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			_, err := validator.ValidateCreate(ctx, serverClaim)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(metalv1alpha1.ServerMaintenanceApprovalKey))
			Expect(err.Error()).To(ContainSubstring(metalv1alpha1.ServerMaintenanceApprovedLabelKey))
		})

		It("should allow a ServerClaim with the current approved annotation", func() {
			serverClaim.Annotations = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovedLabelKey: "true",
			}
			Expect(validator.ValidateCreate(ctx, serverClaim)).Error().NotTo(HaveOccurred())
		})

		It("should allow a ServerClaim with no approval keys", func() {
			Expect(validator.ValidateCreate(ctx, serverClaim)).Error().NotTo(HaveOccurred())
		})
	})

	Context("When updating a ServerClaim under Validating Webhook", func() {
		It("should reject an update that adds the deprecated approval annotation", func() {
			updated := serverClaim.DeepCopy()
			updated.Annotations = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			Expect(validator.ValidateUpdate(ctx, serverClaim, updated)).Error().To(HaveOccurred())
		})

		It("should reject an update that adds the deprecated approval label", func() {
			updated := serverClaim.DeepCopy()
			updated.Labels = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovalKey: "true",
			}
			Expect(validator.ValidateUpdate(ctx, serverClaim, updated)).Error().To(HaveOccurred())
		})

		It("should allow an update that uses the current approved label", func() {
			updated := serverClaim.DeepCopy()
			updated.Labels = map[string]string{
				metalv1alpha1.ServerMaintenanceApprovedLabelKey: "true",
			}
			Expect(validator.ValidateUpdate(ctx, serverClaim, updated)).Error().NotTo(HaveOccurred())
		})

		It("should allow an update with no approval keys", func() {
			updated := serverClaim.DeepCopy()
			Expect(validator.ValidateUpdate(ctx, serverClaim, updated)).Error().NotTo(HaveOccurred())
		})
	})
})

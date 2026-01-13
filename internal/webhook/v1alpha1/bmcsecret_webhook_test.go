// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMCSecret Webhook", func() {
	var (
		BMCSecret *metalv1alpha1.BMCSecret
		validator BMCSecretCustomValidator
	)

	BeforeEach(func() {
		validator = BMCSecretCustomValidator{
			Client: k8sClient,
		}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		BMCSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "ns.Name",
				GenerateName: "test-bmc-secret",
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("adminpass"),
			},
			Immutable: &[]bool{true}[0],
		}
		By("Creating an BMCSecret")
		Expect(k8sClient.Create(ctx, BMCSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSecret)
		SetClient(k8sClient)

	})

	Context("When creating or updating BMCSecret under Validating Webhook", func() {
		It("Should deny Update BMCSecret if Immutable is set to True", func(ctx SpecContext) {
			By("Updating an BMCSecret with Immutable set to True")
			BMCSecretUpdated := BMCSecret.DeepCopy()
			BMCSecretUpdated.Data["username"] = []byte("newadmin")
			Expect(validator.ValidateUpdate(ctx, BMCSecret, BMCSecretUpdated)).Error().To(HaveOccurred())
		})

		It("Should allow Update BMCSecret if Immutable is set to False", func(ctx SpecContext) {
			By("Updating an BMCSecret with Immutable set to False")
			BMCSecretMutable := BMCSecret.DeepCopy()
			BMCSecretMutable.Immutable = ptr.To(false)
			Expect(k8sClient.Update(ctx, BMCSecretMutable)).To(Succeed())

			BMCSecretUpdated := BMCSecretMutable.DeepCopy()
			BMCSecretUpdated.Data["username"] = []byte("newadmin")
			Expect(validator.ValidateUpdate(ctx, BMCSecretMutable, BMCSecretUpdated)).Error().NotTo(HaveOccurred())
		})
	})

})

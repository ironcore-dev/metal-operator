// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMCSecret Webhook", func() {
	var (
<<<<<<< HEAD
		obj       *metalv1alpha1.BMCSecret
		oldObj    *metalv1alpha1.BMCSecret
		validator BMCSecretValidator
	)

	BeforeEach(func() {
		obj = &metalv1alpha1.BMCSecret{}
		oldObj = &metalv1alpha1.BMCSecret{}
		validator = BMCSecretValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
=======
		BMCSecret *metalv1alpha1.BMCSecret
		validator BMCSecretCustomValidator
	)

	BeforeEach(func() {
		validator = BMCSecretCustomValidator{
			Client: k8sClient,
		}
		BMCSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    "test-namespace",
				GenerateName: "test-bmc-secret",
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("adminpass"),
			},
		}
		By("Creating a BMCSecret")
		Expect(k8sClient.Create(ctx, BMCSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, BMCSecret)
>>>>>>> tmp-original-15-09-26-00-47
	})

	Context("When creating or updating BMCSecret under Validating Webhook", func() {
		It("should deny update BMCSecret if immutable is set to true", func(ctx SpecContext) {
			By("Setting Immutable to True")
			Eventually(Update(BMCSecret, func() {
				BMCSecret.Immutable = new(true)
			})).Should(Succeed())

			By("Updating an BMCSecret with Immutable set to True")
			BMCSecretUpdated := BMCSecret.DeepCopy()
			BMCSecretUpdated.Data["username"] = []byte("newadmin")
			Expect(validator.ValidateUpdate(ctx, BMCSecret, BMCSecretUpdated)).Error().To(HaveOccurred())
		})

		It("should allow update BMCSecret if immutable is set to false", func(ctx SpecContext) {
			By("Updating an BMCSecret with Immutable set to False")
			BMCSecretMutable := BMCSecret.DeepCopy()
			BMCSecretMutable.Immutable = new(false)
			Expect(k8sClient.Update(ctx, BMCSecretMutable)).To(Succeed())

			BMCSecretUpdated := BMCSecretMutable.DeepCopy()
			BMCSecretUpdated.Data["username"] = []byte("newadmin")
			Expect(validator.ValidateUpdate(ctx, BMCSecretMutable, BMCSecretUpdated)).Error().NotTo(HaveOccurred())
		})
	})

})

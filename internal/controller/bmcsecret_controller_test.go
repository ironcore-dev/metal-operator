// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("BMCSecret Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		bmcsecret := &metalv1alpha1.BMCSecret{}

		BeforeEach(func(ctx SpecContext) {
			By("creating the custom resource for the Kind BMCSecret")
			err := k8sClient.Get(ctx, typeNamespacedName, bmcsecret)
			if err != nil && errors.IsNotFound(err) {
				resource := &metalv1alpha1.BMCSecret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func(ctx SpecContext) {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &metalv1alpha1.BMCSecret{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance BMCSecret")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func(ctx SpecContext) {
			By("Reconciling the created resource")
			controllerReconciler := &BMCSecretReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).To(Succeed())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

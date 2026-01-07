// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/controller"
)

var _ = Describe("Server Webhook", func() {
	var (
		server    *metalv1alpha1.Server
		validator ServerCustomValidator
	)

	BeforeEach(func() {
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		validator = ServerCustomValidator{Client: k8sClient}
		SetClient(k8sClient)
		By("Creating an server")
	})

	AfterEach(func(ctx context.Context) {
		By("Deleting the server")
		Expect(k8sClient.DeleteAllOf(ctx, server)).To(Succeed())

		By("Ensuring clean state")
		controller.EnsureCleanState()
	})

	Context("When deleting Server under Validating Webhook", func() {
		It("Should refuse to delete if in Maintenance", func() {
			By("Patching the server to a maintenance state and adding finalizer")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateMaintenance
			})).Should(Succeed())

			By("Deleting the Server should not pass")
			Expect(validator.ValidateDelete(ctx, server)).Error().To(HaveOccurred())

			By("Patching the server to have force delete annotation")
			Eventually(Update(server, func() {
				server.Annotations = map[string]string{
					metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress,
				}
			})).Should(Succeed())
		})
	})
})

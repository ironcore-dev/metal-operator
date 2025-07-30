// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
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
				Power:      metalv1alpha1.PowerUnmanaged,
				UUID:       "38947555-7742-3448-3784-823347823834",
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
		DeferCleanup(k8sClient.Delete, server)
		validator = ServerCustomValidator{Client: k8sClient}
		SetClient(k8sClient)
		By("Creating an server")
	})

	Context("When deleting Server under Validating Webhook", func() {
		It("Should refuse to delete if in Maintenance", func() {
			By("Patching the server to a maintenance state and adding finalizer")
			Eventually(UpdateStatus(server, func() {
				server.Status.State = metalv1alpha1.ServerStateMaintenance
			})).Should(Succeed())
			Eventually(Update(server, func() {
				server.Finalizers = append(server.Finalizers, controller.ServerFinalizer)
			})).Should(Succeed())

			By("Deleting the Server should not pass")
			Expect(validator.ValidateDelete(ctx, server)).Error().To(HaveOccurred())

			By("Patching the server to have force delete annotation")
			Eventually(Update(server, func() {
				server.Annotations = map[string]string{
					metalv1alpha1.ForceUpdateAnnotation: metalv1alpha1.OperationAnnotationForceUpdateOrDeleteInProgress,
				}
			})).Should(Succeed())

			By("Deleting the server should pass: by DeferCleanup")
		})
	})

})

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
		By("Creating a Server")
	})

	AfterEach(func(ctx context.Context) {
		By("Deleting the server")
		Expect(k8sClient.DeleteAllOf(ctx, server)).To(Succeed())
	})

	Context("When deleting Server under Validating Webhook", func() {
		It("should refuse to delete if in Maintenance", func() {
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

	Context("When updating ServerClaimRef under CEL validation", func() {
		It("should reject changing the name of an existing ServerClaimRef", func() {
			By("Setting a ServerClaimRef")
			Eventually(Update(server, func() {
				server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
					Namespace: "default",
					Name:      "claim-a",
				}
			})).Should(Succeed())

			By("Trying to change the name")
			Eventually(Object(server)).Should(HaveField("Spec.ServerClaimRef.Name", "claim-a"))
			Expect(Update(server, func() {
				server.Spec.ServerClaimRef.Name = "claim-b"
			})()).To(Not(Succeed()))
		})

		It("should reject changing the namespace of an existing ServerClaimRef", func() {
			By("Setting a ServerClaimRef")
			Eventually(Update(server, func() {
				server.Spec.ServerClaimRef = &metalv1alpha1.ImmutableObjectReference{
					Namespace: "ns-a",
					Name:      "claim",
				}
			})).Should(Succeed())

			By("Trying to change the namespace")
			Eventually(Object(server)).Should(HaveField("Spec.ServerClaimRef.Namespace", "ns-a"))
			Expect(Update(server, func() {
				server.Spec.ServerClaimRef.Namespace = "ns-b"
			})()).To(Not(Succeed()))
		})
	})
})

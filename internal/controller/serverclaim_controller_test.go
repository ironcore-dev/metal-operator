// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerClaim Controller", func() {
	ns := SetupTest(nil)

	var (
		server    *metalv1alpha1.Server
		bmcSecret *metalv1alpha1.BMCSecret
	)

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret = &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a Server")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-claim-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: MockServerPort,
					},
					Address: MockServerIP,
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		EnsureCleanState(ctx)
	})

	It("should successfully claim a server in available state", func(ctx SpecContext) {
		By("Creating an Ignition secret")
		ignitionSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				"foo": []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, ignitionSecret)).To(Succeed())

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:             metalv1alpha1.PowerOn,
				ServerRef:         &v1.LocalObjectReference{Name: server.Name},
				IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
				Image:             "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerBootConfiguration has been created")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerClaim",
				Name:               claim.Name,
				UID:                claim.UID,
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			})),
			HaveField("Spec.ServerRef.Name", server.Name),
			HaveField("Spec.Image", "foo:bar"),
			HaveField("Spec.IgnitionSecretRef.Name", ignitionSecret.Name),
		))

		By("Ensuring that the server has a correct boot configuration ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", &metalv1alpha1.ObjectReference{
				Namespace: ns.Name,
				Name:      config.Name,
			}),
		))
		By("Patching the boot configuration to a Ready state")
		Eventually(UpdateStatus(config, func() {
			config.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed(), fmt.Sprintf("Unable to set the bootconfig %v to Ready State", config))

		By("Ensuring that the Server has the correct PowerStatus")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(claim.Spec.Power)),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
	})

	It("should allow deletion of ServerClaim without a Server", func(ctx SpecContext) {
		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: "non-existent-server"},
				Image:     "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should stand down and not revert power while the bound server is parked", func(ctx SpecContext) {
		By("Creating a ServerClaim with PowerOn")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "parked-stand-down-claim",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				Image:     "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring the server is reserved and the claim is bound")
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateReserved))

		By("Patching the boot configuration to a Ready state so the claim binds and powers on")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(UpdateStatus(config, func() {
			config.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		})).Should(Succeed())
		Eventually(Object(claim)).Should(HaveField("Status.Phase", metalv1alpha1.PhaseBound))
		Eventually(Object(server)).Should(HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState))

		By("Parking the server")
		Eventually(Update(server, func() {
			metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationPark)
		})).Should(Succeed())
		Eventually(Object(server)).Should(HaveField("Status.State", metalv1alpha1.ServerStateParked))

		By("Ensuring the server is powered off while parked")
		Eventually(Object(server)).Should(HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState))

		By("Resuming the server via an unpark request")
		Eventually(Update(server, func() {
			metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
		})).Should(Succeed())

		By("Ensuring the server resumes to reserved and the state machine re-applies power")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
		))

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Eventually(Get(claim)).To(Satisfy(apierrors.IsNotFound))
	})
})

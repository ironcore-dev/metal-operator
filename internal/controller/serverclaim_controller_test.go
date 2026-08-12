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

		By("Ensuring that the Server has the correct claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef.Name", claim.Name),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(serverClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseBound),
			HaveField("Spec.ServerRef", Not(BeNil())),
			HaveField("Spec.ServerRef.Name", server.Name),
		))

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
			HaveField("Spec.Power", BeEmpty()),
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

	It("should not unbind an already-bound claim when the server is cordoned", func(ctx SpecContext) {
		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-cordon-bound-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				Image:     "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring the server is claimed")
		Eventually(Object(claim)).Should(
			HaveField("Status.Phase", Equal(metalv1alpha1.PhaseBound)),
		)

		By("Cordoning the server after binding")
		Eventually(Update(server, func() {
			server.Spec.Unclaimable = true
		})).Should(Succeed())

		By("Ensuring the existing claim stays bound")
		Consistently(Object(claim)).Should(
			HaveField("Status.Phase", Equal(metalv1alpha1.PhaseBound)),
		)
		By("Ensuring the claim ref remains in place")
		Consistently(Object(server)).Should(
			HaveField("Spec.ServerClaimRef.Name", claim.Name),
		)

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
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

		By("Asserting spec.power stays cleared while parked")
		Consistently(Object(server)).Should(HaveField("Spec.Power", BeEmpty()))

		By("Resuming the server via an unpark request")
		Eventually(Update(server, func() {
			metav1.SetMetaDataAnnotation(&server.ObjectMeta, metalv1alpha1.OperationAnnotation, metalv1alpha1.OperationAnnotationUnpark)
		})).Should(Succeed())

		By("Ensuring the server resumes to reserved and the state machine re-applies power")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Status.PowerState", metalv1alpha1.ServerOnPowerState),
			HaveField("Spec.Power", BeEmpty()),
		))

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Eventually(Get(claim)).To(Satisfy(apierrors.IsNotFound))
	})
})

var _ = Describe("ServerClaim Validation", func() {
	ns := SetupTest(nil)

	var claim *metalv1alpha1.ServerClaim
	var claimWithSelector *metalv1alpha1.ServerClaim

	BeforeEach(func(ctx SpecContext) {
		By("Creating a new ServerClaim")
		claim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Updating the ServerRef to claim a Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("Creating a new ServerClaim")
		claimWithSelector = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claimWithSelector)).To(Succeed())

		By("Updating the ServerSelector to claim a Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Expect(k8sClient.Delete(ctx, claimWithSelector)).To(Succeed())
		EnsureCleanState(ctx)
	})

	It("should deny if the ServerRef changes", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "bar"}
		})).Should(HaveOccurred())

		By("Ensuring that the ServerRef did not change")
		Consistently(Object(claim)).Should(HaveField("Spec.ServerRef.Name", Equal("foo")))
	})

	It("should allow a change of ServerClaim by not changing the ServerRef", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.Power = metalv1alpha1.PowerOn
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("Ensuring that the PowerState changed")
		Consistently(Object(claim)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerRef.Name", Equal("foo")),
		))
	})

	It("should deny if the ServerSelector changes", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bar": "foo",
				},
			}
		})).Should(HaveOccurred())

		By("ensuring that the ServerRef did not change")
		Consistently(Object(claimWithSelector)).Should(
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"})))
	})

	It("should allow a change of ServerClaim by not changing the ServerSelector", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.Power = metalv1alpha1.PowerOn
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())

		By("Ensuring that the PowerState changed")
		Consistently(Object(claimWithSelector)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"}))))
	})
})

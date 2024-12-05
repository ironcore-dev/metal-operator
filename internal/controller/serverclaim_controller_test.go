// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerClaim Controller", func() {
	ns := SetupTest()

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, bmcSecret)

		By("Creating a Server")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerSpec{
				UUID:       "38947555-7742-3448-3784-823347823834",
				SystemUUID: "38947555-7742-3448-3784-823347823834",
				BMC: &metalv1alpha1.BMCAccess{
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolRedfishLocal,
						Port: 8000,
					},
					Address: "127.0.0.1",
					BMCSecretRef: v1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).Should(Succeed())
		DeferCleanup(k8sClient.Delete, server)
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
		DeferCleanup(k8sClient.Delete, ignitionSecret)

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

		By("Patching the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the Server has the correct claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef.Name", claim.Name),
			HaveField("Spec.Power", metalv1alpha1.PowerOn),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
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
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.ServerRef.Name", server.Name),
			HaveField("Spec.Image", "foo:bar"),
			HaveField("Spec.IgnitionSecretRef.Name", ignitionSecret.Name),
		))

		By("Ensuring that the server has a correct boot configuration ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", &v1.ObjectReference{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       config.Name,
				UID:        config.UID,
			}),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
	})

	It("Should successfully claim a server by reference and label selector", func(ctx SpecContext) {
		By("Patching Server labels")
		Eventually(Update(server, func() {
			server.Labels = map[string]string{
				"type": "storage",
				"env":  "staging",
			}
		})).Should(Succeed())

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOff,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": "storage"},
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "env",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"test", "staging"},
					}},
				},
				Image: "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Patching the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the Server has the correct claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef.Name", claim.Name),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseBound),
			HaveField("Spec.ServerRef", Not(BeNil())),
			HaveField("Spec.ServerRef.Name", server.Name),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
	})

	It("should successfully claim a server by label selector", func(ctx SpecContext) {
		By("Patching Server labels")
		Eventually(Update(server, func() {
			server.Labels = map[string]string{
				"type": "storage",
				"env":  "prod",
			}
		})).Should(Succeed())

		By("Patching the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power: metalv1alpha1.PowerOff,
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": "storage"},
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "env",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"prod"},
					}},
				},
				Image: "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Spec.ServerRef", Equal(&v1.LocalObjectReference{Name: server.Name})),
			HaveField("Status.Phase", metalv1alpha1.PhaseBound),
		))

		By("Ensuring that the Server has the correct claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", &v1.ObjectReference{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "ServerClaim",
				Name:       claim.Name,
				Namespace:  claim.Namespace,
				UID:        claim.UID,
			}),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		))
	})

	It("should not claim a server in a non-available state", func(ctx SpecContext) {
		By("Patching the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateInitial
		})).Should(Succeed())

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				Image:     "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claim)

		By("Ensuring that the Server has no claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Status.State", metalv1alpha1.ServerStateInitial),
		))

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseUnbound),
		))

		By("Ensuring that the ServerBootConfiguration has not been created")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(Get(config)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should not claim a server with set claim ref", func(ctx SpecContext) {
		By("Patching the Server to available state")
		Eventually(Update(server, func() {
			server.Spec.ServerClaimRef = &v1.ObjectReference{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "ServerClaim",
				Namespace:  ns.Name,
				Name:       "foo",
				UID:        "12345",
			}
		})).Should(Succeed())

		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				Image:     "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claim)

		By("Ensuring that the Server has no claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", &v1.ObjectReference{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "ServerClaim",
				Namespace:  ns.Name,
				Name:       "foo",
				UID:        "12345",
			}),
			HaveField("Status.State", metalv1alpha1.ServerStateReserved),
		))

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseUnbound),
		))

		By("Ensuring that the ServerBootConfiguration has not been created")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(Get(config)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should not claim a server when labels do not match selector", func(ctx SpecContext) {
		By("Creating a ServerClaim")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:     metalv1alpha1.PowerOn,
				ServerRef: &v1.LocalObjectReference{Name: server.Name},
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": "storage"},
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      "env",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"test", "staging"},
					}},
				},
				Image: "foo:bar",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claim)

		By("Patching the Server to available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Ensuring that the Server has no claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
		))

		By("Ensuring that the ServerClaim is unbound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseUnbound),
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
})

var _ = Describe("ServerClaim Validation", func() {
	ns := SetupTest()

	var claim *metalv1alpha1.ServerClaim
	var claimWithSelector *metalv1alpha1.ServerClaim

	BeforeEach(func(ctx SpecContext) {
		By("creating a new ServerClaim")
		claim = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claim)

		By("updating the ServerRef to claim a Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("creating a new ServerClaim")
		claimWithSelector = &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "claim-",
			},
		}
		Expect(k8sClient.Create(ctx, claimWithSelector)).To(Succeed())
		DeferCleanup(k8sClient.Delete, claimWithSelector)

		By("updating the ServerSelector to claim a Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())
	})

	It("Should deny if the ServerRef changes", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "bar"}
		})).Should(HaveOccurred())

		By("ensuring that the ServerRef did not change")
		Consistently(Object(claim)).Should(HaveField("Spec.ServerRef.Name", Equal("foo")))
	})

	It("Should allow a change of ServerClaim by not changing the ServerRef", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.Power = metalv1alpha1.PowerOn
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "foo"}
		})).Should(Succeed())

		By("ensuring that the PowerState changed")
		Consistently(Object(claim)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerRef.Name", Equal("foo")),
		))
	})

	It("Should deny if the ServerSelector changes", func() {
		By("updating the ServerRef to claim a different Server")
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

	It("Should allow a change of ServerClaim by not changing the ServerSelector", func() {
		By("updating the ServerRef to claim a different Server")
		Eventually(Update(claimWithSelector, func() {
			claimWithSelector.Spec.Power = metalv1alpha1.PowerOn
			claimWithSelector.Spec.ServerSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			}
		})).Should(Succeed())

		By("ensuring that the PowerState changed")
		Consistently(Object(claimWithSelector)).Should(SatisfyAll(
			HaveField("Spec.Power", Equal(metalv1alpha1.PowerOn)),
			HaveField("Spec.ServerSelector.MatchLabels", Equal(map[string]string{"foo": "bar"}))))
	})
})

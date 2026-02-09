// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		EnsureCleanState()
	})

	It("Should successfully claim a server in available state", func(ctx SpecContext) {
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
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseBound),
			HaveField("Spec.ServerRef", Not(BeNil())),
			HaveField("Spec.ServerRef.Name", server.Name),
		))

		By("Ensuring that the ServerBootConfiguration has been created with default boot settings")
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
			HaveField("Spec.BootMethod", metalv1alpha1.BootMethodPXE),
			HaveField("Spec.BootMode", metalv1alpha1.BootModeOnce),
		))

		By("Ensuring that the server has a correct boot configuration ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.BootConfigurationRef", &metalv1alpha1.ObjectReference{
				APIVersion: "metal.ironcore.dev/v1alpha1",
				Kind:       "ServerBootConfiguration",
				Namespace:  ns.Name,
				Name:       config.Name,
				UID:        config.UID,
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
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
	})

	It("Should propagate BootMethod and BootMode from claim to boot configuration", func(ctx SpecContext) {
		By("Creating a ServerClaim with HTTPBoot and Continuous mode")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power:      metalv1alpha1.PowerOn,
				ServerRef:  &v1.LocalObjectReference{Name: server.Name},
				Image:      "foo:bar",
				BootMethod: metalv1alpha1.BootMethodHTTPBoot,
				BootMode:   metalv1alpha1.BootModeContinuous,
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is bound")
		Eventually(Object(claim)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerClaimFinalizer)),
			HaveField("Status.Phase", metalv1alpha1.PhaseBound),
		))

		By("Ensuring that the ServerBootConfiguration has the correct boot settings")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      claim.Name,
			},
		}
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Spec.ServerRef.Name", server.Name),
			HaveField("Spec.Image", "foo:bar"),
			HaveField("Spec.BootMethod", metalv1alpha1.BootMethodHTTPBoot),
			HaveField("Spec.BootMode", metalv1alpha1.BootModeContinuous),
		))

		By("Deleting the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

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

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
		))
	})

	It("Should successfully claim a server by label selector", func(ctx SpecContext) {
		By("Patching Server labels")
		Eventually(Update(server, func() {
			server.Labels = map[string]string{
				"type": "storage",
				"env":  "prod",
			}
		})).Should(Succeed())

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
			HaveField("Spec.ServerClaimRef", &metalv1alpha1.ObjectReference{
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

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server is available")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", BeNil()),
			HaveField("Spec.BootConfigurationRef", BeNil()),
			HaveField("Spec.Power", metalv1alpha1.PowerOff),
			HaveField("Status.State", metalv1alpha1.ServerStateAvailable),
			HaveField("Status.PowerState", metalv1alpha1.ServerOffPowerState),
		))
	})

	It("Should not claim a server in a non-available state", func(ctx SpecContext) {
		By("Patching the Server to Initial state")
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

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should not claim a server with set claim ref", func(ctx SpecContext) {
		By("Patching the Server to available state")
		Eventually(Update(server, func() {
			server.Spec.ServerClaimRef = &metalv1alpha1.ObjectReference{
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

		By("Ensuring that the Server has no claim ref")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerClaimRef", &metalv1alpha1.ObjectReference{
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

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should not claim a server when labels do not match selector", func(ctx SpecContext) {
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

		By("Removing the ServerClaim")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

		By("Ensuring that the ServerClaim is deleted")
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should allow deletion of ServerClaim without a Server", func(ctx SpecContext) {
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
		EnsureCleanState()
	})

	It("Should deny if the ServerRef changes", func() {
		By("Updating the ServerRef to claim a different Server")
		Eventually(Update(claim, func() {
			claim.Spec.ServerRef = &v1.LocalObjectReference{Name: "bar"}
		})).Should(HaveOccurred())

		By("Ensuring that the ServerRef did not change")
		Consistently(Object(claim)).Should(HaveField("Spec.ServerRef.Name", Equal("foo")))
	})

	It("Should allow a change of ServerClaim by not changing the ServerRef", func() {
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

	It("Should deny if the ServerSelector changes", func() {
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

	It("Should allow a change of ServerClaim by not changing the ServerSelector", func() {
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

var _ = Describe("Server Claiming", MustPassRepeatedly(5), func() {
	ns := SetupTest(nil)

	makeServer := func(ctx context.Context) {
		server := metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
				Annotations: map[string]string{
					metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationIgnore,
				},
				Labels: map[string]string{"foo": "bar"},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "38947555-7742-3448-3784-823347823834",
			},
		}
		ExpectWithOffset(1, k8sClient.Create(ctx, &server)).To(Succeed())
		server.Status.State = metalv1alpha1.ServerStateAvailable
		server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		ExpectWithOffset(1, k8sClient.Status().Update(ctx, &server)).To(Succeed())
	}

	makeClaim := func(ctx context.Context, labelSelector *metav1.LabelSelector) {
		claim := metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "claim-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				ServerSelector: labelSelector,
			},
		}
		ExpectWithOffset(1, k8sClient.Create(ctx, &claim)).To(Succeed())
	}

	countUniqueBoundServers := func(ctx context.Context, serverCount int) func(Gomega) int {
		return func(g Gomega) int {
			var serverList metalv1alpha1.ServerList
			g.Expect(k8sClient.List(ctx, &serverList)).To(Succeed())
			g.Expect(serverList.Items).To(HaveLen(serverCount))
			claimNames := make(map[string]struct{})
			for _, server := range serverList.Items {
				if server.Spec.ServerClaimRef != nil {
					claimNames[server.Spec.ServerClaimRef.Name] = struct{}{}
				}
			}
			return len(claimNames)
		}
	}

	countUniqueBoundClaims := func(ctx context.Context) func(Gomega) int {
		return func(g Gomega) int {
			var claimList metalv1alpha1.ServerClaimList
			g.Expect(k8sClient.List(ctx, &claimList)).To(Succeed())
			serverNames := make(map[string]struct{})
			for _, claim := range claimList.Items {
				if claim.Spec.ServerRef != nil {
					serverNames[claim.Spec.ServerRef.Name] = struct{}{}
				}
			}
			return len(serverNames)
		}
	}

	AfterEach(func(ctx SpecContext) {
		claimList := &metalv1alpha1.ServerClaimList{}
		Eventually(List(claimList, client.InNamespace(ns.Name))).Should(Succeed())
		for _, claim := range claimList.Items {
			Expect(k8sClient.Delete(ctx, &claim)).To(Succeed())
		}

		serverList := &metalv1alpha1.ServerList{}
		Eventually(List(serverList, client.MatchingLabels{
			"foo": "bar",
		})).Should(Succeed())
		for _, server := range serverList.Items {
			Expect(k8sClient.Delete(ctx, &server)).To(Succeed())
		}
		EnsureCleanState()
	})

	It("Binds four out of ten server for four best effort claims", func(ctx SpecContext) {
		for range 10 {
			makeServer(ctx)
		}
		for range 4 {
			makeClaim(ctx, nil)
		}
		Eventually(countUniqueBoundServers(ctx, 10)).Should(Equal(4))
		Consistently(countUniqueBoundServers(ctx, 10)).Should(Equal(4))
		Eventually(countUniqueBoundClaims(ctx)).Should(Equal(4))
		Consistently(countUniqueBoundClaims(ctx)).Should(Equal(4))
	})

	It("Binds four out of ten server for four label selector claims", func(ctx SpecContext) {
		for range 10 {
			makeServer(ctx)
		}
		for range 4 {
			makeClaim(ctx, metav1.SetAsLabelSelector(labels.Set{"foo": "bar"}))
		}
		Eventually(countUniqueBoundServers(ctx, 10)).Should(Equal(4))
		Consistently(countUniqueBoundServers(ctx, 10)).Should(Equal(4))
		Eventually(countUniqueBoundClaims(ctx)).Should(Equal(4))
		Consistently(countUniqueBoundClaims(ctx)).Should(Equal(4))
	})

	It("Should not bind the same server to multiple best effort claims", func(ctx SpecContext) {
		By("Creating eight ServerClaims")
		for range 8 {
			makeClaim(ctx, nil)
		}
		makeServer(ctx)
		Eventually(countUniqueBoundServers(ctx, 1)).Should(Equal(1))
		Consistently(countUniqueBoundServers(ctx, 1)).Should(Equal(1))
		Eventually(countUniqueBoundClaims(ctx)).Should(Equal(1))
		Consistently(countUniqueBoundClaims(ctx)).Should(Equal(1))
	})

	It("Should not bind the same server to multiple label selector claims", func(ctx SpecContext) {
		By("Creating eight ServerClaims")
		for range 8 {
			makeClaim(ctx, metav1.SetAsLabelSelector(labels.Set{"foo": "bar"}))
		}
		makeServer(ctx)
		Eventually(countUniqueBoundServers(ctx, 1)).Should(Equal(1))
		Consistently(countUniqueBoundServers(ctx, 1)).Should(Equal(1))
		Eventually(countUniqueBoundClaims(ctx)).Should(Equal(1))
		Consistently(countUniqueBoundClaims(ctx)).Should(Equal(1))
	})
})

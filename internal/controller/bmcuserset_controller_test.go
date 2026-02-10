// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("BMCUserSet Controller", func() {
	_ = SetupTest(nil)

	var (
		bmc1     *metalv1alpha1.BMC
		bmc2     *metalv1alpha1.BMC
		bmcOther *metalv1alpha1.BMC
	)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should create BMCUsers for matching BMCs and update status", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		selectorLabels := map[string]string{
			"role": "admin",
		}

		By("Creating BMCs that match the selector")
		bmc1 = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-bmc-1",
				Labels: selectorLabels,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: corev1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc1)).To(Succeed())

		bmc2 = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-bmc-2",
				Labels: selectorLabels,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EB",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: corev1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc2)).To(Succeed())

		By("Creating a non-matching BMC")
		bmcOther = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc-other",
				Labels: map[string]string{
					"role": "reader",
				},
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EC",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: corev1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcOther)).To(Succeed())

		By("Creating a BMCUserSet")
		bmcUserSet := &metalv1alpha1.BMCUserSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmcuserset-",
			},
			Spec: metalv1alpha1.BMCUserSetSpec{
				BMCSelector: metav1.LabelSelector{
					MatchLabels: selectorLabels,
				},
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName:    "metal-operator",
					RoleID:      "Administrator",
					Description: "managed by bmcuserset",
					BMCSecretRef: &corev1.LocalObjectReference{
						Name: bmcSecret.Name,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcUserSet)).To(Succeed())

		By("Ensuring BMCUsers are created for matching BMCs")
		Eventually(func() []metalv1alpha1.BMCUser {
			list := &metalv1alpha1.BMCUserList{}
			Expect(k8sClient.List(ctx, list)).To(Succeed())
			return list.Items
		}).Should(ConsistOf(
			SatisfyAll(
				HaveField("Spec.BMCRef.Name", bmc1.Name),
				HaveField("Spec.UserName", "metal-operator"),
				HaveField("Spec.RoleID", "Administrator"),
				HaveField("Spec.Description", "managed by bmcuserset"),
				HaveField("Spec.BMCSecretRef.Name", bmcSecret.Name),
			),
			SatisfyAll(
				HaveField("Spec.BMCRef.Name", bmc2.Name),
				HaveField("Spec.UserName", "metal-operator"),
				HaveField("Spec.RoleID", "Administrator"),
				HaveField("Spec.Description", "managed by bmcuserset"),
				HaveField("Spec.BMCSecretRef.Name", bmcSecret.Name),
			),
		))

		By("Ensuring BMCUserSet status is updated")
		Eventually(Object(bmcUserSet)).Should(SatisfyAll(
			HaveField("Status.FullyLabeledBMCs", int32(2)),
			HaveField("Status.AvailableBMCUsers", int32(2)),
		))

		Expect(k8sClient.Delete(ctx, bmcUserSet)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmc1)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmc2)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcOther)).To(Succeed())

		server1 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc1),
			},
		}
		server2 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc2),
			},
		}
		server3 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmcOther),
			},
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server1))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server2))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server3))).To(Succeed())
		Eventually(ObjectList(&metalv1alpha1.ServerList{})).Should(HaveField("Items", HaveLen(0)))

		bmcUserList := &metalv1alpha1.BMCUserList{}
		Expect(k8sClient.List(ctx, bmcUserList)).To(Succeed())
		for i := range bmcUserList.Items {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &bmcUserList.Items[i]))).To(Succeed())
		}
		Eventually(ObjectList(&metalv1alpha1.BMCUserList{})).Should(HaveField("Items", HaveLen(0)))

		secretList := &metalv1alpha1.BMCSecretList{}
		Expect(k8sClient.List(ctx, secretList)).To(Succeed())
		for i := range secretList.Items {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &secretList.Items[i]))).To(Succeed())
		}

	})

	It("Should update existing BMCUsers when template changes", func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a BMC")
		bmc1 = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc-update",
				Labels: map[string]string{
					"role": "admin",
				},
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:ED",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: corev1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc1)).To(Succeed())

		By("Creating a BMCUserSet")
		bmcUserSet := &metalv1alpha1.BMCUserSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmcuserset-",
			},
			Spec: metalv1alpha1.BMCUserSetSpec{
				BMCSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"role": "admin"},
				},
				BMCUserTemplate: metalv1alpha1.BMCUserTemplate{
					UserName:    "metal-operator",
					RoleID:      "Administrator",
					Description: "managed by bmcuserset",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcUserSet)).To(Succeed())

		By("Ensuring BMCUser is created")
		bmcUserList := &metalv1alpha1.BMCUserList{}
		Eventually(ObjectList(bmcUserList)).Should(HaveField("Items", HaveLen(1)))

		By("Updating the BMCUserSet template")
		Eventually(Update(bmcUserSet, func() {
			bmcUserSet.Spec.BMCUserTemplate.Description = "updated description"
		})).Should(Succeed())

		By("Ensuring the BMCUser is patched from the updated template")
		Eventually(ObjectList(bmcUserList)).Should(SatisfyAll(
			HaveField("Items", HaveLen(1)),
			HaveField("Items", ContainElement(
				HaveField("Spec.Description", "updated description"),
			)),
		))
		Expect(k8sClient.Delete(ctx, bmcUserSet)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmc1)).To(Succeed())

		server1 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc1),
			},
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server1))).To(Succeed())
		Eventually(ObjectList(&metalv1alpha1.ServerList{})).Should(HaveField("Items", HaveLen(0)))

		secretList := &metalv1alpha1.BMCSecretList{}
		Expect(k8sClient.List(ctx, secretList)).To(Succeed())
		for i := range secretList.Items {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &secretList.Items[i]))).To(Succeed())
		}
	})
})

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"maps"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metalBmc "github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMC Controller", func() {
	_ = SetupTest()

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should successfully reconcile the a BMC resource", func(ctx SpecContext) {
		By("Creating an Endpoints object")
		endpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())

		By("Ensuring that the BMC resource has been created for an endpoint")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: endpoint.Name,
			},
		}
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "Endpoint",
				Name:               endpoint.Name,
				UID:                endpoint.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Status.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
		))

		By("Ensuring that the Server resource will be created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.BMCRef.Name", endpoint.Name),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, endpoint)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmc.Name,
			},
		}
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should successfully reconcile the a BMC resource with inline access information", func(ctx SpecContext) {
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

		bmcLabels := map[string]string{
			"foo": "bar",
		}

		By("Creating a BMC resource")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Labels:       bmcLabels,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
		))

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("ObjectMeta.Labels", bmcLabels),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.BMCRef.Name", bmc.Name),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should successfully reconcile the a BMC resource and patch on BMC label changes", func(ctx SpecContext) {
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

		bmcLabels := map[string]string{
			"foo": "bar",
		}

		By("Creating a BMC resource")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Labels:       bmcLabels,
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
		))

		By("Ensuring that the Server resource has been created")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("ObjectMeta.Labels", bmcLabels),
			HaveField("Spec.UUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemUUID", "38947555-7742-3448-3784-823347823834"),
			HaveField("Spec.SystemURI", "/redfish/v1/Systems/437XR1138R2"),
			HaveField("Spec.BMCRef.Name", bmc.Name),
		))

		By("Updating BMC labels")
		bmcLabels["foo"] = "baz"

		Eventually(Update(bmc, func() {
			maps.Copy(bmc.Labels, bmcLabels)
		})).Should(Succeed())

		By("Ensuring that the Server resource has been updated")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("ObjectMeta.Labels", bmcLabels),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
	})
})

var _ = Describe("BMC Validation", func() {
	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should deny if the BMC has EndpointRef and InlineEndpoint spec fields", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-invalid",
			},
			Spec: metalv1alpha1.BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should deny if the BMC has no EndpointRef and InlineEndpoint spec fields", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-empty",
			},
			Spec: metalv1alpha1.BMCSpec{},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should admit if the BMC has an EndpointRef but no InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should deny if the BMC EndpointRef spec field has been removed", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(HaveField(
			"Spec.EndpointRef", &v1.LocalObjectReference{Name: "foo"})))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should admit if the BMC is changing EndpointRef to InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = nil
			bmc.Spec.Endpoint = &metalv1alpha1.InlineEndpoint{
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
				MACAddress: "aa:bb:cc:dd:ee:ff",
			}
		})).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should admit if the BMC has no EndpointRef but an InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should deny if the BMC InlineEndpoint spec field has been removed", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Update(bmc, func() {
			bmc.Spec.Endpoint = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.Endpoint.IP", metalv1alpha1.MustParseIP("127.0.0.1")),
			HaveField("Spec.Endpoint.MACAddress", "aa:bb:cc:dd:ee:ff"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("Should admit if the BMC has is changing to an EndpointRef from an InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Update(bmc, func() {
			bmc.Spec.EndpointRef = &v1.LocalObjectReference{Name: "foo"}
			bmc.Spec.Endpoint = nil
		})).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})
})

var _ = Describe("BMC Reset", func() {
	_ = SetupTest()

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should reset the BMC", func(ctx SpecContext) {
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

		By("Creating a BMC resource")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		By("Resetting the BMC")
		Eventually(Update(bmc, func() {
			bmc.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.GracefulRestartBMC,
			}
		})).Should(Succeed())

		By("Ensuring that the reset annotation has been removed")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))),
		))
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Get(server)).Should(Succeed())
		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
	})
})

var _ = Describe("BMC Conditions", func() {
	_ = SetupTest()

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should create ready conditions when there are bmc connection errors", func(ctx SpecContext) {
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = true
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

		By("Creating a BMC resource")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-reset-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		By("Ensuring right conditions are present")
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", bmcReadyConditionType),
					HaveField("Status", metav1.ConditionFalse),
				),
			)),
		)

		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = false

		By("Ensuring right conditions are present, after bmc becomes responsive again")
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", HaveLen(1)),
		)
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", bmcReadyConditionType),
					HaveField("Status", metav1.ConditionTrue),
				),
			)),
		)

		By("resetting the BMC")
		Eventually(Update(bmc, func() {
			bmc.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.GracefulRestartBMC,
			}
		},
		)).Should(Succeed())

		By("Ensuring right conditions are present, after user requested reset")
		Eventually(Object(bmc)).WithPolling(1 * time.Microsecond).MustPassRepeatedly(1).Should(SatisfyAll(
			HaveField("Status.Conditions", HaveLen(2)),
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", bmcResetConditionType),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", bmcUserResetReason),
				),
			)),
		))
		By("BMC reset should remove the reset annotation")
		Eventually(Object(bmc)).Should(
			HaveField("Annotations", BeNil()),
		)
		By("Ensuring right conditions are present, after bmc reset is done")
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", bmcResetConditionType),
					HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", "ResetComplete"),
				),
			)),
		)
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(server)).Should(Satisfy(apierrors.IsNotFound))
	})
})

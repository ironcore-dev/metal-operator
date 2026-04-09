// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"maps"
	"sync"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("BMC Controller", func() {
	ns := SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("should successfully reconcile a BMC resource", func(ctx SpecContext) {
		By("Creating an Endpoints object")
		endpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.EndpointSpec{
				// emulator BMC mac address
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP(MockServerIP),
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
			HaveField("Status.IP", metalv1alpha1.MustParseIP(MockServerIP)),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
			HaveField("Status.MetricsReportSubscriptionLink", Equal("/redfish/v1/EventService/Subscriptions/5")),
			HaveField("Status.EventsSubscriptionLink", Equal("/redfish/v1/EventService/Subscriptions/6")),
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

	It("should successfully reconcile a BMC resource with inline access information", func(ctx SpecContext) {
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
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.IP", metalv1alpha1.MustParseIP(MockServerIP)),
			HaveField("Status.MACAddress", "23:11:8A:33:CF:EA"),
			HaveField("Status.Model", "Joo Janta 200"),
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
			HaveField("Status.PowerState", metalv1alpha1.OnPowerState),
			HaveField("Status.FirmwareVersion", "1.45.455b66-rev4"),
			HaveField("Status.MetricsReportSubscriptionLink", Equal("/redfish/v1/EventService/Subscriptions/5")),
			HaveField("Status.EventsSubscriptionLink", Equal("/redfish/v1/EventService/Subscriptions/6")),
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

	It("should successfully reconcile a BMC resource and patch on BMC label changes", func(ctx SpecContext) {
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
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "23:11:8A:33:CF:EA",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.IP", metalv1alpha1.MustParseIP(MockServerIP)),
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

	It("should create a DNS record for the BMC if configured", func(ctx SpecContext) {
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
				Hostname: ptr.To("node001r-bb001.region.cloud.com"),
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		By("Ensuring that the DNSRecord resource has been created for the bmc")
		dnsRecord := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bmc.Name,
				Namespace: ns.Name,
			},
		}
		Eventually(Object(dnsRecord)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "BMC",
				Name:               bmc.Name,
				UID:                bmc.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Data", HaveKeyWithValue("hostname", "node001r-bb001.region.cloud.com")),
			HaveField("Data", HaveKeyWithValue("ip", "127.0.0.1")),
			HaveField("Data", HaveKeyWithValue("recordType", "A")),
			HaveField("Data", HaveKeyWithValue("ttl", "300")),
		))
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		Expect(k8sClient.Delete(ctx, dnsRecord)).To(Succeed())
	})

})

var _ = Describe("BMC Validation", func() {
	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("should deny if the BMC has EndpointRef and InlineEndpoint spec fields", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-invalid",
			},
			Spec: metalv1alpha1.BMCSpec{
				EndpointRef: &v1.LocalObjectReference{Name: "foo"},
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should deny if the BMC has no EndpointRef and InlineEndpoint spec fields", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-empty",
			},
			Spec: metalv1alpha1.BMCSpec{},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(HaveOccurred())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should admit if the BMC has an EndpointRef but no InlineEndpoint spec field", func(ctx SpecContext) {
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

	It("should deny if the BMC EndpointRef spec field has been removed", func(ctx SpecContext) {
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

	It("should admit if the BMC is changing EndpointRef to InlineEndpoint spec field", func(ctx SpecContext) {
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
				IP:         metalv1alpha1.MustParseIP(MockServerIP),
				MACAddress: "aa:bb:cc:dd:ee:ff",
			}
		})).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should admit if the BMC has no EndpointRef but an InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should deny if the BMC InlineEndpoint spec field has been removed", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		Eventually(Update(bmc, func() {
			bmc.Spec.Endpoint = nil
		})).Should(Not(Succeed()))

		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.Endpoint.IP", metalv1alpha1.MustParseIP(MockServerIP)),
			HaveField("Spec.Endpoint.MACAddress", "aa:bb:cc:dd:ee:ff"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should admit if the BMC is changing to an EndpointRef from an InlineEndpoint spec field", func(ctx SpecContext) {
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
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
	_ = SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("should reset the BMC", func(ctx SpecContext) {
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
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
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
				metalv1alpha1.OperationAnnotation: metalv1alpha1.ForceResetBMC,
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
	_ = SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("should create ready conditions when there are BMC connection errors", func(ctx SpecContext) {
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
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:ff",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

		By("Ensuring right conditions are present")
		Eventually(Object(bmc)).Should(
			HaveField("Status.Conditions", Not(BeEmpty())),
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
			HaveField("Status.Conditions", Not(BeEmpty())),
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
				metalv1alpha1.OperationAnnotation: metalv1alpha1.ForceResetBMC,
			}
		},
		)).Should(Succeed())

		By("Ensuring right conditions are present, after user requested reset")
		Eventually(Object(bmc)).WithPolling(1 * time.Microsecond).MustPassRepeatedly(1).Should(
			HaveField("Status.Conditions", ContainElement(
				SatisfyAll(
					HaveField("Type", bmcResetConditionType),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", bmcUserResetReason),
				),
			)),
		)
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

// Helper to create BMCSecret for SSH tests
func createBMCSecretForSSHTest(ctx context.Context) *metalv1alpha1.BMCSecret {
	bmcSecret := &metalv1alpha1.BMCSecret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-ssh-",
		},
		Data: map[string][]byte{
			metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
			metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
		},
	}
	Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())
	return bmcSecret
}

// Helper to create BMC for SSH reset tests
func createBMCForSSHTest(ctx context.Context, bmcSecret *metalv1alpha1.BMCSecret) *metalv1alpha1.BMC {
	bmc := &metalv1alpha1.BMC{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-bmc-ssh-",
		},
		Spec: metalv1alpha1.BMCSpec{
			Endpoint: &metalv1alpha1.InlineEndpoint{
				IP:         metalv1alpha1.MustParseIP(MockServerIP),
				MACAddress: "aa:bb:cc:dd:ee:ff",
			},
			Protocol: metalv1alpha1.Protocol{
				Name: metalv1alpha1.ProtocolRedfishLocal,
				Port: MockServerPort,
			},
			BMCSecretRef: v1.LocalObjectReference{
				Name: bmcSecret.Name,
			},
		},
	}
	Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
	return bmc
}

var _ = Describe("BMC SSH Reset", func() {
	_ = SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		// Clean up Servers with finalizers first
		serverList := &metalv1alpha1.ServerList{}
		if err := k8sClient.List(ctx, serverList); err == nil {
			for i := range serverList.Items {
				server := &serverList.Items[i]
				if controllerutil.ContainsFinalizer(server, "metal.ironcore.dev/server") {
					// Remove finalizer to allow deletion
					controllerutil.RemoveFinalizer(server, "metal.ironcore.dev/server")
					Expect(k8sClient.Update(ctx, server)).To(Succeed())
				}
				// Explicitly delete the server (ignore errors - server may already be deleted via owner references)
				_ = k8sClient.Delete(ctx, server)
			}
		}
		EnsureCleanState()
		// Restore real SSH function
		bmcutils.SSHResetBMCFunc = bmcutils.SSHResetBMC
		// Reset mock flag
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = false
	})

	It("Should successfully perform SSH reset after Redfish 503 error", func(ctx SpecContext) {
		// Setup mock SSH function with proper synchronization
		var mu sync.Mutex
		sshResetCalled := false
		var capturedIP, capturedManufacturer, capturedUsername, capturedPassword string
		var capturedTimeout time.Duration

		bmcutils.SSHResetBMCFunc = func(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
			mu.Lock()
			defer mu.Unlock()
			sshResetCalled = true
			capturedIP = ip
			capturedManufacturer = manufacturer
			capturedUsername = username
			capturedPassword = password
			capturedTimeout = timeout
			return nil
		}

		// Create BMC with Redfish available to get manufacturer info
		bmcSecret := createBMCSecretForSSHTest(ctx)
		bmc := createBMCForSSHTest(ctx, bmcSecret)

		// Wait for BMC to become Ready
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Ready"),
			HaveField("Status", metav1.ConditionTrue),
		))))

		// Wait for manufacturer to be populated
		Eventually(Object(bmc)).WithTimeout(10 * time.Second).Should(HaveField("Status.Manufacturer", Not(BeEmpty())))

		// Simulate Redfish failure and add annotation
		// The controller will try to get BMC client (fails with 503), detect reset annotation,
		// and enqueue SSH reset
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = true

		Eventually(Update(bmc, func() {
			if bmc.Annotations == nil {
				bmc.Annotations = map[string]string{}
			}
			bmc.Annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.ForceResetBMC
		})).Should(Succeed())

		// Wait for SSH to be called
		Eventually(func() bool {
			mu.Lock()
			defer mu.Unlock()
			return sshResetCalled
		}).WithTimeout(15 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())

		// Verify SSH was called with correct parameters
		mu.Lock()
		Expect(capturedIP).To(Equal(MockServerIP))
		Expect(capturedManufacturer).NotTo(BeEmpty())
		Expect(capturedUsername).To(Equal("foo"))
		Expect(capturedPassword).To(Equal("bar"))
		Expect(capturedTimeout).To(Equal(1 * time.Second))
		mu.Unlock()

		// Verify Reset condition was created
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Reset"),
			HaveField("Status", metav1.ConditionTrue),
		))))

		// Verify LastResetTime was updated
		Eventually(Object(bmc)).Should(HaveField("Status.LastResetTime", Not(BeNil())))

		// Verify annotation was removed
		Eventually(Object(bmc)).Should(HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))))

		// Cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should handle SSH reset connection failure gracefully", func(ctx SpecContext) {
		// Setup mock SSH function to return error
		bmcutils.SSHResetBMCFunc = func(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
			return fmt.Errorf("connection refused")
		}

		// Create BMC and let it get manufacturer info first
		bmcSecret := createBMCSecretForSSHTest(ctx)
		bmc := createBMCForSSHTest(ctx, bmcSecret)

		// Wait for BMC to become Ready
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Ready"),
			HaveField("Status", metav1.ConditionTrue),
		))))

		// Wait for manufacturer to be populated
		Eventually(Object(bmc)).WithTimeout(10 * time.Second).Should(HaveField("Status.Manufacturer", Not(BeEmpty())))

		// Now simulate Redfish unavailability and trigger reset via annotation
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = true

		Eventually(Update(bmc, func() {
			if bmc.Annotations == nil {
				bmc.Annotations = map[string]string{}
			}
			bmc.Annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.ForceResetBMC
		})).Should(Succeed())

		// check for status state pending
		Eventually(Object(bmc)).Should(HaveField("Status.State", metalv1alpha1.BMCStatePending))

		// Wait for SSH reset to be attempted and fail
		Eventually(Object(bmc), 2*time.Second).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Reset"),
			HaveField("Status", metav1.ConditionFalse),
			HaveField("Reason", "InternalServerError"),
			HaveField("Message", ContainSubstring("SSH reset failed")),
		))))

		// Cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should not trigger duplicate SSH resets during wait period", func(ctx SpecContext) {
		// Setup mock SSH function with call counter and proper synchronization
		var mu sync.Mutex
		callCount := 0
		bmcutils.SSHResetBMCFunc = func(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			return nil
		}

		// Create BMC and let it get manufacturer info first
		bmcSecret := createBMCSecretForSSHTest(ctx)
		bmc := createBMCForSSHTest(ctx, bmcSecret)

		// Wait for BMC to become Ready
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.Conditions", ContainElement(SatisfyAll(
				HaveField("Type", "Ready"),
				HaveField("Status", metav1.ConditionTrue),
			))),
			HaveField("Status.Manufacturer", Not(BeEmpty())),
		))

		// Now simulate Redfish unavailability and trigger reset via annotation
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = true

		Eventually(Update(bmc, func() {
			if bmc.Annotations == nil {
				bmc.Annotations = map[string]string{}
			}
			bmc.Annotations[metalv1alpha1.OperationAnnotation] = metalv1alpha1.ForceResetBMC
		})).Should(Succeed())

		// Wait for first SSH reset
		Eventually(func() int {
			mu.Lock()
			defer mu.Unlock()
			return callCount
		}).Should(Equal(1))

		// Wait through the reset wait period
		time.Sleep(500 * time.Millisecond)

		// verify reset annotation was removed and not re-added
		Eventually(Object(bmc)).Should(HaveField("Annotations", Not(HaveKey(metalv1alpha1.OperationAnnotation))))

		// Verify SSH was only called once (idempotency guard works)
		mu.Lock()
		Expect(callCount).To(Equal(1))
		mu.Unlock()

		// Verify Reset condition prevents re-triggering
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Reset"),
			HaveField("Status", metav1.ConditionTrue),
		))))

		// Cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should not trigger SSH reset for authentication errors", func(ctx SpecContext) {
		// Setup mock SSH function with call tracker and proper synchronization
		var mu sync.Mutex
		sshCalled := false
		bmcutils.SSHResetBMCFunc = func(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
			mu.Lock()
			defer mu.Unlock()
			sshCalled = true
			return nil
		}

		// Create BMC with invalid credentials (will get 401)
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ssh-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("invalid"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("invalid"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		bmc := createBMCForSSHTest(ctx, bmcSecret)

		// Wait for authentication failure
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Ready"),
			HaveField("Status", metav1.ConditionFalse),
			HaveField("Reason", "AuthenticationFailed"),
		))))

		// Wait a bit to ensure SSH is not triggered
		Consistently(func() bool {
			mu.Lock()
			defer mu.Unlock()
			return sshCalled
		}).WithTimeout(2 * time.Second).Should(BeFalse())

		// Verify no Reset condition was created
		Consistently(Object(bmc)).Should(HaveField("Status.Conditions", Not(ContainElement(HaveField("Type", "Reset")))))

		// Cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should handle BMC deletion during SSH processing", func(ctx SpecContext) {
		// Setup mock SSH function with delay and panic detection with proper synchronization
		var mu sync.Mutex
		panicOccurred := false
		bmcutils.SSHResetBMCFunc = func(ctx context.Context, ip, manufacturer, username, password string, timeout time.Duration) error {
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					defer mu.Unlock()
					panicOccurred = true
				}
			}()
			time.Sleep(100 * time.Millisecond)
			return nil
		}

		// Create BMC and let it get manufacturer info first
		bmcSecret := createBMCSecretForSSHTest(ctx)
		bmc := createBMCForSSHTest(ctx, bmcSecret)

		// Wait for BMC to be created and become Ready
		Eventually(Object(bmc)).Should(HaveField("Status.Conditions", ContainElement(SatisfyAll(
			HaveField("Type", "Ready"),
			HaveField("Status", metav1.ConditionTrue),
		))))

		// Wait for manufacturer to be populated
		Eventually(Object(bmc)).WithTimeout(10 * time.Second).Should(HaveField("Status.Manufacturer", Not(BeEmpty())))

		// Now simulate Redfish unavailability
		metalBmc.UnitTestMockUps.SimulateUnvailableBMC = true

		// Delete BMC while processing
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())

		// Wait to ensure worker continues without panic
		Eventually(Get(bmc)).Should(Satisfy(apierrors.IsNotFound))

		// Verify no panic occurred
		Consistently(func() bool {
			mu.Lock()
			defer mu.Unlock()
			return panicOccurred
		}).WithTimeout(1 * time.Second).Should(BeFalse())
	})
})

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"

	bmcPkg "github.com/ironcore-dev/metal-operator/bmc"
)

var _ = Describe("BMCSettings Controller", func() {
	ns := SetupTest()

	var server *metalv1alpha1.Server
	var bmc *metalv1alpha1.BMC

	BeforeEach(func(ctx SpecContext) {
		By("Creating a BMCSecret")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-secret-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("foo"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("bar"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a BMC resource")
		bmc = &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Namespace:    ns.Name,
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

		By("Ensuring that the Server resource will be created")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: bmcutils.GetServerNameFromBMCandIndex(0, bmc),
			},
		}
		Eventually(Get(server)).Should(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
		bmcPkg.UnitTestMockUps.ResetBMCSettings()
	})

	It("should successfully patch BMCSettings reference to referred BMC", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)

		By("Creating a bmcSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "1.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
	})

	It("should move to completed if no BMCSettings changes to referred BMC", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)

		By("Creating a bmcSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-nochange",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "1.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))
	})

	It("should update the setting if BMCSettings changes requested in Available State", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-bmc-setting"

		By("update the server state to Available state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "1.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		By("Ensuring that the BMCSettings has reached next state")
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))
	})

	It("should create maintenance and wait for its approval before applying settings", func(ctx SpecContext) {

		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-to-req-server-maintenance-through-Ownerapproved"

		// put server in reserved state. and create a bmc setting in Ownerapproved which needs reboot.
		// this is needed to check the states traversed.
		serverClaim := CreateServerClaim(ctx, k8sClient, *server, ns.Name, nil, metalv1alpha1.PowerOff, "foo:bar")
		TransitionServerToReservedState(ctx, k8sClient, serverClaim, server, ns.Name)

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "1.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by BMCSettings resource")
		Eventually(Object(bmcSettings)).Should(
			HaveField("Spec.ServerMaintenanceRefs",
				[]metalv1alpha1.ServerMaintenanceRefItem{{
					ServerMaintenanceRef: &v1.ObjectReference{
						Kind:       "ServerMaintenance",
						Name:       serverMaintenance.Name,
						Namespace:  serverMaintenance.Namespace,
						UID:        serverMaintenance.UID,
						APIVersion: metalv1alpha1.GroupVersion.String(),
					}}}),
		)

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Approving the maintenance")
		Eventually(Update(serverClaim, func() {
			metautils.SetAnnotation(serverClaim, metalv1alpha1.ServerMaintenanceApprovalKey, "true")
		})).Should(Succeed())

		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Deleting the BMCSettings")
		Expect(k8sClient.Delete(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))
	})

	It("should wait for upgrade and reconcile BMCSettings version is correct", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "145"

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		BMCSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "2.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettings)).To(Succeed())

		By("Ensuring that the BMC has the correct BMC settings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", Not(BeNil())),
			HaveField("Spec.BMCSettingRef.Name", BMCSettings.Name),
		))

		By("Ensuring that the BMCSettings resource state is correct State inVersionUpgrade")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Ensuring that the serverMaintenance not ref. while waiting for upgrade")
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Simulate the server BMCSettings version update by matching the spec version")
		Eventually(Update(BMCSettings, func() {
			BMCSettings.Spec.Version = "1.45.455b66-rev4"
		})).Should(Succeed())

		By("Ensuring that the BMCSettings resource has completed Upgrade and setting update, and moved the state")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", HaveLen(1)))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      serverMaintenanceList.Items[0].Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the BMCSettings resource hasmoved to next state")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
		Eventually(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefs", BeNil()),
		))

		By("Deleting the BMCSetting resource")
		Expect(k8sClient.Delete(ctx, BMCSettings)).To(Succeed())

		By("Ensuring that the BMCSettings resource is removed")
		Eventually(Get(BMCSettings)).Should(Satisfy(apierrors.IsNotFound))
		Consistently(Get(BMCSettings)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server BMCSettings ref is empty on BMC")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", BeNil()),
		))
	})

	It("should allow retry using annotation", func(ctx SpecContext) {
		// settings which does not reboot. mocked at
		// metal-operator/bmc/redfish_local.go defaultMockedBMCSetting
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "145"

		By("update the server state to Available  state")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
			server.Status.PowerState = metalv1alpha1.ServerOffPowerState
		})).Should(Succeed())

		By("Creating a BMCSetting")
		BMCSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				Version:                 "1.45.455b66-rev4",
				SettingsMap:             bmcSetting,
				BMCRef:                  &v1.LocalObjectReference{Name: bmc.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, BMCSettings)).To(Succeed())

		By("Moving to Failed state")
		Eventually(UpdateStatus(BMCSettings, func() {
			BMCSettings.Status.State = metalv1alpha1.BMCSettingsStateFailed
		})).Should(Succeed())

		Eventually(Update(BMCSettings, func() {
			BMCSettings.Annotations = map[string]string{
				metalv1alpha1.OperationAnnotation: metalv1alpha1.OperationAnnotationRetry,
			}
		})).Should(Succeed())

		Eventually(Object(BMCSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		)

		Eventually(Object(BMCSettings)).Should(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		)
	})
})

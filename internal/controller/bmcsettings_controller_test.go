// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ironcore-dev/controller-utils/metautils"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("BMCSettings Controller", func() {
	ns := SetupTest()
	ns.Name = "default"

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
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
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

	It("should move to completed if no BMCSettings changes to referred server", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)

		By("Creating a bmcSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-nochange",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
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

	It("should update the setting if BMCSettings changes requested", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-bmc-setting"

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
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

		By("Ensuring that the Maintenance resource has been referenced by BMCSettings resource")
		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
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

	It("should successfully reconcile if BMCRef is provided instead of serverRefList", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "change-with-BMCRef"

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
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

		By("Ensuring that the Maintenance resource has been referenced by BMCSettings resource")
		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
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

	It("should not create maintenance if policy is enforced", func(ctx SpecContext) {
		bmcSetting := make(map[string]string)
		bmcSetting["fooreboot"] = "214"

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{

			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())

		By("Ensuring that the BMC has the BMCSettings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.BMCSettingRef", &v1.LocalObjectReference{Name: bmcSettings.Name}),
		))

		By("Ensuring that the Maintenance resource has not been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Ensuring that the Maintenance resource has not been referenced by BMCSettings resource")
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
		))

		By("Ensuring that the state refects updating settings state")
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has not been created")
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		By("Ensuring that the Maintenance resource has not been referenced by BMCSettings resource")
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
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

	It("should create maintenance if policy is Ownerapproved", func(ctx SpecContext) {

		bmcSetting := make(map[string]string)
		bmcSetting["abc"] = "changed-to-req-server-maintenance-through-Ownerapproved"

		// put server in reserved state. and create a bmc setting in Ownerapproved which needs reboot.
		// this is needed to check the states traversed.
		serverClaim := transitionServerToReserved(ctx, ns, server, metalv1alpha1.PowerOff)

		By("Creating a BMCSetting")
		bmcSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-change",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "1.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, bmcSettings)).To(Succeed())
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      fmt.Sprintf("%s-%s", bmcSettings.Name, server.Name),
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the Maintenance resource has been referenced by BMCSettings resource")
		Eventually(Object(bmcSettings)).Should(SatisfyAny(
			HaveField("Spec.ServerMaintenanceRefMap",
				map[string]*v1.ObjectReference{server.Name: {
					Kind:       "ServerMaintenance",
					Name:       serverMaintenance.Name,
					Namespace:  serverMaintenance.Namespace,
					UID:        serverMaintenance.UID,
					APIVersion: serverMaintenance.GroupVersionKind().GroupVersion().String(),
				}}),
			HaveField("Spec.ServerMaintenanceRefMap",
				map[string]*v1.ObjectReference{server.Name: {
					Kind:       "ServerMaintenance",
					Name:       serverMaintenance.Name,
					Namespace:  serverMaintenance.Namespace,
					UID:        serverMaintenance.UID,
					APIVersion: "metal.ironcore.dev/v1alpha1",
				}}),
		))

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

		Eventually(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the Maintenance resource has been deleted")
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))
		Consistently(Object(bmcSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
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

		By("Creating a BMCSetting")
		BMCSettings := &metalv1alpha1.BMCSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-bmc-upgrade",
			},
			Spec: metalv1alpha1.BMCSettingsSpec{
				BMCSettings:             metalv1alpha1.BMCSettingsMap{Version: "2.45.455b66-rev4", Settings: bmcSetting},
				ServerRefList:           []*v1.LocalObjectReference{{Name: server.Name}},
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

		By("Ensuring that the serverMaintenance not ref.")
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
		))

		By("Simulate the server BMCSettings version update by matching the spec version")
		Eventually(Update(BMCSettings, func() {
			BMCSettings.Spec.BMCSettings.Version = "1.45.455b66-rev4"
		})).Should(Succeed())

		By("Ensuring that the BMCSettings resource has completed Upgrade and setting update, and moved the state")
		Eventually(Object(BMCSettings)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateInProgress),
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))
		Eventually(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCSettingsStateApplied),
		))

		By("Ensuring that the serverMaintenance not ref.")
		Consistently(Object(BMCSettings)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRefMap", BeNil()),
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
})

func transitionServerToReserved(ctx SpecContext, ns *v1.Namespace, server *metalv1alpha1.Server, powerState metalv1alpha1.Power) *metalv1alpha1.ServerClaim {

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
	serverClaim := &metalv1alpha1.ServerClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    ns.Name,
			GenerateName: "test-",
		},
		Spec: metalv1alpha1.ServerClaimSpec{
			Power:             powerState,
			ServerRef:         &v1.LocalObjectReference{Name: server.Name},
			IgnitionSecretRef: &v1.LocalObjectReference{Name: ignitionSecret.Name},
			Image:             "foo:bar",
		},
	}
	Expect(k8sClient.Create(ctx, serverClaim)).To(Succeed())

	By("Patching the Server to available state")
	Eventually(UpdateStatus(server, func() {
		server.Status.State = metalv1alpha1.ServerStateAvailable
	})).Should(Succeed())

	// unfortunately, ServerClaim force creates the bootconfig and that does not transition to completed state.
	// in reserved state, Hence, manually move bootconfig to completed to be able to put server in powerOn state.
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	bootConfig.Name = serverClaim.Name
	bootConfig.Namespace = serverClaim.Namespace

	Eventually(Get(bootConfig)).Should(Succeed())

	By("Patching the bootConfig to ready state")
	Eventually(UpdateStatus(bootConfig, func() {
		bootConfig.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	})).Should(Succeed())

	Eventually(Get(server)).Should(Succeed())

	By("Ensuring that the Server has the spec and state")
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Spec.ServerClaimRef.Name", serverClaim.Name),
		HaveField("Spec.Power", powerState),
		HaveField("Status.State", metalv1alpha1.ServerStateReserved),
	))
	By("Ensuring that the Server has the correct power state")
	Eventually(Object(server)).Should(SatisfyAll(
		HaveField("Status.PowerState", metalv1alpha1.ServerPowerState(powerState)),
	))

	return serverClaim
}

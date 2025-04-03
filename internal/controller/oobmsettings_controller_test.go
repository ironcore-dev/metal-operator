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

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/bmcutils"
)

var _ = Describe("OOBMSettings Controller", func() {
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
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("should successfully patch OoBM reference to referred BMC", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "1.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC has the correct OoBM setting ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))

		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))
	})

	It("should move to completed if no OoBM setting changes to referred server", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-nochange",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "1.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Ensuring that bmc  has the OoBM setting ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))

		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Deleting the OoBM")
		Expect(k8sClient.Delete(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC OoBM ref is empty")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", BeNil()),
		))
	})

	It("should update the setting if OoBM setting changes requested", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)
		OoBMSetting["abc"] = "blahblah"

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-change",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "1.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Ensuring that the Server has the OoBM setting ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))
		By("Ensuring that the OoBM setting has reached next state")
		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInSettingUpdate),
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Ensuring that the Maintenance resource has been referenced by OoBM resource")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Deleting the OoBM")
		Expect(k8sClient.Delete(ctx, OoBM)).To(Succeed())

		By("Ensuring that the Server OoBM ref is empty")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", BeNil()),
		))
	})

	It("should not create maintenance if policy is enforced", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)
		OoBMSetting["fooreboot"] = "214"

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{

			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-change",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "1.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyEnforced,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Ensuring that the Server has the OoBM setting ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))

		By("Ensuring that the state refects updating settings state")
		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInSettingUpdate),
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Ensuring that the Maintenance resource has not been referenced by OoBM resource")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", BeEmpty()))

		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Deleting the OoBM")
		Expect(k8sClient.Delete(ctx, OoBM)).To(Succeed())

		By("Ensuring that the Server OoBM ref is empty")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", BeNil()),
		))
	})

	It("should create maintenance if policy is Ownerapproved", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)
		OoBMSetting["fooreboot"] = "144"

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-change",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "1.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Ensuring that the Server has the OoBM setting ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))

		// due to how we mock the settng update in unit tests, this is super fast Hence can reach completeed state already
		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInSettingUpdate),
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Ensuring that the Maintenance resource has been created")
		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		By("Ensuring that the Maintenance resource has been referenced by OoBM resource")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
		))

		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))
	})

	It("should move to through upgrade state if no OoBM version is not right", func(ctx SpecContext) {
		OoBMSetting := make(map[string]string)

		By("Ensuring that the BMC has right state: enabled")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.BMCStateEnabled),
		))

		By("Creating a OoBMSetting")
		OoBM := &metalv1alpha1.OOBMSettings{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-oobm-upgrade",
			},
			Spec: metalv1alpha1.OOBMSettingsSpec{
				OOBMSettings:            metalv1alpha1.OOBMSettingsMap{Version: "2.45.455b66-rev4", Settings: OoBMSetting},
				ServerRef:               &v1.LocalObjectReference{Name: server.Name},
				ServerMaintenancePolicy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
			},
		}
		Expect(k8sClient.Create(ctx, OoBM)).To(Succeed())

		By("Ensuring that the OoBM has the correct state, reached InVersionUpgrade")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInVersionUpgrade),
		))

		By("Ensuring that the BMC has the correct BMC settings ref")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", Not(BeNil())),
			HaveField("Spec.OoBMSettingRef.Name", OoBM.Name),
		))

		var serverMaintenanceList metalv1alpha1.ServerMaintenanceList
		Eventually(ObjectList(&serverMaintenanceList)).Should(HaveField("Items", Not(BeEmpty())))

		By("Ensuring that the Maintenance resource has been referenced by OoBM resource")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
		))

		By("Ensuring that the OoBM controller has created the Maintenance request")
		serverMaintenance := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      OoBM.Spec.ServerMaintenanceRef.Name,
			},
		}
		Eventually(Get(serverMaintenance)).Should(Succeed())

		By("Ensuring that the server has accepted the Maintenance request")
		Eventually(Object(server)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", Not(BeNil())),
			HaveField("Spec.ServerMaintenanceRef.UID", serverMaintenance.UID),
			HaveField("Spec.ServerMaintenanceRef.UID", OoBM.Spec.ServerMaintenanceRef.UID),
		))

		By("Ensuring that the OoBM resource state is correct State inVersionUpgrade")
		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInVersionUpgrade),
		))

		By("Simulate the server OoBM version update by matching the spec version")
		Eventually(Update(OoBM, func() {
			OoBM.Spec.OOBMSettings.Version = "1.45.455b66-rev4"
		})).Should(Succeed())

		By("Ensuring that the OoBM resource has completed Upgrade and setting update, and moved the state")
		Eventually(Object(OoBM)).Should(SatisfyAny(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateInSettingUpdate),
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.OoBMMaintenanceStateSynced),
		))

		By("Ensuring that the serverMaintenance not ref anymore")
		Eventually(Object(OoBM)).Should(SatisfyAll(
			HaveField("Spec.ServerMaintenanceRef", BeNil()),
		))

		By("Ensuring that the serverMaintenance is deleted")
		Eventually(Get(serverMaintenance)).Should(Satisfy(apierrors.IsNotFound))

		By("Deleting the OoBMSetting resource")
		Expect(k8sClient.Delete(ctx, OoBM)).To(Succeed())

		By("Ensuring that the OoBM resource is removed")
		Eventually(Get(OoBM)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring that the Server OoBM ref is empty")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Spec.OoBMSettingRef", BeNil()),
		))
	})
})

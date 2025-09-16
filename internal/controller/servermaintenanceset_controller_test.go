// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerMaintenanceSet Controller", func() {
	ns := SetupTest()

	var servermaintenanceset *metalv1alpha1.ServerMaintenanceSet
	var server01 *metalv1alpha1.Server
	var server02 *metalv1alpha1.Server
	var bmcSecret *metalv1alpha1.BMCSecret

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
		server01 = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-1-",
				Labels: map[string]string{
					"metal.ironcore.dev/hostname": "test-hostname",
				},
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
		Expect(k8sClient.Create(ctx, server01)).Should(Succeed())

		By("Creating a second Server")
		server02 = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-2-",
				Labels: map[string]string{
					"metal.ironcore.dev/hostname": "test-hostname",
					"metal.ironcore.dev/foo":      "bar",
				},
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
		Expect(k8sClient.Create(ctx, server02)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})
	It("should successfully reconcile the resource", func(ctx SpecContext) {
		By("Reconciling the created resource")

		servermaintenanceset = &metalv1alpha1.ServerMaintenanceSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSetSpec{
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"metal.ironcore.dev/hostname": "test-hostname",
					},
				},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy: metalv1alpha1.ServerMaintenancePolicyEnforced,
				},
			},
		}
		Expect(k8sClient.Create(ctx, servermaintenanceset)).To(Succeed())

		By("Checking if the maintenance has been created")
		maintenance01 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servermaintenanceset.Name + "-0",
				Namespace: servermaintenanceset.Namespace,
			},
		}
		Eventually(Object(maintenance01)).Should(SatisfyAll(
			HaveField("Status.State", Equal(metalv1alpha1.ServerMaintenanceStateInMaintenance)),
			HaveField("Spec.ServerRef.Name", Not(BeEmpty())),
			HaveField("Spec.Policy", Equal(metalv1alpha1.ServerMaintenancePolicyEnforced)),
		))
		By("Checking if the 2nd maintenance has been created")
		maintenance02 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servermaintenanceset.Name + "-1",
				Namespace: servermaintenanceset.Namespace,
			},
		}
		Eventually(Object(maintenance02)).Should(SatisfyAll(
			HaveField("Status.State", Equal(metalv1alpha1.ServerMaintenanceStateInMaintenance)),
			HaveField("Spec.ServerRef.Name", Not(BeEmpty())),
			HaveField("Spec.Policy", Equal(metalv1alpha1.ServerMaintenancePolicyEnforced)),
		))

		By("Checking if the status has been updated")
		Eventually(Object(servermaintenanceset)).Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 2)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 2)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
		))

		By("Deleting the first maintenance")
		Expect(k8sClient.Delete(ctx, maintenance01)).To(Succeed())

		By("Checking that maintenance is recreated")
		Eventually(Object(servermaintenanceset)).Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 2)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 2)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
		))

		By("Creating a third Server after set has been created")
		server03 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-3-",
				Labels: map[string]string{
					"metal.ironcore.dev/hostname": "test-hostname",
				},
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

		Expect(k8sClient.Create(ctx, server03)).Should(Succeed())

		By("Checking if the third maintenance has been created")
		maintenance03 := &metalv1alpha1.ServerMaintenance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servermaintenanceset.Name + "-2",
				Namespace: servermaintenanceset.Namespace,
			},
		}
		Eventually(Object(maintenance03)).Should(SatisfyAll(
			HaveField("Status.State", Equal(metalv1alpha1.ServerMaintenanceStateInMaintenance)),
			HaveField("Spec.ServerRef.Name", Not(BeEmpty())),
		))

		By("Checking if the status has been updated")
		Eventually(Object(servermaintenanceset), "5s").Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 3)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 3)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
			HaveField("Status.Completed", BeNumerically("==", 0)),
		))

		By("Setting the maintenances to completed")
		Eventually(UpdateStatus(maintenance01, func() {
			maintenance01.Status.State = metalv1alpha1.ServerMaintenanceStateCompleted
		})).Should(Succeed())
		Eventually(UpdateStatus(maintenance02, func() {
			maintenance02.Status.State = metalv1alpha1.ServerMaintenanceStateCompleted
		})).Should(Succeed())
		Eventually(UpdateStatus(maintenance03, func() {
			maintenance03.Status.State = metalv1alpha1.ServerMaintenanceStateCompleted
		})).Should(Succeed())

		By("Deleting the first server")
		Expect(k8sClient.Delete(ctx, server01)).To(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(servermaintenanceset)).Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 2)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 0)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
			HaveField("Status.Completed", BeNumerically("==", 2)),
		))

		By("Deleting the resource")
		Expect(k8sClient.Delete(ctx, servermaintenanceset)).To(Succeed())

	})

	It("should successfully react to server label changes", func(ctx SpecContext) {
		By("Reconciling the created resource")

		servermaintenanceset = &metalv1alpha1.ServerMaintenanceSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceSetSpec{
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"metal.ironcore.dev/hostname": "test-hostname",
					},
				},
				ServerMaintenanceTemplate: metalv1alpha1.ServerMaintenanceTemplate{
					Policy: metalv1alpha1.ServerMaintenancePolicyOwnerApproval,
				},
			},
		}
		Expect(k8sClient.Create(ctx, servermaintenanceset)).To(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(servermaintenanceset)).Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 2)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 2)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
			HaveField("Status.Completed", BeNumerically("==", 0)),
		))

		By("Changing the label of the first server")
		Eventually(Update(server01, func() {
			server01.Labels["metal.ironcore.dev/hostname"] = "nope"
		})).Should(Succeed())

		By("Checking if the status has been updated")
		Eventually(Object(servermaintenanceset)).Should(SatisfyAll(
			HaveField("Status.Maintenances", BeNumerically("==", 1)),
			HaveField("Status.Pending", BeNumerically("==", 0)),
			HaveField("Status.InMaintenance", BeNumerically("==", 1)),
			HaveField("Status.Failed", BeNumerically("==", 0)),
			HaveField("Status.Completed", BeNumerically("==", 0)),
		))

		By("Deleting the resource")
		Expect(k8sClient.Delete(ctx, servermaintenanceset)).To(Succeed())
	})
})

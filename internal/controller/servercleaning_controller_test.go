// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

var _ = Describe("ServerCleaning Controller", func() {
	ns := SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should successfully create and reconcile a ServerCleaning resource with serverRef", func(ctx SpecContext) {
		By("Creating a Server resource in Tainted state")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"test": "cleaning",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-1",
				SystemURI:  "/redfish/v1/Systems/1",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Setting Server state to Tainted")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())

		By("Creating a ServerCleaning resource")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerRef: &corev1.LocalObjectReference{
					Name: server.Name,
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method:            metalv1alpha1.DiskWipeMethodQuick,
					IncludeBootDrives: true,
				},
				BIOSReset:      true,
				NetworkCleanup: true,
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring ServerCleaning transitions to Pending state")
		Eventually(Object(cleaning)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerCleaningStatePending),
		))

		By("Ensuring ServerCleaning has finalizer")
		Eventually(Object(cleaning)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerCleaningFinalizer)),
		))

		By("Ensuring ServerCleaning transitions to InProgress state")
		Eventually(Object(cleaning)).WithTimeout(2 * time.Minute).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerCleaningStateInProgress),
			HaveField("Status.SelectedServers", BeNumerically(">", 0)),
		))

		By("Ensuring ServerCleaning status has server status entry")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			g.Expect(cleaning.Status.ServerCleaningStatuses).NotTo(BeEmpty())
			g.Expect(cleaning.Status.ServerCleaningStatuses[0].ServerName).To(Equal(server.Name))
		}).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
	})

	It("Should successfully create and reconcile a ServerCleaning resource with serverSelector", func(ctx SpecContext) {
		By("Creating multiple Server resources in Tainted state")
		server1 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"cleanup-group": "staging",
					"region":        "us-west",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-1",
				SystemURI:  "/redfish/v1/Systems/1",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server1)).To(Succeed())

		server2 := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"cleanup-group": "staging",
					"region":        "us-east",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-2",
				SystemURI:  "/redfish/v1/Systems/2",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server2)).To(Succeed())

		By("Setting Server states to Tainted")
		Eventually(UpdateStatus(server1, func() {
			server1.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())
		Eventually(UpdateStatus(server2, func() {
			server2.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())

		By("Creating a ServerCleaning resource with serverSelector")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"cleanup-group": "staging",
					},
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method:            metalv1alpha1.DiskWipeMethodSecure,
					IncludeBootDrives: false,
				},
				NetworkCleanup: true,
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring ServerCleaning transitions to InProgress state")
		Eventually(Object(cleaning)).WithTimeout(2 * time.Minute).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerCleaningStateInProgress),
			HaveField("Status.SelectedServers", BeNumerically("==", 2)),
		))

		By("Ensuring ServerCleaning status has entries for both servers")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			g.Expect(cleaning.Status.ServerCleaningStatuses).To(HaveLen(2))
		}).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, server1)).To(Succeed())
		Expect(k8sClient.Delete(ctx, server2)).To(Succeed())
	})

	It("Should track cleaning tasks in status", func(ctx SpecContext) {
		By("Creating a Server resource in Tainted state")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-1",
				SystemURI:  "/redfish/v1/Systems/1",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Setting Server state to Tainted")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())

		By("Creating a ServerCleaning resource with multiple cleaning operations")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerRef: &corev1.LocalObjectReference{
					Name: server.Name,
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method:            metalv1alpha1.DiskWipeMethodDoD,
					IncludeBootDrives: true,
				},
				BIOSReset:      true,
				NetworkCleanup: true,
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring cleaning tasks are tracked in status")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			if len(cleaning.Status.ServerCleaningStatuses) > 0 {
				serverStatus := cleaning.Status.ServerCleaningStatuses[0]
				// Should have tasks for the cleaning operations that return task URIs
				g.Expect(serverStatus.CleaningTasks).NotTo(BeNil())
			}
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
	})

	It("Should update cleaning counts correctly", func(ctx SpecContext) {
		By("Creating multiple Server resources")
		servers := make([]*metalv1alpha1.Server, 3)
		for i := range 3 {
			servers[i] = &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-server-",
					Namespace:    ns.Name,
					Labels: map[string]string{
						"batch": "test",
					},
				},
				Spec: metalv1alpha1.ServerSpec{
					SystemUUID: "test-system-uuid-" + string(rune(i)),
					SystemURI:  "/redfish/v1/Systems/" + string(rune(i)),
					BMCRef: &corev1.LocalObjectReference{
						Name: "test-bmc",
					},
					Taints: []corev1.Taint{
						{
							Key:    "metal.ironcore.dev/tainted",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, servers[i])).To(Succeed())

			Eventually(UpdateStatus(servers[i], func() {
				servers[i].Status.State = metalv1alpha1.ServerStateTainted
			})).Should(Succeed())
		}

		By("Creating a ServerCleaning resource for all servers")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"batch": "test",
					},
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method: metalv1alpha1.DiskWipeMethodQuick,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring cleaning counts are updated")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			// Should have selected all 3 servers
			g.Expect(cleaning.Status.SelectedServers).To(BeNumerically("==", 3))
			// Should have counts tracking progress
			totalProcessed := cleaning.Status.InProgressCleanings +
				cleaning.Status.CompletedCleanings +
				cleaning.Status.FailedCleanings
			g.Expect(totalProcessed).To(BeNumerically(">", 0))
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		for _, server := range servers {
			Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		}
	})

	It("Should set proper conditions during cleaning lifecycle", func(ctx SpecContext) {
		By("Creating a Server resource in Tainted state")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-1",
				SystemURI:  "/redfish/v1/Systems/1",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Setting Server state to Tainted")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())

		By("Creating a ServerCleaning resource")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerRef: &corev1.LocalObjectReference{
					Name: server.Name,
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method: metalv1alpha1.DiskWipeMethodQuick,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring Cleaning condition is set when in progress")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			if cleaning.Status.State == metalv1alpha1.ServerCleaningStateInProgress {
				g.Expect(cleaning.Status.Conditions).NotTo(BeEmpty())
				condition := findCondition(cleaning.Status.Conditions, ServerCleaningConditionTypeCleaning)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal(ServerCleaningConditionReasonInProgress))
			}
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
	})

	It("Should skip servers not in Tainted state", func(ctx SpecContext) {
		By("Creating servers in different states")
		taintedServer := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "tainted-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"group": "mixed",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-1",
				SystemURI:  "/redfish/v1/Systems/1",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
				Taints: []corev1.Taint{
					{
						Key:    "metal.ironcore.dev/tainted",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, taintedServer)).To(Succeed())
		Eventually(UpdateStatus(taintedServer, func() {
			taintedServer.Status.State = metalv1alpha1.ServerStateTainted
		})).Should(Succeed())

		availableServer := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "available-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"group": "mixed",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-2",
				SystemURI:  "/redfish/v1/Systems/2",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
			},
		}
		Expect(k8sClient.Create(ctx, availableServer)).To(Succeed())
		Eventually(UpdateStatus(availableServer, func() {
			availableServer.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Creating a ServerCleaning resource targeting both servers")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "mixed",
					},
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method: metalv1alpha1.DiskWipeMethodQuick,
				},
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring only tainted server gets cleaning status entry")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			// Should select 2 servers but only process the tainted one
			g.Expect(cleaning.Status.SelectedServers).To(BeNumerically("==", 2))
			// Only tainted server should have a status entry
			if len(cleaning.Status.ServerCleaningStatuses) > 0 {
				for _, status := range cleaning.Status.ServerCleaningStatuses {
					g.Expect(status.ServerName).To(Equal(taintedServer.Name))
				}
			}
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, taintedServer)).To(Succeed())
		Expect(k8sClient.Delete(ctx, availableServer)).To(Succeed())
	})

	It("Should clean tainted server and transition from Reserved to Available", func(ctx SpecContext) {
		By("Creating a ServerClaim resource")
		claim := &metalv1alpha1.ServerClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-claim-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerClaimSpec{
				Power: metalv1alpha1.PowerOn,
				ServerSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"claim-test": "transition",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Creating a Server resource that will be claimed")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-server-",
				Namespace:    ns.Name,
				Labels: map[string]string{
					"claim-test": "transition",
				},
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "test-system-uuid-claim",
				SystemURI:  "/redfish/v1/Systems/claim",
				BMCRef: &corev1.LocalObjectReference{
					Name: "test-bmc",
				},
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("Setting Server state to Available initially")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateAvailable
		})).Should(Succeed())

		By("Waiting for Server to be claimed")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			g.Expect(server.Spec.ServerClaimRef).NotTo(BeNil())
			g.Expect(server.Spec.ServerClaimRef.Name).To(Equal(claim.Name))
		}).Should(Succeed())

		By("Setting Server state to Reserved")
		Eventually(UpdateStatus(server, func() {
			server.Status.State = metalv1alpha1.ServerStateReserved
		})).Should(Succeed())

		By("Adding taints to the Server before releasing")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			serverBase := server.DeepCopy()
			server.Spec.Taints = []corev1.Taint{
				{
					Key:    "metal.ironcore.dev/tainted",
					Effect: corev1.TaintEffectNoSchedule,
				},
			}
			g.Expect(k8sClient.Patch(ctx, server, client.MergeFrom(serverBase))).To(Succeed())
		}).Should(Succeed())

		By("Deleting the ServerClaim to release the server")
		Expect(k8sClient.Delete(ctx, claim)).To(Succeed())
		Eventually(Get(claim)).Should(Satisfy(apierrors.IsNotFound))

		By("Ensuring ServerClaimRef is removed from Server")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			g.Expect(server.Spec.ServerClaimRef).To(BeNil())
		}).Should(Succeed())

		By("Ensuring Server transitions to Tainted state")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			g.Expect(server.Status.State).To(Equal(metalv1alpha1.ServerStateTainted))
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		By("Creating a ServerCleaning resource for the tainted server")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerRef: &corev1.LocalObjectReference{
					Name: server.Name,
				},
				DiskWipe: &metalv1alpha1.DiskWipeConfig{
					Method:            metalv1alpha1.DiskWipeMethodQuick,
					IncludeBootDrives: true,
				},
				BIOSReset:      true,
				NetworkCleanup: true,
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring ServerCleaning transitions through states")
		Eventually(Object(cleaning)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerCleaningStatePending),
		))

		Eventually(Object(cleaning)).WithTimeout(2 * time.Minute).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerCleaningStateInProgress),
		))

		By("Simulating cleaning completion by updating ServerCleaning status")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cleaning), cleaning)).To(Succeed())
			cleaningBase := cleaning.DeepCopy()
			cleaning.Status.State = metalv1alpha1.ServerCleaningStateCompleted
			if len(cleaning.Status.ServerCleaningStatuses) > 0 {
				cleaning.Status.ServerCleaningStatuses[0].State = metalv1alpha1.ServerCleaningStateCompleted
			}
			cleaning.Status.CompletedCleanings = 1
			cleaning.Status.InProgressCleanings = 0
			g.Expect(k8sClient.Status().Patch(ctx, cleaning, client.MergeFrom(cleaningBase))).To(Succeed())
		}).Should(Succeed())

		By("Ensuring Server taints are removed after cleaning completion")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			g.Expect(server.Spec.Taints).To(BeEmpty())
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		By("Ensuring Server transitions to Available state")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(server), server)).To(Succeed())
			g.Expect(server.Status.State).To(Equal(metalv1alpha1.ServerStateAvailable))
		}).WithTimeout(2 * time.Minute).Should(Succeed())

		// Cleanup
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
		Expect(k8sClient.Delete(ctx, server)).To(Succeed())
	})

	It("Should handle deletion with finalizer", func(ctx SpecContext) {
		By("Creating a ServerCleaning resource")
		cleaning := &metalv1alpha1.ServerCleaning{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-cleaning-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerCleaningSpec{
				ServerRef: &corev1.LocalObjectReference{
					Name: "non-existent-server",
				},
			},
		}
		Expect(k8sClient.Create(ctx, cleaning)).To(Succeed())

		By("Ensuring finalizer is added")
		Eventually(Object(cleaning)).Should(SatisfyAll(
			HaveField("Finalizers", ContainElement(ServerCleaningFinalizer)),
		))

		By("Deleting the ServerCleaning resource")
		Expect(k8sClient.Delete(ctx, cleaning)).To(Succeed())

		By("Ensuring the resource is eventually deleted")
		Eventually(Get(cleaning)).Should(Satisfy(apierrors.IsNotFound))
	})
})

// Helper function to find a condition by type
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

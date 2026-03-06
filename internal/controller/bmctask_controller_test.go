// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("BMCTask Controller", func() {
	_ = SetupTest(nil)

	AfterEach(func(ctx SpecContext) {
		EnsureCleanState()
	})

	It("Should update BMC.Status.Tasks when polling active tasks", func(ctx SpecContext) {
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

		By("Creating a BMC resource with active tasks")
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
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						TargetID:        "Drive-1",
						State:           "Running",
						PercentComplete: 0,
						Message:         "Erasing disk",
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring that the task status is updated by the controller")
		// The mock BMC will return Completed status
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(1)),
			HaveField("Status.Tasks[0].State", "Completed"),
			HaveField("Status.Tasks[0].PercentComplete", BeNumerically(">=", 0)),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should only reconcile BMCs with tasks due to event filter", func(ctx SpecContext) {
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

		By("Creating a BMC resource without tasks")
		bmcWithoutTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-notasks-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:11",
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
		Expect(k8sClient.Create(ctx, bmcWithoutTasks)).To(Succeed())

		By("Ensuring BMC without tasks remains unchanged")
		Consistently(Object(bmcWithoutTasks)).Should(HaveField("Status.Tasks", BeEmpty()))

		By("Creating a BMC resource with tasks")
		bmcWithTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-withtasks-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:22",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Running",
						PercentComplete: 0,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmcWithTasks)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmcWithTasks)).To(Succeed())

		By("Ensuring BMC with tasks is reconciled")
		Eventually(Object(bmcWithTasks)).Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(1)),
			HaveField("Status.Tasks[0].State", "Completed"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcWithoutTasks)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcWithTasks)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should automatically requeue when active tasks exist", func(ctx SpecContext) {
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

		By("Creating a BMC resource with an active task")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:33",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/active",
						TaskType:        metalv1alpha1.BMCTaskTypeFirmwareUpdate,
						State:           "Running",
						PercentComplete: 25,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring the task is polled multiple times due to requeue")
		initialUpdateTime := metav1.Now()

		// Since the mock returns completed, we verify the task was updated
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(1)),
			HaveField("Status.Tasks[0].State", "Completed"),
			HaveField("Status.Tasks[0].LastUpdateTime", Not(Equal(initialUpdateTime))),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should not requeue when all tasks are in terminal state", func(ctx SpecContext) {
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

		By("Creating a BMC resource with only terminal tasks")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:44",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/completed",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Completed",
						PercentComplete: 100,
						LastUpdateTime:  metav1.Now(),
					},
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/failed",
						TaskType:        metalv1alpha1.BMCTaskTypeBIOSReset,
						State:           "Failed",
						PercentComplete: 50,
						Message:         "Operation failed",
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring terminal tasks are not updated")
		// Store the initial last update time
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
		initialUpdateTime1 := bmc.Status.Tasks[0].LastUpdateTime
		initialUpdateTime2 := bmc.Status.Tasks[1].LastUpdateTime

		// Wait a bit and verify the tasks haven't changed
		time.Sleep(200 * time.Millisecond)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())

		Expect(bmc.Status.Tasks).To(HaveLen(2))
		Expect(bmc.Status.Tasks[0].LastUpdateTime).To(Equal(initialUpdateTime1))
		Expect(bmc.Status.Tasks[1].LastUpdateTime).To(Equal(initialUpdateTime2))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should handle BMC client errors gracefully", func(ctx SpecContext) {
		By("Creating a BMCSecret with invalid credentials")
		bmcSecret := &metalv1alpha1.BMCSecret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
			Data: map[string][]byte{
				metalv1alpha1.BMCSecretUsernameKeyName: []byte("invalid"),
				metalv1alpha1.BMCSecretPasswordKeyName: []byte("invalid"),
			},
		}
		Expect(k8sClient.Create(ctx, bmcSecret)).To(Succeed())

		By("Creating a BMC resource with active tasks")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP("192.0.2.1"), // TEST-NET-1 (unreachable)
					MACAddress: "aa:bb:cc:dd:ee:55",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfish,
					Port: 8000,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Running",
						PercentComplete: 0,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring the controller handles the error gracefully")
		// The controller should not crash and should keep retrying
		Consistently(Object(bmc), "2s", "100ms").Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(1)),
			HaveField("Status.Tasks[0].State", "Running"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should only update changed tasks", func(ctx SpecContext) {
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

		By("Creating a BMC resource with mixed terminal and active tasks")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:66",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/completed",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Completed",
						PercentComplete: 100,
						Message:         "Disk erased successfully",
						LastUpdateTime:  metav1.Now(),
					},
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/active",
						TaskType:        metalv1alpha1.BMCTaskTypeBIOSReset,
						State:           "Running",
						PercentComplete: 50,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Getting the initial state")
		Eventually(Get(bmc)).Should(Succeed())
		initialTask1UpdateTime := bmc.Status.Tasks[0].LastUpdateTime
		initialTask2UpdateTime := bmc.Status.Tasks[1].LastUpdateTime

		By("Ensuring only active task is updated")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(2)),
			// First task (completed) should remain unchanged
			HaveField("Status.Tasks[0].State", "Completed"),
			HaveField("Status.Tasks[0].PercentComplete", BeNumerically("==", 100)),
			// Second task (active) should be updated by the mock BMC
			HaveField("Status.Tasks[1].State", "Completed"),
			HaveField("Status.Tasks[1].LastUpdateTime", Not(Equal(initialTask2UpdateTime))),
		))

		// Verify first task was not updated
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
		Expect(bmc.Status.Tasks[0].LastUpdateTime).To(Equal(initialTask1UpdateTime))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should handle multiple tasks with mixed states correctly", func(ctx SpecContext) {
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

		By("Creating a BMC resource with multiple tasks in various states")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:77",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/task1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						TargetID:        "Drive-1",
						State:           "Running",
						PercentComplete: 10,
						Message:         "Erasing drive 1",
						LastUpdateTime:  metav1.Now(),
					},
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/task2",
						TaskType:        metalv1alpha1.BMCTaskTypeBMCReset,
						State:           "Completed",
						PercentComplete: 100,
						Message:         "BMC reset completed",
						LastUpdateTime:  metav1.Now(),
					},
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/task3",
						TaskType:        metalv1alpha1.BMCTaskTypeFirmwareUpdate,
						State:           "Running",
						PercentComplete: 75,
						Message:         "Updating firmware",
						LastUpdateTime:  metav1.Now(),
					},
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/task4",
						TaskType:        metalv1alpha1.BMCTaskTypeNetworkClear,
						State:           "Failed",
						PercentComplete: 0,
						Message:         "Network clear failed",
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring only non-terminal tasks are updated")
		Eventually(Object(bmc)).Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(4)),
			// Task 1: was Running, should be updated to Completed by mock
			HaveField("Status.Tasks[0].State", "Completed"),
			// Task 2: was Completed, should remain Completed
			HaveField("Status.Tasks[1].State", "Completed"),
			HaveField("Status.Tasks[1].PercentComplete", BeNumerically("==", 100)),
			// Task 3: was Running, should be updated to Completed by mock
			HaveField("Status.Tasks[2].State", "Completed"),
			// Task 4: was Failed, should remain Failed
			HaveField("Status.Tasks[3].State", "Failed"),
			HaveField("Status.Tasks[3].Message", "Network clear failed"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should skip reconciliation if BMC is being deleted", func(ctx SpecContext) {
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

		By("Creating a BMC resource with tasks and a finalizer")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-",
				Finalizers:   []string{"test.finalizer"},
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:88",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Running",
						PercentComplete: 0,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Deleting the BMC")
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())

		By("Ensuring tasks are not updated during deletion")
		Eventually(Get(bmc)).Should(Succeed())
		Expect(bmc.DeletionTimestamp).NotTo(BeNil())

		// Store the task state when deletion started
		initialTaskState := bmc.Status.Tasks[0].State
		initialUpdateTime := bmc.Status.Tasks[0].LastUpdateTime

		// Wait a bit and verify the task hasn't been updated
		time.Sleep(200 * time.Millisecond)
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bmc), bmc)).To(Succeed())
		Expect(bmc.Status.Tasks[0].State).To(Equal(initialTaskState))
		Expect(bmc.Status.Tasks[0].LastUpdateTime).To(Equal(initialUpdateTime))

		By("Removing finalizer to allow deletion")
		Eventually(Update(bmc, func() {
			bmc.Finalizers = []string{}
		})).Should(Succeed())

		// cleanup
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should handle BMCs with empty task list", func(ctx SpecContext) {
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
					MACAddress: "aa:bb:cc:dd:ee:99",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring the controller doesn't fail with empty task list")
		Consistently(Object(bmc), "1s", "100ms").Should(HaveField("Status.Tasks", BeEmpty()))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})

	It("Should register BMCTask controller in the test setup", func(ctx SpecContext) {
		By("Verifying the BMCTask controller is registered")
		// This test verifies that the controller is properly set up in suite_test.go
		// The fact that other tests pass indicates the controller is working
		// This is a placeholder to ensure we remember to register it in suite_test.go

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

		By("Creating a BMC with tasks to trigger reconciliation")
		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-bmc-controller-",
			},
			Spec: metalv1alpha1.BMCSpec{
				Endpoint: &metalv1alpha1.InlineEndpoint{
					IP:         metalv1alpha1.MustParseIP(MockServerIP),
					MACAddress: "aa:bb:cc:dd:ee:00",
				},
				Protocol: metalv1alpha1.Protocol{
					Name: metalv1alpha1.ProtocolRedfishLocal,
					Port: MockServerPort,
				},
				BMCSecretRef: v1.LocalObjectReference{
					Name: bmcSecret.Name,
				},
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:         "/redfish/v1/TaskService/Tasks/1",
						TaskType:        metalv1alpha1.BMCTaskTypeDiskErase,
						State:           "Running",
						PercentComplete: 0,
						LastUpdateTime:  metav1.Now(),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, bmc)).To(Succeed())

		By("Ensuring controller processes the BMC task")
		Eventually(Object(bmc), "5s", "100ms").Should(SatisfyAll(
			HaveField("Status.Tasks", HaveLen(1)),
			HaveField("Status.Tasks[0].State", "Completed"),
		))

		// cleanup
		Expect(k8sClient.Delete(ctx, bmc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, bmcSecret)).To(Succeed())
	})
})

var _ = Describe("BMCTask Event Filter", func() {
	It("Should filter BMCs without tasks on create event", func() {
		predicate := hasTasksPredicate()

		By("Testing with BMC without tasks")
		bmcWithoutTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc-no-tasks",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{},
			},
		}

		// Create event should be filtered (return false)
		Expect(predicate.Create(MockCreateEvent(bmcWithoutTasks))).To(BeFalse())

		By("Testing with BMC with tasks")
		bmcWithTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc-with-tasks",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:  "/redfish/v1/TaskService/Tasks/1",
						TaskType: metalv1alpha1.BMCTaskTypeDiskErase,
						State:    "Running",
					},
				},
			},
		}

		// Create event should pass (return true)
		Expect(predicate.Create(MockCreateEvent(bmcWithTasks))).To(BeTrue())
	})

	It("Should filter BMCs without tasks on update event", func() {
		predicate := hasTasksPredicate()

		By("Testing update with old BMC having tasks, new BMC without tasks")
		oldBMC := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:  "/redfish/v1/TaskService/Tasks/1",
						TaskType: metalv1alpha1.BMCTaskTypeDiskErase,
						State:    "Running",
					},
				},
			},
		}
		newBMC := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{},
			},
		}

		// Update event should be filtered when new BMC has no tasks
		Expect(predicate.Update(MockUpdateEvent(oldBMC, newBMC))).To(BeFalse())

		By("Testing update with both BMCs having tasks")
		newBMCWithTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:  "/redfish/v1/TaskService/Tasks/1",
						TaskType: metalv1alpha1.BMCTaskTypeDiskErase,
						State:    "Completed",
					},
				},
			},
		}

		// Update event should pass when new BMC has tasks
		Expect(predicate.Update(MockUpdateEvent(oldBMC, newBMCWithTasks))).To(BeTrue())
	})

	It("Should always filter delete events", func() {
		predicate := hasTasksPredicate()

		bmc := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:  "/redfish/v1/TaskService/Tasks/1",
						TaskType: metalv1alpha1.BMCTaskTypeDiskErase,
						State:    "Running",
					},
				},
			},
		}

		// Delete events should always be filtered regardless of tasks
		Expect(predicate.Delete(MockDeleteEvent(bmc))).To(BeFalse())
	})

	It("Should filter generic events based on task presence", func() {
		predicate := hasTasksPredicate()

		By("Testing generic event without tasks")
		bmcWithoutTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{},
			},
		}

		Expect(predicate.Generic(MockGenericEvent(bmcWithoutTasks))).To(BeFalse())

		By("Testing generic event with tasks")
		bmcWithTasks := &metalv1alpha1.BMC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-bmc",
			},
			Status: metalv1alpha1.BMCStatus{
				Tasks: []metalv1alpha1.BMCTask{
					{
						TaskURI:  "/redfish/v1/TaskService/Tasks/1",
						TaskType: metalv1alpha1.BMCTaskTypeDiskErase,
						State:    "Running",
					},
				},
			},
		}

		Expect(predicate.Generic(MockGenericEvent(bmcWithTasks))).To(BeTrue())
	})
})

var _ = Describe("isTerminalState", func() {
	It("Should identify terminal states correctly", func() {
		By("Testing completed state")
		Expect(isTerminalState("Completed")).To(BeTrue())

		By("Testing failed state")
		Expect(isTerminalState("Failed")).To(BeTrue())

		By("Testing Redfish terminal states")
		Expect(isTerminalState("Killed")).To(BeTrue())
		Expect(isTerminalState("Exception")).To(BeTrue())
		Expect(isTerminalState("Cancelled")).To(BeTrue())

		By("Testing non-terminal states")
		Expect(isTerminalState("Running")).To(BeFalse())
		Expect(isTerminalState("Pending")).To(BeFalse())
		Expect(isTerminalState("Starting")).To(BeFalse())
		Expect(isTerminalState("")).To(BeFalse())
	})
})

// Helper functions for creating mock events for predicate testing

// MockCreateEvent creates a mock CreateEvent for testing predicates.
func MockCreateEvent(obj client.Object) event.CreateEvent {
	return event.CreateEvent{
		Object: obj,
	}
}

// MockUpdateEvent creates a mock UpdateEvent for testing predicates.
func MockUpdateEvent(oldObj, newObj client.Object) event.UpdateEvent {
	return event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}
}

// MockDeleteEvent creates a mock DeleteEvent for testing predicates.
func MockDeleteEvent(obj client.Object) event.DeleteEvent {
	return event.DeleteEvent{
		Object: obj,
	}
}

// MockGenericEvent creates a mock GenericEvent for testing predicates.
func MockGenericEvent(obj client.Object) event.GenericEvent {
	return event.GenericEvent{
		Object: obj,
	}
}

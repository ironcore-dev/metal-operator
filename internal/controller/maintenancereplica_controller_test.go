// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

var _ = Describe("ServerServerMaintenanceReplica Controller", func() {
	ns := SetupTest()

	var server *metalv1alpha1.Server

	BeforeEach(func(ctx SpecContext) {
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

		By("Creating a Server")
		server = &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-",
				Labels: map[string]string{
					"server": "test",
				},
				Namespace: ns.Name,
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
		Expect(k8sClient.Create(ctx, server)).Should(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		DeleteAllMetalResources(ctx, ns.Name)
	})

	It("should create a ServerServerMaintenanceReplica", func(ctx SpecContext) {

		By("Creating a ServerServerMaintenanceReplica")
		replica := &metalv1alpha1.ServerMaintenanceReplica{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-maintenance-replica-",
				Namespace:    ns.Name,
			},
			Spec: metalv1alpha1.ServerMaintenanceReplicaSpec{
				ServerSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"server": "test",
					},
				},
				Template: metalv1alpha1.ServerMaintenanceSpec{
					Policy:      metalv1alpha1.ServerMaintenancePolicyEnforced,
					ServerPower: metalv1alpha1.PowerOff,
				},
			},
		}
		Expect(k8sClient.Create(ctx, replica)).Should(Succeed())

		By("Expecting the ServerServerMaintenanceReplica to be created")
		Eventually(func() bool {
			key := types.NamespacedName{
				Namespace: replica.Namespace,
				Name:      replica.Name,
			}
			err := k8sClient.Get(ctx, key, replica)
			return err == nil
		}).Should(BeTrue())

		maintenance := &metalv1alpha1.ServerMaintenance{}
		By("Expecting a ServerMaintenance to be created")
		Eventually(func() bool {
			key := types.NamespacedName{
				Namespace: replica.Namespace,
				Name:      replica.Name,
			}
			err := k8sClient.Get(ctx, key, maintenance)
			fmt.Println(err)
			return err == nil
		}).Should(BeTrue())

		Eventually(Object(replica)).Should(SatisfyAll(
			HaveField("Status.Replicas", 1),
		))
	})
})

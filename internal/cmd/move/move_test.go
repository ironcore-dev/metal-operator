// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package move

import (
	"errors"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sSchema "k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("metalctl move", func() {
	_ = SetupTest()

	It("Should successfully move metal CRs from a source cluster on a target cluster", func(ctx SpecContext) {
		// source cluster setup
		sourceCommonEndpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-common-"},
			Spec: metalv1alpha1.EndpointSpec{
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP(MockServerIP),
			}}
		sourceEndpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"},
			Spec: metalv1alpha1.EndpointSpec{
				MACAddress: "23:11:8A:33:CF:EB",
				IP:         metalv1alpha1.MustParseIP(MockServerIP2),
			}}
		Expect(clients.Source.Create(ctx, sourceCommonEndpoint)).To(Succeed())
		Expect(clients.Source.Create(ctx, sourceEndpoint)).To(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Source.Get(ctx, client.ObjectKeyFromObject(sourceCommonEndpoint), sourceCommonEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Source.Get(ctx, client.ObjectKeyFromObject(sourceEndpoint), sourceEndpoint)
		}).Should(Succeed())

		sourceBmc := &metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"},
			Spec: metalv1alpha1.BMCSpec{EndpointRef: &v1.LocalObjectReference{}}}
		Expect(controllerutil.SetOwnerReference(sourceEndpoint, sourceBmc, k8sSchema.Scheme)).To(Succeed())
		Expect(clients.Source.Create(ctx, sourceBmc)).To(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Source.Get(ctx, client.ObjectKeyFromObject(sourceBmc), sourceBmc)
		}).Should(Succeed())
		sourceBmc.Status.PowerState = metalv1alpha1.PoweringOnPowerState
		Expect(clients.Source.Status().Update(ctx, sourceBmc)).To(Succeed())
		Eventually(func(g Gomega) error {
			if err := clients.Source.Get(ctx, client.ObjectKeyFromObject(sourceBmc), sourceBmc); err == nil {
				if sourceBmc.Status.PowerState == metalv1alpha1.PoweringOnPowerState {
					return nil
				}
			}
			return errors.New("waiting for status update")
		}).Should(Succeed())

		sourceBmcSecret := &metalv1alpha1.BMCSecret{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
		Expect(controllerutil.SetOwnerReference(sourceBmc, sourceBmcSecret, k8sSchema.Scheme)).To(Succeed())
		Expect(clients.Source.Create(ctx, sourceBmcSecret)).To(Succeed())

		// target cluster setup
		targetCommonEndpoint := sourceCommonEndpoint.DeepCopy()
		targetCommonEndpoint.SetResourceVersion("")
		Expect(clients.Target.Create(ctx, targetCommonEndpoint)).To(Succeed())
		targetEndpoint := &metalv1alpha1.Endpoint{}
		targetBmc := &metalv1alpha1.BMC{}
		targetBmcSecret := &metalv1alpha1.BMCSecret{}

		// TEST
		crds := []string{"BMC", "BMCSecret", "Endpoint", "Server", "ServerBootConfiguration", "ServerClaim"}
		crsSchema := make([]schema.GroupVersionKind, len(crds))
		for i, crdKind := range crds {
			crsSchema[i] = schema.GroupVersionKind{Group: "metal.ironcore.dev", Version: "v1alpha1", Kind: crdKind}
		}
		Expect(Move(ctx, clients, crsSchema, "", false)).ToNot(HaveOccurred())

		Eventually(func(g Gomega) error {
			return clients.Target.Get(ctx, client.ObjectKeyFromObject(targetCommonEndpoint), targetCommonEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Target.Get(ctx, client.ObjectKeyFromObject(sourceEndpoint), targetEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Target.Get(ctx, client.ObjectKeyFromObject(sourceBmc), targetBmc)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.Target.Get(ctx, client.ObjectKeyFromObject(sourceBmcSecret), targetBmcSecret)
		}).Should(Succeed())
		Expect(targetBmc.GetOwnerReferences()[0].UID).To(Equal(targetEndpoint.GetUID()))
		Expect(targetBmc.Status.PowerState).To(Equal(sourceBmc.Status.PowerState))
		Expect(targetBmcSecret.GetOwnerReferences()[0].UID).To(Equal(targetBmc.GetUID()))
	})
})

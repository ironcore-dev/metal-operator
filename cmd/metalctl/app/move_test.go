package app

import (
	"context"
	"log/slog"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sSchema "k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("metalctl move", func() {
	_ = SetupTest()

	It("Should successfully create metal CRDs and CRs from a source cluster on a target cluster", func(ctx SpecContext) {
		slog.SetLogLoggerLevel(slog.LevelDebug)

		// source cluster setup
		sourceCommonEndpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-common-"},
			Spec: metalv1alpha1.EndpointSpec{
				MACAddress: "23:11:8A:33:CF:EA",
				IP:         metalv1alpha1.MustParseIP("127.0.0.1"),
			}}
		sourceEndpoint := &metalv1alpha1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"},
			Spec: metalv1alpha1.EndpointSpec{
				MACAddress: "23:11:8A:33:CF:EB",
				IP:         metalv1alpha1.MustParseIP("127.0.0.2"),
			}}
		Expect(clients.source.Create(ctx, sourceCommonEndpoint)).To(Succeed())
		Expect(clients.source.Create(ctx, sourceEndpoint)).To(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceCommonEndpoint), sourceCommonEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceEndpoint), sourceEndpoint)
		}).Should(Succeed())

		sourceBmc := &metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
		controllerutil.SetOwnerReference(sourceCommonEndpoint, sourceBmc, k8sSchema.Scheme)
		Expect(clients.source.Create(ctx, sourceBmc)).To(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceBmc), sourceBmc)
		}).Should(Succeed())

		sourceBmcSecret := &metalv1alpha1.BMCSecret{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
		controllerutil.SetOwnerReference(sourceBmc, sourceBmcSecret, k8sSchema.Scheme)
		Expect(clients.source.Create(ctx, sourceBmcSecret)).To(Succeed())

		// target cluster setup
		targetCommonEndpoint := sourceCommonEndpoint.DeepCopy()
		targetCommonEndpoint.SetResourceVersion("")
		Expect(clients.target.Create(ctx, targetCommonEndpoint)).To(Succeed())
		targetEndpoint := &metalv1alpha1.Endpoint{}
		targetBmc := &metalv1alpha1.BMC{}
		targetBmcSecret := &metalv1alpha1.BMCSecret{}

		// TEST
		err := move(context.TODO(), clients)
		Expect(err).To(BeNil())

		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(targetCommonEndpoint), targetCommonEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceEndpoint), targetEndpoint)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceBmc), targetBmc)
		}).Should(Succeed())
		Eventually(func(g Gomega) error {
			return clients.source.Get(ctx, client.ObjectKeyFromObject(sourceBmcSecret), targetBmcSecret)
		}).Should(Succeed())
		Expect(targetBmc.GetOwnerReferences()[0].UID).To(Equal(targetCommonEndpoint.GetUID()))
		Expect(targetBmcSecret.GetOwnerReferences()[0].UID).To(Equal(targetBmc.GetUID()))
	})
})

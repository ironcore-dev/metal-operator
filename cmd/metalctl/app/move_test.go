package app

import (
	"context"
	"log/slog"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("metalctl move", func() {
	_ = SetupTest()

	It("Should successfully create metal CRDs and CRs from a source cluster on a target cluster", func(ctx SpecContext) {
		slog.SetLogLoggerLevel(slog.LevelDebug)

		sourceCr1 := &metalv1alpha1.Server{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
		sourceCr2 := &metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"}}
		commonSourceCr := &metalv1alpha1.BMC{ObjectMeta: metav1.ObjectMeta{Name: "test-common"}}
		commonTargetCr := commonSourceCr.DeepCopy()
		Expect(clients.source.Create(ctx, sourceCr1)).To(Succeed())
		Expect(clients.source.Create(ctx, sourceCr2)).To(Succeed())
		Expect(clients.source.Create(ctx, commonSourceCr)).To(Succeed())
		Expect(clients.target.Create(ctx, commonTargetCr)).To(Succeed())

		err := move(context.TODO(), clients)
		Expect(err).To(BeNil())

		Eventually(func(g Gomega) error {
			targetCr := metalv1alpha1.Server{}
			return clients.target.Get(context.Background(), client.ObjectKeyFromObject(sourceCr1), &targetCr)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			targetCr := metalv1alpha1.BMC{}
			return clients.target.Get(context.Background(), client.ObjectKeyFromObject(sourceCr2), &targetCr)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			targetCr := metalv1alpha1.BMC{}
			return clients.target.Get(context.Background(), client.ObjectKeyFromObject(commonSourceCr), &targetCr)
		}).Should(Succeed())
	})
})

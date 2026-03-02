// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"context"
	"testing"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/metrics"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var _ = Describe("ServerReconciliationTotal", func() {
	BeforeEach(func() {
		// Reset metric before each test
		metrics.ServerReconciliationTotal.Reset()
	})

	It("should increment success counter", func() {
		metrics.ServerReconciliationTotal.WithLabelValues("success").Inc()
		metrics.ServerReconciliationTotal.WithLabelValues("success").Inc()

		value := testutil.ToFloat64(metrics.ServerReconciliationTotal.WithLabelValues("success"))
		Expect(value).To(Equal(2.0))
	})

	It("should increment error counters separately", func() {
		metrics.ServerReconciliationTotal.WithLabelValues("success").Inc()
		metrics.ServerReconciliationTotal.WithLabelValues("error_fetch").Inc()
		metrics.ServerReconciliationTotal.WithLabelValues("error_reconcile").Inc()
		metrics.ServerReconciliationTotal.WithLabelValues("error_reconcile").Inc()

		successCount := testutil.ToFloat64(metrics.ServerReconciliationTotal.WithLabelValues("success"))
		Expect(successCount).To(Equal(1.0))

		fetchErrorCount := testutil.ToFloat64(metrics.ServerReconciliationTotal.WithLabelValues("error_fetch"))
		Expect(fetchErrorCount).To(Equal(1.0))

		reconcileErrorCount := testutil.ToFloat64(metrics.ServerReconciliationTotal.WithLabelValues("error_reconcile"))
		Expect(reconcileErrorCount).To(Equal(2.0))
	})
})

var _ = Describe("ServerStateCollector", func() {
	var (
		scheme     *runtime.Scheme
		fakeClient client.Client
		collector  *metrics.ServerStateCollector
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(metalv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		collector = metrics.NewServerStateCollector(fakeClient)
	})

	It("should implement prometheus.Collector interface", func() {
		var _ prometheus.Collector = collector
	})

	It("should emit zero metrics when no servers exist", func() {
		ch := make(chan prometheus.Metric, 100)
		collector.Collect(ch)
		close(ch)

		count := 0
		for range ch {
			count++
		}

		Expect(count).To(Equal(0))
	})

	It("should count servers by state", func() {
		ctx := context.Background()

		// Create servers in different states
		servers := []metalv1alpha1.Server{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server1"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid1"},
				Status:     metalv1alpha1.ServerStatus{State: metalv1alpha1.ServerStateAvailable},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server2"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid2"},
				Status:     metalv1alpha1.ServerStatus{State: metalv1alpha1.ServerStateAvailable},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server3"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid3"},
				Status:     metalv1alpha1.ServerStatus{State: metalv1alpha1.ServerStateReserved},
			},
		}

		for _, server := range servers {
			Expect(fakeClient.Create(ctx, &server)).To(Succeed())
		}

		// Collect metrics
		ch := make(chan prometheus.Metric, 100)
		collector.Collect(ch)
		close(ch)

		// Verify metrics were emitted
		metricCount := 0
		for range ch {
			metricCount++
		}

		Expect(metricCount).To(BeNumerically(">", 0))
	})

	It("should count servers by power state", func() {
		ctx := context.Background()

		// Create servers with different power states
		servers := []metalv1alpha1.Server{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server1"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid1"},
				Status: metalv1alpha1.ServerStatus{
					State:      metalv1alpha1.ServerStateAvailable,
					PowerState: metalv1alpha1.ServerOnPowerState,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server2"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid2"},
				Status: metalv1alpha1.ServerStatus{
					State:      metalv1alpha1.ServerStateAvailable,
					PowerState: metalv1alpha1.ServerOnPowerState,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "server3"},
				Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid3"},
				Status: metalv1alpha1.ServerStatus{
					State:      metalv1alpha1.ServerStateReserved,
					PowerState: metalv1alpha1.ServerOffPowerState,
				},
			},
		}

		for _, server := range servers {
			Expect(fakeClient.Create(ctx, &server)).To(Succeed())
		}

		// Collect metrics
		ch := make(chan prometheus.Metric, 100)
		collector.Collect(ch)
		close(ch)

		// Verify metrics were emitted
		metricCount := 0
		for range ch {
			metricCount++
		}

		Expect(metricCount).To(BeNumerically(">", 0))
	})

	It("should emit condition metrics", func() {
		ctx := context.Background()

		// Create server with conditions
		server := metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{Name: "server1"},
			Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid1"},
			Status: metalv1alpha1.ServerStatus{
				State:      metalv1alpha1.ServerStateAvailable,
				PowerState: metalv1alpha1.ServerOnPowerState,
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
					},
					{
						Type:   "Discovered",
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		Expect(fakeClient.Create(ctx, &server)).To(Succeed())

		// Collect metrics
		ch := make(chan prometheus.Metric, 100)
		collector.Collect(ch)
		close(ch)

		// Verify metrics were emitted
		metricCount := 0
		for range ch {
			metricCount++
		}

		// Should have state metric, power state metric, and condition metrics
		Expect(metricCount).To(BeNumerically(">=", 3))
	})

	It("should handle servers without state gracefully", func() {
		ctx := context.Background()

		// Create server without state set
		server := metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{Name: "server1"},
			Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid1"},
			Status:     metalv1alpha1.ServerStatus{},
		}

		Expect(fakeClient.Create(ctx, &server)).To(Succeed())

		// Collect metrics - should not panic
		ch := make(chan prometheus.Metric, 100)
		Expect(func() {
			collector.Collect(ch)
			close(ch)
		}).NotTo(Panic())
	})
})

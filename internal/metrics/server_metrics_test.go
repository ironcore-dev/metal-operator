// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"testing"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/metrics"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	prometheus_io "github.com/prometheus/client_model/go"
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

	It("should count servers by state", func(ctx SpecContext) {
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

		// Parse metrics and verify exact counts
		metrics := make(map[string]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil && len(dto.GetLabel()) > 0 {
				label := dto.GetLabel()[0]
				if label.GetName() == "state" {
					metrics[label.GetValue()] = dto.GetGauge().GetValue()
				}
			}
		}

		// Assert exact label/value pairs
		Expect(metrics["Available"]).To(Equal(2.0), "Expected 2 Available servers")
		Expect(metrics["Reserved"]).To(Equal(1.0), "Expected 1 Reserved server")
	})

	It("should count servers by power state", func(ctx SpecContext) {
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

		// Parse metrics and verify exact counts
		metrics := make(map[string]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil && len(dto.GetLabel()) > 0 {
				label := dto.GetLabel()[0]
				if label.GetName() == "power_state" {
					metrics[label.GetValue()] = dto.GetGauge().GetValue()
				}
			}
		}

		// Assert exact label/value pairs
		Expect(metrics["On"]).To(Equal(2.0), "Expected 2 On servers")
		Expect(metrics["Off"]).To(Equal(1.0), "Expected 1 Off server")
	})

	It("should emit condition metrics", func(ctx SpecContext) {
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

		// Parse metrics and verify exact counts
		conditionMetrics := make(map[string]map[string]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil && len(dto.GetLabel()) >= 2 {
				var condType, status string
				for _, label := range dto.GetLabel() {
					if label.GetName() == "condition_type" {
						condType = label.GetValue()
					}
					if label.GetName() == "status" {
						status = label.GetValue()
					}
				}
				if condType != "" && status != "" {
					if conditionMetrics[condType] == nil {
						conditionMetrics[condType] = make(map[string]float64)
					}
					conditionMetrics[condType][status] = dto.GetGauge().GetValue()
				}
			}
		}

		// Assert exact label/value pairs
		Expect(conditionMetrics["Ready"]["True"]).To(Equal(1.0), "Expected 1 Ready=True condition")
		Expect(conditionMetrics["Discovered"]["True"]).To(Equal(1.0), "Expected 1 Discovered=True condition")
	})

	It("should handle servers without state gracefully", func(ctx SpecContext) {
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

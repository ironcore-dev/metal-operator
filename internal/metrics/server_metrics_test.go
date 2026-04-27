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

const (
	labelServer = "server"
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

	It("should emit enum metrics for all states per server", func(ctx SpecContext) {
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

		// Parse metrics and verify enum pattern: all states emitted per server
		type stateKey struct {
			server, state string
		}
		stateMetrics := make(map[stateKey]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil {
				var key stateKey
				for _, label := range dto.GetLabel() {
					switch label.GetName() {
					case labelServer:
						key.server = label.GetValue()
					case "state":
						key.state = label.GetValue()
					}
				}
				if key.state != "" {
					stateMetrics[key] = dto.GetGauge().GetValue()
				}
			}
		}

		// Assert enum pattern: all 6 states emitted per server with 1 for current, 0 for others
		// server1 is Available
		Expect(stateMetrics[stateKey{server: "server1", state: "Initial"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Discovery"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Available"}]).To(Equal(1.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Reserved"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Error"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Maintenance"}]).To(Equal(0.0))

		// server2 is Available
		Expect(stateMetrics[stateKey{server: "server2", state: "Available"}]).To(Equal(1.0))
		Expect(stateMetrics[stateKey{server: "server2", state: "Reserved"}]).To(Equal(0.0))

		// server3 is Reserved
		Expect(stateMetrics[stateKey{server: "server3", state: "Available"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server3", state: "Reserved"}]).To(Equal(1.0))

		// Total: 3 servers × 6 states = 18 state metrics
		Expect(stateMetrics).To(HaveLen(18))
	})

	It("should emit enum metrics for all power states per server", func(ctx SpecContext) {
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

		// Parse metrics and verify enum pattern: all power states emitted per server
		type powerKey struct {
			server, powerState string
		}
		powerMetrics := make(map[powerKey]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil {
				var key powerKey
				for _, label := range dto.GetLabel() {
					switch label.GetName() {
					case labelServer:
						key.server = label.GetValue()
					case "power_state":
						key.powerState = label.GetValue()
					}
				}
				if key.powerState != "" {
					powerMetrics[key] = dto.GetGauge().GetValue()
				}
			}
		}

		// Assert enum pattern: all 5 power states emitted per server with 1 for current, 0 for others
		// server1 is On
		Expect(powerMetrics[powerKey{server: "server1", powerState: "On"}]).To(Equal(1.0))
		Expect(powerMetrics[powerKey{server: "server1", powerState: "Off"}]).To(Equal(0.0))
		Expect(powerMetrics[powerKey{server: "server1", powerState: "Paused"}]).To(Equal(0.0))
		Expect(powerMetrics[powerKey{server: "server1", powerState: "PoweringOn"}]).To(Equal(0.0))
		Expect(powerMetrics[powerKey{server: "server1", powerState: "PoweringOff"}]).To(Equal(0.0))

		// server2 is On
		Expect(powerMetrics[powerKey{server: "server2", powerState: "On"}]).To(Equal(1.0))
		Expect(powerMetrics[powerKey{server: "server2", powerState: "Off"}]).To(Equal(0.0))

		// server3 is Off
		Expect(powerMetrics[powerKey{server: "server3", powerState: "On"}]).To(Equal(0.0))
		Expect(powerMetrics[powerKey{server: "server3", powerState: "Off"}]).To(Equal(1.0))

		// Total: 3 servers × 5 power states = 15 power state metrics
		Expect(powerMetrics).To(HaveLen(15))
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

		// Parse metrics and verify per-server condition metrics with all labels
		type conditionKey struct {
			server, conditionType, status string
		}
		conditionMetrics := make(map[conditionKey]int)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil {
				var key conditionKey
				for _, label := range dto.GetLabel() {
					switch label.GetName() {
					case labelServer:
						key.server = label.GetValue()
					case "condition_type":
						key.conditionType = label.GetValue()
					case "status":
						key.status = label.GetValue()
					}
				}
				if key.conditionType != "" && key.status != "" {
					// Each per-server metric has value 1
					Expect(dto.GetGauge().GetValue()).To(Equal(1.0))
					conditionMetrics[key]++
				}
			}
		}

		// Assert exact per-server condition metrics
		Expect(conditionMetrics).To(Equal(map[conditionKey]int{
			{server: "server1", conditionType: "Ready", status: "True"}:      1,
			{server: "server1", conditionType: "Discovered", status: "True"}: 1,
		}))
	})

	It("should emit all enum states with value 0 when server has no state set", func(ctx SpecContext) {
		// Create server without state set
		server := metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{Name: "server1"},
			Spec:       metalv1alpha1.ServerSpec{SystemUUID: "uuid1"},
			Status:     metalv1alpha1.ServerStatus{},
		}

		Expect(fakeClient.Create(ctx, &server)).To(Succeed())

		// Collect metrics - should not panic and should emit all states with value 0
		ch := make(chan prometheus.Metric, 100)
		Expect(func() {
			collector.Collect(ch)
			close(ch)
		}).NotTo(Panic())

		// Parse metrics and verify all states are emitted with value 0
		type stateKey struct {
			server, state string
		}
		stateMetrics := make(map[stateKey]float64)
		for metric := range ch {
			var m = metric
			dto := &prometheus_io.Metric{}
			Expect(m.Write(dto)).To(Succeed())

			if dto.GetGauge() != nil {
				var key stateKey
				for _, label := range dto.GetLabel() {
					switch label.GetName() {
					case labelServer:
						key.server = label.GetValue()
					case "state":
						key.state = label.GetValue()
					}
				}
				if key.state != "" {
					stateMetrics[key] = dto.GetGauge().GetValue()
				}
			}
		}

		// All states should be 0 since no state is set
		Expect(stateMetrics[stateKey{server: "server1", state: "Initial"}]).To(Equal(0.0))
		Expect(stateMetrics[stateKey{server: "server1", state: "Available"}]).To(Equal(0.0))
		Expect(stateMetrics).To(HaveLen(6)) // All 6 states emitted
	})
})

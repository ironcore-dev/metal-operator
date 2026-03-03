// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ServerReconciliationTotal tracks reconciliation operations by result
	ServerReconciliationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "metal_server_reconciliation_total",
			Help: "Total number of server reconciliations by result",
		},
		[]string{"result"},
	)
)

func init() {
	// Register counter metrics with controller-runtime's global registry
	metrics.Registry.MustRegister(
		ServerReconciliationTotal,
	)
}

// ServerStateCollector implements prometheus.Collector to provide accurate
// server state counts by listing all servers on each scrape.
type ServerStateCollector struct {
	Client client.Client

	stateDesc      *prometheus.Desc
	powerStateDesc *prometheus.Desc
	conditionDesc  *prometheus.Desc
}

// NewServerStateCollector creates a new ServerStateCollector with the given client.
func NewServerStateCollector(c client.Client) *ServerStateCollector {
	return &ServerStateCollector{
		Client: c,
		stateDesc: prometheus.NewDesc(
			"metal_server_state",
			"Current count of servers in each state",
			[]string{"state"},
			nil,
		),
		powerStateDesc: prometheus.NewDesc(
			"metal_server_power_state",
			"Current count of servers in each power state",
			[]string{"power_state"},
			nil,
		),
		conditionDesc: prometheus.NewDesc(
			"metal_server_condition_status",
			"Count of servers with each condition status",
			[]string{"condition_type", "status"},
			nil,
		),
	}
}

// Describe sends the descriptors of metrics collected by this Collector
// to the provided channel.
func (c *ServerStateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.stateDesc
	ch <- c.powerStateDesc
	ch <- c.conditionDesc
}

// Collect is called by Prometheus when collecting metrics.
// It queries all servers and emits aggregated metrics.
func (c *ServerStateCollector) Collect(ch chan<- prometheus.Metric) {
	// Add 5-second timeout for metrics collection to prevent hanging scrapes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	servers := &metalv1alpha1.ServerList{}

	// List all servers in the cluster
	if err := c.Client.List(ctx, servers); err != nil {
		// Log error but don't block metric collection
		// The metrics endpoint will continue to work
		return
	}

	// Count servers by state
	stateCounts := make(map[string]float64)
	powerStateCounts := make(map[string]float64)
	conditionCounts := make(map[string]map[string]float64)

	for _, server := range servers.Items {
		// Count by server state
		if server.Status.State != "" {
			stateCounts[string(server.Status.State)]++
		}

		// Count by power state
		if server.Status.PowerState != "" {
			powerStateCounts[string(server.Status.PowerState)]++
		}

		// Count by condition status
		for _, condition := range server.Status.Conditions {
			conditionType := condition.Type
			status := string(condition.Status)

			if conditionCounts[conditionType] == nil {
				conditionCounts[conditionType] = make(map[string]float64)
			}
			conditionCounts[conditionType][status]++
		}
	}

	// Emit state metrics
	for state, count := range stateCounts {
		ch <- prometheus.MustNewConstMetric(
			c.stateDesc,
			prometheus.GaugeValue,
			count,
			state,
		)
	}

	// Emit power state metrics
	for powerState, count := range powerStateCounts {
		ch <- prometheus.MustNewConstMetric(
			c.powerStateDesc,
			prometheus.GaugeValue,
			count,
			powerState,
		)
	}

	// Emit condition metrics with server counts
	for conditionType, statusMap := range conditionCounts {
		for status, count := range statusMap {
			ch <- prometheus.MustNewConstMetric(
				c.conditionDesc,
				prometheus.GaugeValue,
				count,
				conditionType,
				status,
			)
		}
	}
}

// Verify that ServerStateCollector implements prometheus.Collector
var _ prometheus.Collector = &ServerStateCollector{}

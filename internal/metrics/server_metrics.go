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

// AllServerStates defines all possible server states for enum metrics
var AllServerStates = []metalv1alpha1.ServerState{
	metalv1alpha1.ServerStateInitial,
	metalv1alpha1.ServerStateDiscovery,
	metalv1alpha1.ServerStateAvailable,
	metalv1alpha1.ServerStateReserved,
	metalv1alpha1.ServerStateError,
	metalv1alpha1.ServerStateMaintenance,
}

// AllServerPowerStates defines all possible server power states for enum metrics
var AllServerPowerStates = []metalv1alpha1.ServerPowerState{
	metalv1alpha1.ServerOnPowerState,
	metalv1alpha1.ServerOffPowerState,
	metalv1alpha1.ServerPausedPowerState,
	metalv1alpha1.ServerPoweringOnPowerState,
	metalv1alpha1.ServerPoweringOffPowerState,
}

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
			"Server state as enum metric (1 for current state, 0 for others)",
			[]string{"server", "state"},
			nil,
		),
		powerStateDesc: prometheus.NewDesc(
			"metal_server_power_state",
			"Server power state as enum metric (1 for current state, 0 for others)",
			[]string{"server", "power_state"},
			nil,
		),
		conditionDesc: prometheus.NewDesc(
			"metal_server_condition_status",
			"Current condition status of a server (value is always 1)",
			[]string{"server", "condition_type", "status"},
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

// Collect is called by Prometheus when scraping metrics.
// It queries all Server resources and emits enum metrics for state and power state
// (1 for current state, 0 for all other states), plus condition metrics.
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

	for _, server := range servers.Items {
		serverName := server.Name

		// Emit enum metrics for all possible states (1 for current, 0 for others)
		for _, state := range AllServerStates {
			value := 0.0
			if server.Status.State == state {
				value = 1.0
			}
			ch <- prometheus.MustNewConstMetric(
				c.stateDesc,
				prometheus.GaugeValue,
				value,
				serverName,
				string(state),
			)
		}

		// Emit enum metrics for all possible power states (1 for current, 0 for others)
		for _, powerState := range AllServerPowerStates {
			value := 0.0
			if server.Status.PowerState == powerState {
				value = 1.0
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerStateDesc,
				prometheus.GaugeValue,
				value,
				serverName,
				string(powerState),
			)
		}

		// Emit per-server condition metrics
		for _, condition := range server.Status.Conditions {
			ch <- prometheus.MustNewConstMetric(
				c.conditionDesc,
				prometheus.GaugeValue,
				1,
				serverName,
				condition.Type,
				string(condition.Status),
			)
		}
	}
}

// Verify that ServerStateCollector implements prometheus.Collector
var _ prometheus.Collector = &ServerStateCollector{}

// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestCleanupStaleDataRemovesExpiredAlertsAndMetrics(t *testing.T) {
	now := time.Now()
	alertKeyFresh := EventKey{Source: "node-1", Severity: "Warning", EventID: "FAN001", Component: "Fan1"}
	alertKeyStale := EventKey{Source: "node-2", Severity: "Critical", EventID: "TEMP001", Component: "CPU1"}

	collector := &RedfishEventCollector{
		lastReadings: map[string]MetricEntry{
			"fresh": {Timestamp: now.Add(-staleMetricTTL + time.Minute)},
			"stale": {Timestamp: now.Add(-staleMetricTTL - time.Minute)},
		},
		alertCounts: map[EventKey]AlertEntry{
			alertKeyFresh: {Count: 2, LastSeen: now.Add(-staleAlertTTL + time.Hour)},
			alertKeyStale: {Count: 3, LastSeen: now.Add(-staleAlertTTL - time.Hour)},
		},
	}

	collector.cleanupStaleData()

	if _, ok := collector.lastReadings["fresh"]; !ok {
		t.Fatalf("expected fresh metric reading to remain")
	}
	if _, ok := collector.lastReadings["stale"]; ok {
		t.Fatalf("expected stale metric reading to be removed")
	}
	if _, ok := collector.alertCounts[alertKeyFresh]; !ok {
		t.Fatalf("expected fresh alert to remain")
	}
	if _, ok := collector.alertCounts[alertKeyStale]; ok {
		t.Fatalf("expected stale alert to be removed")
	}
}

func TestUpdateFromEventRefreshesAlertLastSeenAndCount(t *testing.T) {
	staleTime := time.Now().Add(-staleAlertTTL - time.Hour)
	key := EventKey{Source: "node-1", Severity: "Critical", EventID: "TEMP001", Component: "CPU1"}

	collector := &RedfishEventCollector{
		lastReadings: map[string]MetricEntry{},
		alertCounts: map[EventKey]AlertEntry{
			key: {Count: 4, LastSeen: staleTime},
		},
	}

	collector.UpdateFromEvent("node-1", EventData{
		Events: []Event{{
			EventID:           "TEMP001",
			Severity:          "Critical",
			OriginOfCondition: "/redfish/v1/Chassis/1/Thermal#/Temperatures/CPU1",
		}},
	})

	entry, ok := collector.alertCounts[key]
	if !ok {
		t.Fatalf("expected alert entry to exist after update")
	}
	if entry.Count != 5 {
		t.Fatalf("expected alert count to increment to 5, got %d", entry.Count)
	}
	if !entry.LastSeen.After(staleTime) {
		t.Fatalf("expected alert lastSeen to be refreshed")
	}
}

func TestCriticalEventHandlerBlocksUntilCapacityAvailable(t *testing.T) {
	// Create a collector with a semaphore capacity of 2
	collector := &RedfishEventCollector{
		lastReadings: make(map[string]MetricEntry),
		alertCounts:  make(map[EventKey]AlertEntry),
		log:          logr.Discard(),
		eventSem:     make(chan struct{}, 2),
	}

	var handledCount atomic.Int32
	var wg sync.WaitGroup

	// Set up a handler that blocks for 100ms
	collector.SetCriticalEventHandler(func(ctx context.Context, bmcName string, event Event) {
		handledCount.Add(1)
		time.Sleep(100 * time.Millisecond)
		wg.Done()
	})

	// Send 5 critical events - all should be queued and eventually processed
	numEvents := 5
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		collector.UpdateFromEvent("test-bmc", EventData{
			Events: []Event{{
				EventID:           "CRIT001",
				Severity:          "Critical",
				Message:           "Test critical event",
				OriginOfCondition: "/redfish/v1/Systems/1",
			}},
		})
	}

	// Wait for all events to be processed (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - all events were processed
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for events to be processed")
	}

	// Verify all 5 events were handled (none were dropped)
	if count := handledCount.Load(); count != int32(numEvents) {
		t.Fatalf("expected all %d events to be handled, but only %d were processed", numEvents, count)
	}
}

func TestCriticalEventHandlerWithSlowHandler(t *testing.T) {
	// Create a collector with a semaphore capacity of 1
	collector := &RedfishEventCollector{
		lastReadings: make(map[string]MetricEntry),
		alertCounts:  make(map[EventKey]AlertEntry),
		log:          logr.Discard(),
		eventSem:     make(chan struct{}, 1),
	}

	var processOrder []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Set up a handler that records the order of processing
	collector.SetCriticalEventHandler(func(ctx context.Context, bmcName string, event Event) {
		mu.Lock()
		processOrder = append(processOrder, len(processOrder)+1)
		mu.Unlock()
		time.Sleep(50 * time.Millisecond) // Simulate slow processing
		wg.Done()
	})

	// Send 3 critical events rapidly
	numEvents := 3
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		collector.UpdateFromEvent("test-bmc", EventData{
			Events: []Event{{
				EventID:           "CRIT001",
				Severity:          "Critical",
				Message:           "Test critical event",
				OriginOfCondition: "/redfish/v1/Systems/1",
			}},
		})
	}

	// Wait for all events to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for events to be processed")
	}

	// Verify all events were processed in order
	mu.Lock()
	defer mu.Unlock()
	if len(processOrder) != numEvents {
		t.Fatalf("expected %d events to be processed, got %d", numEvents, len(processOrder))
	}
	for i := 0; i < numEvents; i++ {
		if processOrder[i] != i+1 {
			t.Fatalf("expected event %d to be processed in order, got process order: %v", i+1, processOrder)
		}
	}
}

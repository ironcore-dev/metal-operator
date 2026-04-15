// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ironcore-dev/metal-operator/bmc"
)

func TestServerEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ServerEvents Suite")
}

var _ = Describe("RedfishEventCollector", func() {
	var collector *RedfishEventCollector

	BeforeEach(func() {
		collector = &RedfishEventCollector{
			lastReadings:      make(map[string]MetricEntry),
			alertCounts:       make(map[EventKey]uint64),
			metricsSourceType: make(map[string]float64),
			sensorPollCount:   make(map[string]uint64),
			sensorPollErrors:  make(map[string]uint64),
		}
	})

	Describe("UpdateFromSensorPoll", func() {
		Context("when processing sensor data", func() {
			It("should convert sensor data to metric entries", func() {
				hostname := "test-server.example.com"
				sensors := []bmc.Sensor{
					{
						ID:              "CPU1Temp",
						Name:            "CPU 1 Temperature",
						Reading:         65.5,
						Units:           "Cel",
						State:           "Enabled",
						PhysicalContext: "CPU",
					},
					{
						ID:              "Fan1RPM",
						Name:            "System Fan 1",
						Reading:         3200.0,
						Units:           "RPM",
						State:           "Enabled",
						PhysicalContext: "Fan",
					},
				}

				beforeTime := time.Now()
				collector.UpdateFromSensorPoll(hostname, sensors)
				afterTime := time.Now()

				Expect(collector.lastReadings).To(HaveLen(2))

				// Verify CPU temperature metric
				cpuMetric, ok := collector.lastReadings["CPU1Temp"]
				Expect(ok).To(BeTrue(), "CPU1Temp metric should exist")
				Expect(cpuMetric.Value).To(Equal(65.5))
				Expect(cpuMetric.Type).To(Equal("Gauge"))
				Expect(cpuMetric.Unit).To(Equal("Cel"))
				Expect(cpuMetric.MetricID).To(Equal("CPU1Temp"))
				Expect(cpuMetric.OriginContext).To(Equal("CPU 1 Temperature"))
				Expect(cpuMetric.Source).To(Equal(hostname))
				Expect(cpuMetric.Timestamp).To(BeTemporally(">=", beforeTime))
				Expect(cpuMetric.Timestamp).To(BeTemporally("<=", afterTime))

				// Verify fan metric
				fanMetric, ok := collector.lastReadings["Fan1RPM"]
				Expect(ok).To(BeTrue(), "Fan1RPM metric should exist")
				Expect(fanMetric.Value).To(Equal(3200.0))
				Expect(fanMetric.Unit).To(Equal("RPM"))
				Expect(fanMetric.Source).To(Equal(hostname))
			})

			It("should determine metric type from PhysicalContext", func() {
				testCases := []struct {
					context      string
					expectedType string
				}{
					{"CPUTemp", "Temperature"},
					{"SystemTemp", "Temperature"},
					{"PowerSupply", "Power"},
					{"PowerControl", "Power"},
					{"VoltageRegulator", "Voltage"},
					{"12VVoltage", "Voltage"},
					{"Fan", "Gauge"},
					{"Memory", "Gauge"},
				}

				for _, tc := range testCases {
					sensors := []bmc.Sensor{
						{
							ID:              "test-" + tc.context,
							Name:            "Test Sensor",
							Reading:         100.0,
							Units:           "Unit",
							PhysicalContext: tc.context,
						},
					}

					collector.UpdateFromSensorPoll("test-host", sensors)

					metric := collector.lastReadings["test-"+tc.context]
					Expect(metric.Type).To(Equal(tc.expectedType),
						"Context %s should result in type %s", tc.context, tc.expectedType)
				}
			})

			It("should handle multiple hosts separately", func() {
				host1Sensors := []bmc.Sensor{
					{
						ID:      "Sensor1",
						Name:    "Host1 Sensor",
						Reading: 50.0,
						Units:   "Unit",
					},
				}

				host2Sensors := []bmc.Sensor{
					{
						ID:      "Sensor1",
						Name:    "Host2 Sensor",
						Reading: 75.0,
						Units:   "Unit",
					},
				}

				collector.UpdateFromSensorPoll("host1.example.com", host1Sensors)
				collector.UpdateFromSensorPoll("host2.example.com", host2Sensors)

				// Both sensors with same ID should exist but with different sources
				metric := collector.lastReadings["Sensor1"]
				// The last update wins (host2)
				Expect(metric.Source).To(Equal("host2.example.com"))
				Expect(metric.Value).To(Equal(75.0))
			})

			It("should update existing sensor readings", func() {
				hostname := "test-server.example.com"
				initialSensors := []bmc.Sensor{
					{
						ID:      "TempSensor",
						Name:    "Temperature",
						Reading: 60.0,
						Units:   "Cel",
					},
				}

				collector.UpdateFromSensorPoll(hostname, initialSensors)
				firstTimestamp := collector.lastReadings["TempSensor"].Timestamp

				time.Sleep(10 * time.Millisecond)

				updatedSensors := []bmc.Sensor{
					{
						ID:      "TempSensor",
						Name:    "Temperature",
						Reading: 65.0,
						Units:   "Cel",
					},
				}

				collector.UpdateFromSensorPoll(hostname, updatedSensors)
				secondTimestamp := collector.lastReadings["TempSensor"].Timestamp

				metric := collector.lastReadings["TempSensor"]
				Expect(metric.Value).To(Equal(65.0), "Value should be updated")
				Expect(secondTimestamp).To(BeTemporally(">", firstTimestamp), "Timestamp should be updated")
			})

			It("should handle empty sensor list", func() {
				hostname := "test-server.example.com"
				collector.UpdateFromSensorPoll(hostname, []bmc.Sensor{})

				Expect(collector.lastReadings).To(BeEmpty())
			})
		})
	})

	Describe("GetLastUpdateTime", func() {
		Context("when metrics exist for hostname", func() {
			It("should return the most recent timestamp", func() {
				hostname := "test-server.example.com"

				// Add metrics at different times
				now := time.Now()
				collector.lastReadings["Sensor1"] = MetricEntry{
					Source:    hostname,
					Timestamp: now.Add(-5 * time.Minute),
				}
				collector.lastReadings["Sensor2"] = MetricEntry{
					Source:    hostname,
					Timestamp: now.Add(-2 * time.Minute),
				}
				collector.lastReadings["Sensor3"] = MetricEntry{
					Source:    hostname,
					Timestamp: now.Add(-10 * time.Minute),
				}

				lastUpdate := collector.GetLastUpdateTime(hostname)
				Expect(lastUpdate).To(BeTemporally("~", now.Add(-2*time.Minute), time.Second))
			})

			It("should only consider metrics from the specified hostname", func() {
				host1 := "host1.example.com"
				host2 := "host2.example.com"

				now := time.Now()
				collector.lastReadings["Host1Sensor"] = MetricEntry{
					Source:    host1,
					Timestamp: now.Add(-5 * time.Minute),
				}
				collector.lastReadings["Host2Sensor"] = MetricEntry{
					Source:    host2,
					Timestamp: now.Add(-1 * time.Minute),
				}

				lastUpdate := collector.GetLastUpdateTime(host1)
				Expect(lastUpdate).To(BeTemporally("~", now.Add(-5*time.Minute), time.Second))
			})

			It("should match hostname by key prefix", func() {
				hostname := "test-server.example.com"
				now := time.Now()

				// Simulate metrics with hostname-prefixed keys
				collector.lastReadings[hostname+"_Sensor1"] = MetricEntry{
					Source:    hostname,
					Timestamp: now.Add(-3 * time.Minute),
				}

				lastUpdate := collector.GetLastUpdateTime(hostname)
				Expect(lastUpdate).To(BeTemporally("~", now.Add(-3*time.Minute), time.Second))
			})
		})

		Context("when no metrics exist for hostname", func() {
			It("should return zero time", func() {
				hostname := "unknown-server.example.com"

				// Add metrics for a different host
				collector.lastReadings["OtherSensor"] = MetricEntry{
					Source:    "other-host.example.com",
					Timestamp: time.Now(),
				}

				lastUpdate := collector.GetLastUpdateTime(hostname)
				Expect(lastUpdate.IsZero()).To(BeTrue())
			})

			It("should return zero time when collector is empty", func() {
				hostname := "test-server.example.com"
				lastUpdate := collector.GetLastUpdateTime(hostname)

				Expect(lastUpdate.IsZero()).To(BeTrue())
			})
		})
	})

	Describe("Thread Safety", func() {
		It("should handle concurrent UpdateFromSensorPoll calls", func(ctx SpecContext) {
			done := make(chan bool)

			// Spawn multiple goroutines updating metrics
			for i := 0; i < 10; i++ {
				go func(id int) {
					sensors := []bmc.Sensor{
						{
							ID:      "ConcurrentSensor",
							Name:    "Concurrent Test",
							Reading: float64(id),
							Units:   "Unit",
						},
					}
					collector.UpdateFromSensorPoll("concurrent-host", sensors)
					done <- true
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < 10; i++ {
				Eventually(done).Should(Receive())
			}

			// Verify that at least one update succeeded
			_, exists := collector.lastReadings["ConcurrentSensor"]
			Expect(exists).To(BeTrue())
		}, SpecTimeout(5*time.Second))

		It("should handle concurrent GetLastUpdateTime calls", func(ctx SpecContext) {
			hostname := "test-server.example.com"
			collector.lastReadings["TestSensor"] = MetricEntry{
				Source:    hostname,
				Timestamp: time.Now(),
			}

			done := make(chan bool)

			// Spawn multiple goroutines reading the timestamp
			for i := 0; i < 10; i++ {
				go func() {
					lastUpdate := collector.GetLastUpdateTime(hostname)
					Expect(lastUpdate.IsZero()).To(BeFalse())
					done <- true
				}()
			}

			// Wait for all goroutines to complete
			for i := 0; i < 10; i++ {
				Eventually(done).Should(Receive())
			}
		}, SpecTimeout(5*time.Second))
	})
})

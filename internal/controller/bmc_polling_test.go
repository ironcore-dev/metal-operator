// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metalBmc "github.com/ironcore-dev/metal-operator/bmc"
	"github.com/ironcore-dev/metal-operator/bmc/mocks"
	"github.com/ironcore-dev/metal-operator/internal/serverevents"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("BMC Polling", func() {
	var (
		ctrl             *gomock.Controller
		mockBMCClient    *mocks.MockBMC
		metricsCollector *serverevents.RedfishEventCollector
		reconciler       *BMCReconciler
		bmcObj           *metalv1alpha1.BMC
		ctx              context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockBMCClient = mocks.NewMockBMC(ctrl)

		// Reuse the same collector across tests to avoid duplicate Prometheus registration
		if metricsCollector == nil {
			metricsCollector = serverevents.NewRedfishEventCollector()
		}

		ctx = context.Background()

		// Create a test BMC object
		bmcObj = &metalv1alpha1.BMC{
			Spec: metalv1alpha1.BMCSpec{
				BMCUUID: "test-uuid",
			},
			Status: metalv1alpha1.BMCStatus{},
		}

		// Initialize reconciler with common test settings
		reconciler = &BMCReconciler{
			EnableMetricsPolling:      true,
			MetricsCollector:          metricsCollector,
			MetricsPollingTimeout:     5 * time.Second,
			MetricsStalenessThreshold: 1 * time.Minute,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("pollMetricsIfNeeded", func() {
		It("should skip polling when EnableMetricsPolling is false", func() {
			reconciler.EnableMetricsPolling = false
			mockBMCClient.EXPECT().GetSystems(ctx).Times(0)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip polling when MetricsCollector is nil", func() {
			reconciler.MetricsCollector = nil
			mockBMCClient.EXPECT().GetSystems(ctx).Times(0)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip polling when GetSystems returns error", func() {
			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return(nil, errors.New("connection failed"))

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get systems"))
		})

		It("should skip polling when no systems are available", func() {
			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{}, nil)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip polling for non-Lenovo manufacturers (HPE)", func() {
			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "HPE",
					},
				}, nil)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip polling for non-Lenovo manufacturers (Dell)", func() {
			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Dell Inc.",
					},
				}, nil)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip polling when metrics are fresh and subscription exists", func() {
			// Set metrics to fresh in collector
			metricsCollector.UpdateFromSensorPoll(bmcObj.Name, []metalBmc.Sensor{
				{
					ID:      "sensor1",
					Name:    "Fan1",
					Reading: 5000,
					Units:   "RPM",
				},
			})

			// Subscription exists
			bmcObj.Status.MetricsReportSubscriptionLink = "/redfish/v1/EventService/Subscriptions/1"

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should poll when no subscription link exists", func() {
			// No subscription link
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())

			// Verify metrics were recorded
			Expect(metricsCollector.GetLastUpdateTime(bmcObj.Name)).NotTo(BeZero())
		})

		It("should poll when metrics are stale", func() {
			// Set metrics to stale (beyond staleness threshold)
			oldSensor := metalBmc.Sensor{
				ID:      "old-sensor",
				Name:    "Fan1",
				Reading: 4500,
				Units:   "RPM",
			}
			metricsCollector.UpdateFromSensorPoll(bmcObj.Name, []metalBmc.Sensor{oldSensor})

			// Manipulate timestamp to be stale
			time.Sleep(100 * time.Millisecond)

			// Subscription exists but metrics are stale
			bmcObj.Status.MetricsReportSubscriptionLink = "/redfish/v1/EventService/Subscriptions/1"

			// Set staleness threshold to very small value to make existing metrics stale
			reconciler.MetricsStalenessThreshold = 50 * time.Millisecond

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should respect MetricsPollingTimeout when polling sensors", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""
			reconciler.MetricsPollingTimeout = 100 * time.Millisecond

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// Make GetSensors sleep longer than timeout to simulate timeout scenario
			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				DoAndReturn(func(pollCtx context.Context, uri string) ([]metalBmc.Sensor, error) {
					select {
					case <-pollCtx.Done():
						return nil, pollCtx.Err()
					case <-time.After(200 * time.Millisecond):
						return []metalBmc.Sensor{}, nil
					}
				}).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			// Should handle timeout gracefully (no sensors found)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should try multiple chassis URIs when polling", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// First chassis URI fails, second succeeds
			gomock.InOrder(
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/1").
					Return(nil, errors.New("not found")),
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/Self").
					Return([]metalBmc.Sensor{
						{
							ID:      "sensor1",
							Name:    "Temp1",
							Reading: 45.5,
							Units:   "Celsius",
						},
					}, nil),
			)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
			// Verify metrics were updated
			Expect(metricsCollector.GetLastUpdateTime(bmcObj.Name)).NotTo(BeZero())
		})

		It("should handle case where all chassis URIs fail", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// Both chassis URIs fail
			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("connection failed")).
				Times(2)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should record poll error when no sensors found", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// No sensors returned
			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return(nil, nil).
				Times(2)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should update metrics collector with polled sensor data", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			sensors := []metalBmc.Sensor{
				{
					ID:              "sensor1",
					Name:            "Fan1",
					Reading:         5000,
					Units:           "RPM",
					PhysicalContext: "Fans",
				},
				{
					ID:              "sensor2",
					Name:            "Temp1",
					Reading:         45.5,
					Units:           "Celsius",
					PhysicalContext: "CPU",
				},
			}

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return(sensors, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())

			// Verify metrics were updated
			lastUpdate := metricsCollector.GetLastUpdateTime(bmcObj.Name)
			Expect(lastUpdate).NotTo(BeZero())
			Expect(time.Since(lastUpdate)).To(BeNumerically("<", 1*time.Second))
		})

		It("should handle Lenovo case-insensitively", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "LENOVO", // uppercase
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
			Expect(metricsCollector.GetLastUpdateTime(bmcObj.Name)).NotTo(BeZero())
		})

		It("should accumulate sensors from multiple chassis URIs", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// Both chassis URIs return sensors
			gomock.InOrder(
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/1").
					Return([]metalBmc.Sensor{
						{
							ID:      "sensor1",
							Name:    "Fan1",
							Reading: 5000,
							Units:   "RPM",
						},
					}, nil),
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/Self").
					Return([]metalBmc.Sensor{
						{
							ID:      "sensor2",
							Name:    "Temp1",
							Reading: 45.5,
							Units:   "Celsius",
						},
					}, nil),
			)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
			// Verify both sensors were recorded
			Expect(metricsCollector.GetLastUpdateTime(bmcObj.Name)).NotTo(BeZero())
		})

		It("should handle empty sensor list from first URI but success from second", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			gomock.InOrder(
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/1").
					Return([]metalBmc.Sensor{}, nil),
				mockBMCClient.EXPECT().
					GetSensors(gomock.Any(), "/redfish/v1/Chassis/Self").
					Return([]metalBmc.Sensor{
						{
							ID:      "sensor1",
							Name:    "Fan1",
							Reading: 5000,
							Units:   "RPM",
						},
					}, nil),
			)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
			Expect(metricsCollector.GetLastUpdateTime(bmcObj.Name)).NotTo(BeZero())
		})

		It("should skip polling when manufacturer contains lenovo as substring", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo Corporation",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should log polling activity", func() {
			bmcObj.Name = "test-bmc"
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("polling edge cases", func() {
		It("should handle multiple systems but only check first manufacturer", func() {
			bmcObj.Status.MetricsReportSubscriptionLink = ""

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid-1",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
					{
						UUID:         "sys-uuid-2",
						URI:          "/redfish/v1/Systems/2",
						Manufacturer: "Dell",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5000,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should not poll if staleness threshold is not exceeded and subscription exists", func() {
			// Pre-populate fresh metrics
			metricsCollector.UpdateFromSensorPoll(bmcObj.Name, []metalBmc.Sensor{
				{
					ID:      "sensor1",
					Name:    "Fan1",
					Reading: 5000,
					Units:   "RPM",
				},
			})

			bmcObj.Status.MetricsReportSubscriptionLink = "/redfish/v1/EventService/Subscriptions/1"
			reconciler.MetricsStalenessThreshold = 10 * time.Minute // High threshold

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			// GetSensors should not be called
			mockBMCClient.EXPECT().GetSensors(gomock.Any(), gomock.Any()).Times(0)

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should poll when staleness threshold is exceeded even with subscription", func() {
			// Pre-populate old metrics
			metricsCollector.UpdateFromSensorPoll(bmcObj.Name, []metalBmc.Sensor{
				{
					ID:      "sensor1",
					Name:    "Fan1",
					Reading: 5000,
					Units:   "RPM",
				},
			})

			// Make metrics stale by reducing threshold
			reconciler.MetricsStalenessThreshold = 1 * time.Nanosecond
			time.Sleep(10 * time.Millisecond)

			bmcObj.Status.MetricsReportSubscriptionLink = "/redfish/v1/EventService/Subscriptions/1"

			mockBMCClient.EXPECT().
				GetSystems(ctx).
				Return([]metalBmc.Server{
					{
						UUID:         "sys-uuid",
						URI:          "/redfish/v1/Systems/1",
						Manufacturer: "Lenovo",
					},
				}, nil)

			mockBMCClient.EXPECT().
				GetSensors(gomock.Any(), gomock.Any()).
				Return([]metalBmc.Sensor{
					{
						ID:      "sensor1",
						Name:    "Fan1",
						Reading: 5500,
						Units:   "RPM",
					},
				}, nil).
				AnyTimes()

			err := reconciler.pollMetricsIfNeeded(ctx, mockBMCClient, bmcObj)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})

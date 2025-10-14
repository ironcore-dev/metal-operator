// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/redfish"
)

func SubscribeMetricsReport(ctx context.Context, vendor, hostname string, bmcClient bmc.BMC) error {
	if err := bmcClient.CreateEventSubscription(
		ctx,
		fmt.Sprintf("https://localhost:8888/%s/%s/metrics", vendor, hostname),
		redfish.MetricReportEventFormatType,
		redfish.TerminateAfterRetriesDeliveryRetryPolicy,
	); err != nil {
		return fmt.Errorf("failed to create event subscription: %w", err)
	}
	return nil
}

func UnsubscribeMetricsReport(ctx context.Context, vendor, hostname string, bmcClient bmc.BMC) error {
	if err := bmcClient.DeleteEventSubscription(ctx, fmt.Sprintf("https://localhost:8888/%s/%s/metrics", vendor, hostname)); err != nil {
		return fmt.Errorf("failed to delete event subscription: %w", err)
	}
	return nil
}

func SubscribeEvents(ctx context.Context, vendor, hostname string, bmcClient bmc.BMC) error {
	if err := bmcClient.CreateEventSubscription(
		ctx,
		fmt.Sprintf("https://localhost:8888/%s/%s/alerts", vendor, hostname),
		redfish.EventEventFormatType,
		redfish.TerminateAfterRetriesDeliveryRetryPolicy,
	); err != nil {
		return fmt.Errorf("failed to create alert subscription: %w", err)
	}
	return nil
}
func UnsubscribeEvents(ctx context.Context, vendor, hostname string, bmcClient bmc.BMC) error {
	if err := bmcClient.DeleteEventSubscription(ctx, fmt.Sprintf("https://localhost:8888/%s/%s/alerts", vendor, hostname)); err != nil {
		return fmt.Errorf("failed to delete alert subscription: %w", err)
	}
	return nil
}

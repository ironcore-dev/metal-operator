// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/redfish"
)

// SubscribeMetricsReport subscribes to Redfish metric reporting events for the given hostname and callback URL.
func SubscribeMetricsReport(ctx context.Context, url, hostname string, bmcClient bmc.BMC) (string, error) {
	link, err := bmcClient.CreateEventSubscription(
		ctx,
		fmt.Sprintf("%s/%s/metrics", url, hostname),
		redfish.MetricReportEventFormatType,
		redfish.TerminateAfterRetriesDeliveryRetryPolicy,
	)
	if err != nil {
		return link, fmt.Errorf("failed to create event subscription: %w", err)
	}
	return link, nil
}

// SubscribeEvents creates a Redfish event subscription for events.
func SubscribeEvents(ctx context.Context, url, hostname string, bmcClient bmc.BMC) (string, error) {
	link, err := bmcClient.CreateEventSubscription(
		ctx,
		fmt.Sprintf("%s/%s/alerts", url, hostname),
		redfish.EventEventFormatType,
		redfish.TerminateAfterRetriesDeliveryRetryPolicy,
	)
	if err != nil {
		return link, fmt.Errorf("failed to create alert subscription: %w", err)
	}
	return link, nil
}

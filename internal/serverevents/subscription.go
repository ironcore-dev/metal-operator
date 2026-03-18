// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package serverevents

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/bmc"
	"github.com/stmcginnis/gofish/schemas"
)

// SubscribeMetricsReport subscribes to sRedfish metric reporting events for the given hostname and callback URL.
func SubscribeMetricsReport(ctx context.Context, url, hostname string, bmcClient bmc.BMC) (string, error) {
	link, err := bmcClient.CreateEventSubscription(
		ctx,
		fmt.Sprintf("%s/serverevents/metricsreport/%s", url, hostname),
		schemas.MetricReportEventFormatType,
		schemas.TerminateAfterRetriesDeliveryRetryPolicy,
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
		fmt.Sprintf("%s/serverevents/alerts/%s", url, hostname),
		schemas.EventEventFormatType,
		schemas.TerminateAfterRetriesDeliveryRetryPolicy,
	)
	if err != nil {
		return link, fmt.Errorf("failed to create alert subscription: %w", err)
	}
	return link, nil
}

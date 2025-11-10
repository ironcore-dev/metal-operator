// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"context"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
)

func collectLLDPInfo(ctx context.Context, interval, duration time.Duration) (registry.LLDP, error) {
	return registry.LLDP{
		Interfaces: []registry.LLDPInterface{
			{
				Name: "en0",
				Neighbors: []registry.Neighbor{
					{
						ChassisID:         "00:11:22:33:44:55",
						PortID:            "1",
						PortDescription:   "Uplink Port",
						SystemName:        "Switch-01",
						SystemDescription: "Example Switch Model",
						Capabilities:      []string{"bridge", "router"},
					},
				},
			},
		},
	}, nil
}

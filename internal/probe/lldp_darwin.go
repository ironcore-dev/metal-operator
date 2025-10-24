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
				InterfaceIndex:            0,
				InterfaceName:             "en0",
				InterfaceAlternativeNames: []string{"ethernet0"},
				Neighbors: []registry.Neighbor{
					{
						ChassisID:           "00:11:22:33:44:55",
						RawChassisID:        []int{0, 17, 34, 51, 68, 85},
						PortID:              "1",
						RawPortID:           []int{49},
						PortDescription:     "Uplink Port",
						SystemName:          "Switch-01",
						SystemDescription:   "Example Switch Model",
						EnabledCapabilities: 0x14,
					},
				},
			},
		},
	}, nil
}

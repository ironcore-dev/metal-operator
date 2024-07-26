// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishLocalBMC)(nil)

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(
	ctx context.Context,
	endpoint, username, password string,
	basicAuth bool,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, endpoint, username, password, basicAuth)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r RedfishLocalBMC) PowerOn(systemUUID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
			system.PowerState = redfish.OnPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OnPowerState, systemUUID, err)
			}
			break
		}
	}
	return nil
}

func (r RedfishLocalBMC) PowerOff(systemUUID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
			system.PowerState = redfish.OffPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OffPowerState, systemUUID, err)
			}
			break
		}
	}
	return nil
}

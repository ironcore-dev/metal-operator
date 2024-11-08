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
	options Options,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r RedfishLocalBMC) PowerOn(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}
	system.PowerState = redfish.OnPowerState
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OnPowerState, systemUUID, err)
	}
	return nil
}

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get system: %w", err)
	}
	system.PowerState = redfish.OffPowerState
	systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
	if err := system.Patch(systemURI, system); err != nil {
		return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OffPowerState, systemUUID, err)
	}
	return nil
}

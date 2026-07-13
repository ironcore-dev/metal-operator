// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"

	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SupermicroRedfishBMC is the Supermicro-specific implementation of the BMC interface.
type SupermicroRedfishBMC struct {
	*RedfishBaseBMC
}

// SetBootOverride sets the boot device to network boot for the next system boot
// on Supermicro hardware. Newer Supermicro BMCs require BootSourceOverrideMode=UEFI
// to be sent explicitly for the override to take effect; older boards (e.g.
// X10-series with Redfish ComputerSystem v1_3_0) don't expose the property at
// all and reject the whole PATCH when it's included.
func (r *SupermicroRedfishBMC) SetBootOverride(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	setBoot := &schemas.Boot{
		BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  schemas.PxeBootSource,
	}
	if system.Boot.BootSourceOverrideMode != "" {
		setBoot.BootSourceOverrideMode = schemas.UEFIBootSourceOverrideMode
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Setting PXE boot once (Supermicro)", "SystemURI", systemURI, "Boot settings", setBoot)
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	return nil
}

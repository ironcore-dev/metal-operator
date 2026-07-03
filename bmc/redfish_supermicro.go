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

// SetBootOverride sets a network-boot override on Supermicro hardware. Newer
// Supermicro BMCs require BootSourceOverrideMode=UEFI to be sent explicitly for
// the override to take effect; older boards (e.g. X10-series with Redfish
// ComputerSystem v1_3_0) don't expose the property at all and reject the whole
// PATCH when it's included.
func (r *SupermicroRedfishBMC) SetBootOverride(ctx context.Context, systemURI string, persistent bool) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	wantEnabled := schemas.OnceBootSourceOverrideEnabled
	if persistent {
		wantEnabled = schemas.ContinuousBootSourceOverrideEnabled
		if system.Boot.BootSourceOverrideEnabled == schemas.ContinuousBootSourceOverrideEnabled &&
			system.Boot.BootSourceOverrideTarget == schemas.PxeBootSource &&
			(system.Boot.BootSourceOverrideMode == "" ||
				system.Boot.BootSourceOverrideMode == schemas.UEFIBootSourceOverrideMode) {
			return nil
		}
	}

	setBoot := &schemas.Boot{
		BootSourceOverrideEnabled: wantEnabled,
		BootSourceOverrideTarget:  schemas.PxeBootSource,
	}
	if system.Boot.BootSourceOverrideMode != "" {
		setBoot.BootSourceOverrideMode = schemas.UEFIBootSourceOverrideMode
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Setting boot override (Supermicro)", "SystemURI", systemURI, "Boot settings", setBoot)
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set boot override: %w", err)
	}
	return nil
}

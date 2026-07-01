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

// SetBootOverride sets a network-boot override on Supermicro hardware, which
// requires explicitly setting BootSourceOverrideMode to UEFI for the override
// to take effect.
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
			system.Boot.BootSourceOverrideMode == schemas.UEFIBootSourceOverrideMode {
			return nil
		}
	}

	setBoot := &schemas.Boot{
		BootSourceOverrideEnabled: wantEnabled,
		BootSourceOverrideMode:    schemas.UEFIBootSourceOverrideMode,
		BootSourceOverrideTarget:  schemas.PxeBootSource,
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Setting boot override (Supermicro)", "SystemURI", systemURI, "Boot settings", setBoot)
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set boot override: %w", err)
	}
	return nil
}

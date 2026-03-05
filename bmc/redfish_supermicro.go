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

// SetPXEBootOnce sets the boot device for the next system boot.
// Supermicro requires explicitly setting BootSourceOverrideMode to UEFI.
func (r *SupermicroRedfishBMC) SetPXEBootOnce(ctx context.Context, systemURI string) error {
	system, err := r.getSystemFromUri(ctx, systemURI)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	setBoot := &schemas.Boot{
		BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
		BootSourceOverrideMode:    schemas.UEFIBootSourceOverrideMode,
		BootSourceOverrideTarget:  schemas.PxeBootSource,
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Setting PXE boot once (Supermicro)", "SystemURI", systemURI, "Boot settings", setBoot)
	if err := system.SetBoot(setBoot); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	return nil
}

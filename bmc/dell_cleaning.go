// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"fmt"
	"io"

	"github.com/stmcginnis/gofish/schemas"
	ctrl "sigs.k8s.io/controller-runtime"
)

// DellCleaning implements cleaning operations for Dell servers
type DellCleaning struct {
	client HTTPClient
}

// NewDellCleaning creates a new DellCleaning instance
func NewDellCleaning(client HTTPClient) *DellCleaning {
	return &DellCleaning{client: client}
}

// EraseDisk performs disk erasing for Dell servers using iDRAC OEM extensions
func (d *DellCleaning) EraseDisk(ctx context.Context, storages []*schemas.Storage, method DiskWipeMethod) error {
	log := ctrl.LoggerFrom(ctx)

	// Dell iDRAC supports secure erase via Storage Controller actions
	for _, storage := range storages {
		drives, err := storage.Drives()
		if err != nil {
			log.Error(err, "Failed to get drives for storage", "storage", storage.Name)
			continue
		}

		for _, drive := range drives {
			// Construct OEM action URI for Dell
			// Dell uses: /redfish/v1/Systems/{id}/Storage/{storageId}/Drives/{driveId}/Actions/Drive.SecureErase
			actionURI := fmt.Sprintf("%s/Actions/Drive.SecureErase", drive.ODataID)

			payload := map[string]any{
				"OverwritePasses": getDellWipePasses(method),
			}

			log.V(1).Info("Initiating Dell drive wipe", "drive", drive.Name, "uri", actionURI)

			resp, err := d.client.Post(actionURI, payload)
			if err != nil {
				log.Error(err, "Failed to initiate disk wipe for drive", "drive", drive.Name)
				continue
			}
			_ = resp.Body.Close()

			if resp.StatusCode >= 300 {
				body, _ := io.ReadAll(resp.Body)
				log.Error(fmt.Errorf("wipe request failed"), "Failed to wipe drive",
					"drive", drive.Name, "status", resp.StatusCode, "body", string(body))
				continue
			}
		}
	}

	return nil
}

func getDellWipePasses(method DiskWipeMethod) int {
	switch method {
	case DiskWipeMethodQuick:
		return 1
	case DiskWipeMethodSecure:
		return 3
	case DiskWipeMethodDoD:
		return 7
	default:
		return 1
	}
}

// ResetBIOS resets BIOS configuration to factory defaults for Dell servers
func (d *DellCleaning) ResetBIOS(ctx context.Context, biosURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// Dell iDRAC: POST to /redfish/v1/Systems/{id}/Bios/Actions/Bios.ResetBios
	actionURI := fmt.Sprintf("%s/Actions/Bios.ResetBios", biosURI)

	log.V(1).Info("Resetting Dell BIOS to defaults", "uri", actionURI)

	resp, err := d.client.Post(actionURI, map[string]any{})
	if err != nil {
		return fmt.Errorf("failed to reset BIOS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BIOS reset failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ResetBMC resets BMC configuration to factory defaults for Dell servers
func (d *DellCleaning) ResetBMC(ctx context.Context, manager *schemas.Manager) error {
	log := ctrl.LoggerFrom(ctx)

	// Dell iDRAC: Use OEM action to reset to defaults
	// /redfish/v1/Managers/{id}/Actions/Oem/DellManager.ResetToDefaults
	actionURI := fmt.Sprintf("%s/Actions/Oem/DellManager.ResetToDefaults", manager.ODataID)

	payload := map[string]any{
		"ResetType": "ResetAllWithRootDefaults",
	}

	log.V(1).Info("Resetting Dell iDRAC to defaults", "uri", actionURI)

	resp, err := d.client.Post(actionURI, payload)
	if err != nil {
		return fmt.Errorf("failed to reset BMC: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BMC reset failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ClearNetworkConfig clears network configuration for Dell servers
func (d *DellCleaning) ClearNetworkConfig(ctx context.Context, systemURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// Dell: Clear network adapters configuration via OEM extensions
	// This typically involves resetting NIC settings to defaults
	actionURI := fmt.Sprintf("%s/NetworkAdapters/Actions/Oem/DellNetworkAdapter.ClearConfiguration", systemURI)

	log.V(1).Info("Clearing Dell network configuration", "uri", actionURI)

	resp, err := d.client.Post(actionURI, map[string]any{})
	if err != nil {
		// Network config clear might not be critical, log and continue
		log.Error(err, "Failed to clear network configuration (non-critical)")
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Error(fmt.Errorf("network config clear failed"), "Failed with status",
			"status", resp.StatusCode, "body", string(body))
	}

	return nil
}

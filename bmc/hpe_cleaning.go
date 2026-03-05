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

// HPECleaning implements cleaning operations for HPE servers
type HPECleaning struct {
	client HTTPClient
}

// NewHPECleaning creates a new HPECleaning instance
func NewHPECleaning(client HTTPClient) *HPECleaning {
	return &HPECleaning{client: client}
}

// EraseDisk performs disk erasing for HPE servers using iLO OEM extensions
func (h *HPECleaning) EraseDisk(ctx context.Context, storages []*schemas.Storage, method DiskWipeMethod) error {
	log := ctrl.LoggerFrom(ctx)

	// HPE iLO supports sanitize operations via OEM extensions
	for _, storage := range storages {
		drives, err := storage.Drives()
		if err != nil {
			log.Error(err, "Failed to get drives for storage", "storage", storage.Name)
			continue
		}

		for _, drive := range drives {
			// HPE OEM action: /redfish/v1/Systems/{id}/Storage/{storageId}/Drives/{driveId}/Actions/Oem/Hpe/HpeDrive.SecureErase
			actionURI := fmt.Sprintf("%s/Actions/Oem/Hpe/HpeDrive.SecureErase", drive.ODataID)

			payload := map[string]any{
				"SanitizeType": getHPEWipeType(method),
			}

			log.V(1).Info("Initiating HPE drive wipe", "drive", drive.Name, "uri", actionURI)

			resp, err := h.client.Post(actionURI, payload)
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

func getHPEWipeType(method DiskWipeMethod) string {
	switch method {
	case DiskWipeMethodQuick:
		return "BlockErase"
	case DiskWipeMethodSecure:
		return "Overwrite"
	case DiskWipeMethodDoD:
		return "CryptographicErase"
	default:
		return "BlockErase"
	}
}

// ResetBIOS resets BIOS configuration to factory defaults for HPE servers
func (h *HPECleaning) ResetBIOS(ctx context.Context, biosURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// HPE iLO: Use ResetBios action
	// /redfish/v1/Systems/{id}/Bios/Actions/Bios.ResetBios
	actionURI := fmt.Sprintf("%s/Actions/Bios.ResetBios", biosURI)

	log.V(1).Info("Resetting HPE BIOS to defaults", "uri", actionURI)

	resp, err := h.client.Post(actionURI, map[string]any{})
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

// ResetBMC resets BMC configuration to factory defaults for HPE servers
func (h *HPECleaning) ResetBMC(ctx context.Context, manager *schemas.Manager) error {
	log := ctrl.LoggerFrom(ctx)

	// HPE iLO: Use OEM action to reset to factory defaults
	// /redfish/v1/Managers/{id}/Actions/Oem/Hpe/HpiLO.ResetToFactoryDefaults
	actionURI := fmt.Sprintf("%s/Actions/Oem/Hpe/HpiLO.ResetToFactoryDefaults", manager.ODataID)

	payload := map[string]any{
		"ResetType": "Default",
	}

	log.V(1).Info("Resetting HPE iLO to defaults", "uri", actionURI)

	resp, err := h.client.Post(actionURI, payload)
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

// ClearNetworkConfig clears network configuration for HPE servers
func (h *HPECleaning) ClearNetworkConfig(ctx context.Context, systemURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// HPE: Clear network adapters configuration
	actionURI := fmt.Sprintf("%s/NetworkAdapters/Actions/Oem/Hpe/HpeNetworkAdapter.ClearConfiguration", systemURI)

	log.V(1).Info("Clearing HPE network configuration", "uri", actionURI)

	resp, err := h.client.Post(actionURI, map[string]any{})
	if err != nil {
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

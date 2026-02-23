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

// LenovoCleaning implements cleaning operations for Lenovo servers
type LenovoCleaning struct {
	client HTTPClient
}

// NewLenovoCleaning creates a new LenovoCleaning instance
func NewLenovoCleaning(client HTTPClient) *LenovoCleaning {
	return &LenovoCleaning{client: client}
}

// EraseDisk performs disk erasing for Lenovo servers using XClarity OEM extensions
func (l *LenovoCleaning) EraseDisk(ctx context.Context, storages []*schemas.Storage, method DiskWipeMethod) error {
	log := ctrl.LoggerFrom(ctx)

	// Lenovo XClarity supports secure erase via OEM extensions
	for _, storage := range storages {
		drives, err := storage.Drives()
		if err != nil {
			log.Error(err, "Failed to get drives for storage", "storage", storage.Name)
			continue
		}

		for _, drive := range drives {
			// Lenovo OEM action path
			actionURI := fmt.Sprintf("%s/Actions/Drive.SecureErase", drive.ODataID)

			payload := map[string]any{
				"EraseMethod": getLenovoWipeMethod(method),
			}

			log.V(1).Info("Initiating Lenovo drive wipe", "drive", drive.Name, "uri", actionURI)

			resp, err := l.client.Post(actionURI, payload)
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

func getLenovoWipeMethod(method DiskWipeMethod) string {
	switch method {
	case DiskWipeMethodQuick:
		return "Simple"
	case DiskWipeMethodSecure:
		return "Cryptographic"
	case DiskWipeMethodDoD:
		return "Sanitize"
	default:
		return "Simple"
	}
}

// ResetBIOS resets BIOS configuration to factory defaults for Lenovo servers
func (l *LenovoCleaning) ResetBIOS(ctx context.Context, biosURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// Lenovo XClarity: POST to reset action
	actionURI := fmt.Sprintf("%s/Actions/Bios.ResetBios", biosURI)

	log.V(1).Info("Resetting Lenovo BIOS to defaults", "uri", actionURI)

	resp, err := l.client.Post(actionURI, map[string]any{})
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

// ResetBMC resets BMC configuration to factory defaults for Lenovo servers
func (l *LenovoCleaning) ResetBMC(ctx context.Context, manager *schemas.Manager) error {
	log := ctrl.LoggerFrom(ctx)

	// Lenovo XClarity: Use OEM action to reset to factory defaults
	// /redfish/v1/Managers/{id}/Actions/Manager.ResetToDefaults
	actionURI := fmt.Sprintf("%s/Actions/Manager.ResetToDefaults", manager.ODataID)

	payload := map[string]any{
		"ResetToDefaultsType": "ResetAll",
	}

	log.V(1).Info("Resetting Lenovo XCC to defaults", "uri", actionURI)

	resp, err := l.client.Post(actionURI, payload)
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

// ClearNetworkConfig clears network configuration for Lenovo servers
func (l *LenovoCleaning) ClearNetworkConfig(ctx context.Context, systemURI string) error {
	log := ctrl.LoggerFrom(ctx)

	// Lenovo: Clear network adapters configuration
	actionURI := fmt.Sprintf("%s/NetworkAdapters/Actions/NetworkAdapter.ClearConfiguration", systemURI)

	log.V(1).Info("Clearing Lenovo network configuration", "uri", actionURI)

	resp, err := l.client.Post(actionURI, map[string]any{})
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

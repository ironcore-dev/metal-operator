// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oem

import (
	"context"
	"net/http"

	"github.com/stmcginnis/gofish/schemas"
)

// CleaningInterface defines methods for OEM-specific cleaning operations
type CleaningInterface interface {
	// EraseDisk erases disks using vendor-specific methods
	EraseDisk(ctx context.Context, storages []*schemas.Storage, method DiskWipeMethod) error

	// ResetBIOS resets BIOS to factory defaults
	ResetBIOS(ctx context.Context, biosURI string) error

	// ResetBMC resets BMC to factory defaults
	ResetBMC(ctx context.Context, manager *schemas.Manager) error

	// ClearNetworkConfig clears network configuration
	ClearNetworkConfig(ctx context.Context, systemURI string) error
}

// DiskWipeMethod defines the disk wiping method
type DiskWipeMethod string

const (
	// DiskWipeMethodQuick performs a quick wipe (single pass with zeros)
	DiskWipeMethodQuick DiskWipeMethod = "quick"

	// DiskWipeMethodSecure performs a secure wipe (3 passes)
	DiskWipeMethodSecure DiskWipeMethod = "secure"

	// DiskWipeMethodDoD performs DoD 5220.22-M standard wipe (7 passes)
	DiskWipeMethodDoD DiskWipeMethod = "dod"
)

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	Post(uri string, payload any) (*http.Response, error)
}

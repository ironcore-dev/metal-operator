// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

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

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	Post(uri string, payload any) (*http.Response, error)
}

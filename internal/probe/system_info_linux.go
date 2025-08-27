// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package probe

import (
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/jaypipes/ghw"
)

func collectSystemInfoData() (registry.DMI, error) {
	product, err := ghw.Product()
	if err != nil {
		return registry.DMI{}, err
	}
	bios, err := ghw.BIOS()
	if err != nil {
		return registry.DMI{}, err
	}
	baseboard, err := ghw.Baseboard()
	if err != nil {
		return registry.DMI{}, err
	}
	dmi := registry.DMI{
		BIOSInformation: registry.BIOSInformation{
			Vendor:  bios.Vendor,
			Version: bios.Version,
			Date:    bios.Date,
		},
		SystemInformation: registry.ServerInformation{
			Manufacturer: product.Vendor,
			ProductName:  product.Name,
			Version:      product.Version,
			SerialNumber: product.SerialNumber,
			UUID:         product.UUID,
			SKUNumber:    product.SKU,
			Family:       product.Family,
		},
		BoardInformation: registry.BoardInformation{
			Manufacturer: baseboard.Vendor,
			Product:      baseboard.Product,
			Version:      baseboard.Version,
			SerialNumber: baseboard.SerialNumber,
			AssetTag:     baseboard.AssetTag,
		},
	}
	return dmi, nil
}

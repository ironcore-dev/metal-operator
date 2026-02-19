// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"github.com/jaypipes/ghw"
)

func collectStorageInfoData() ([]registry.BlockDevice, error) {
	blockDevices := make([]registry.BlockDevice, 0)
	blockStorage, err := ghw.Block()
	if err != nil {
		return blockDevices, fmt.Errorf("failed to get block devices: %w", err)
	}
	for _, b := range blockStorage.Disks {
		rota, err := ToBool(fmt.Sprintf("/sys/class/block/%s/queue/rotational", b.Name))
		if err != nil {
			// TODO: just log this or exit?
			return blockDevices, fmt.Errorf("failed to read rotational state for: %s; %w", b.Name, err)
		}
		ro, err := ToBool(fmt.Sprintf("/sys/class/block/%s/ro", b.Name))
		if err != nil {
			// TODO: just log this or exit?
			return blockDevices, fmt.Errorf("failed to read readonly state for: %s: %w", b.Name, err)
		}
		lbsz, err := ToInt(fmt.Sprintf("/sys/class/block/%s/queue/logical_block_size", b.Name))
		if err != nil {
			return blockDevices, fmt.Errorf("failed to read logical block size for %s: %w", b.Name, err)
		}
		secsz, err := ToInt(fmt.Sprintf("/sys/class/block/%s/queue/hw_sector_size", b.Name))
		if err != nil {
			return blockDevices, fmt.Errorf("failed to read hardware sector size for %s: %w", b.Name, err)
		}
		blockDevices = append(blockDevices, registry.BlockDevice{
			Path:              b.BusPath,
			Name:              b.Name,
			Rotational:        rota,
			Removable:         b.IsRemovable,
			ReadOnly:          ro,
			Vendor:            b.Vendor,
			Model:             b.Model,
			Serial:            b.SerialNumber,
			WWID:              b.WWN,
			PhysicalBlockSize: b.PhysicalBlockSizeBytes,
			LogicalBlockSize:  uint64(lbsz),
			HWSectorSize:      uint64(secsz),
			SizeBytes:         b.SizeBytes,
			NUMANodeID:        b.NUMANodeID,
		})
	}
	return blockDevices, nil
}

func ToString(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("unable to read file %s: %w", path, err)
	}

	contentsString := string(contents)
	trimmedString := strings.TrimSpace(contentsString)

	return trimmedString, nil
}

func ToInt(path string) (int, error) {
	fileString, err := ToString(path)
	if err != nil {
		return 0, fmt.Errorf("unable to read string from file %s: %w", path, err)
	}

	num, err := strconv.Atoi(fileString)
	if err != nil {
		return 0, fmt.Errorf("unable to convert %s file to int: %w", fileString, err)
	}

	return num, nil
}

func ToBool(path string) (bool, error) {
	num, err := ToInt(path)
	if err != nil {
		return false, fmt.Errorf("unable to read int from file %s: %w", path, err)
	}

	return num == 1, nil
}

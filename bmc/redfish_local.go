// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/stmcginnis/gofish/redfish"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ BMC = (*RedfishLocalBMC)(nil)

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(
	ctx context.Context,
	options BMCOptions,
) (BMC, error) {
	bmc, err := NewRedfishBMCClient(ctx, options)
	if err != nil {
		return nil, err
	}
	return &RedfishLocalBMC{RedfishBMC: bmc}, nil
}

func (r RedfishLocalBMC) PowerOn(ctx context.Context, systemUUID string) error {

	go func() {
		time.Sleep(100 * time.Microsecond)
		if system, ok := UnitTestMockUps.ComputeSystemMock[systemUUID]; ok {
			system.PowerState = redfish.OnPowerState
			UnitTestMockUps.ComputeSystemMock[systemUUID] = system
		} else {
			system, _ := r.getSystemByUUID(ctx, systemUUID)
			system.PowerState = redfish.OnPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				log := ctrl.LoggerFrom(ctx)
				log.V(1).Error(err, "failed to set power state On", "systemUUID", systemUUID)
			}
			UnitTestMockUps.ComputeSystemMock[systemUUID] = system
		}

		// mock the bmc update here
		if len(UnitTestMockUps.PendingBIOSSetting) > 0 {
			time.Sleep(50 * time.Millisecond)
			for key, data := range UnitTestMockUps.PendingBIOSSetting {
				if _, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
					UnitTestMockUps.BIOSSettingAttr[key] = data
				}
			}
			UnitTestMockUps.ResetPendingBIOSSetting()
		}
	}()
	return nil
}

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemUUID string) error {

	go func() {
		time.Sleep(100 * time.Microsecond)
		if system, ok := UnitTestMockUps.ComputeSystemMock[systemUUID]; ok {
			system.PowerState = redfish.OffPowerState
			UnitTestMockUps.ComputeSystemMock[systemUUID] = system
		}
		system, _ := r.getSystemByUUID(ctx, systemUUID)
		system.PowerState = redfish.OffPowerState
		systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
		if err := system.Patch(systemURI, system); err != nil {
			log := ctrl.LoggerFrom(ctx)
			log.V(1).Error(err, "failed to set power state Off", "systemUUID", systemUUID)
		}
		UnitTestMockUps.ComputeSystemMock[systemUUID] = system
	}()
	return nil
}

func (r *RedfishLocalBMC) GetBiosPendingAttributeValues(
	ctx context.Context,
	systemUUID string,
) (
	redfish.SettingsAttributes,
	error,
) {
	if len(UnitTestMockUps.PendingBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(UnitTestMockUps.PendingBIOSSetting))

	for key, data := range UnitTestMockUps.PendingBIOSSetting {
		result[key] = data["value"]
	}

	return result, nil
}

// mock SetBiosAttributesOnReset sets given bios attributes for unit testing.
func (r *RedfishLocalBMC) SetBiosAttributesOnReset(
	ctx context.Context,
	systemUUID string,
	attributes redfish.SettingsAttributes,
) error {

	UnitTestMockUps.ResetPendingBIOSSetting()
	for key, attrData := range attributes {
		if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				UnitTestMockUps.PendingBIOSSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	return nil
}

func (r *RedfishLocalBMC) GetBiosAttributeValues(
	ctx context.Context,
	systemUUID string,
	attributes []string,
) (
	redfish.SettingsAttributes,
	error,
) {

	if len(attributes) == 0 {
		return nil, nil
	}

	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := UnitTestMockUps.BIOSSettingAttr[name]; ok {
				result[name] = AttributesData["value"]
			}
		}
	}
	return result, nil
}

func (r *RedfishLocalBMC) getFilteredBiosRegistryAttributes(
	readOnly bool,
	immutable bool,
) (
	map[string]RegistryEntryAttributes,
	error,
) {
	filtered := make(map[string]RegistryEntryAttributes)
	if len(UnitTestMockUps.BIOSSettingAttr) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for name, AttributesData := range UnitTestMockUps.BIOSSettingAttr {
		data := RegistryEntryAttributes{}
		data.AttributeName = name
		data.Immutable = immutable
		data.ReadOnly = readOnly
		data.Type = AttributesData["type"].(string)
		data.ResetRequired = AttributesData["reboot"].(bool)
		filtered[name] = data
	}
	return filtered, nil
}

// check if the attributes need to reboot when changed, and are correct type.
// supported attrType, bmc and bios
func (r *RedfishLocalBMC) CheckBiosAttributes(attrs redfish.SettingsAttributes) (bool, error) {
	reset := false
	filtered, err := r.getFilteredBiosRegistryAttributes(false, false)

	if err != nil {
		return reset, err
	}

	if len(filtered) == 0 {
		return reset, err
	}
	return r.checkAttribues(attrs, filtered)
}

func (r *RedfishLocalBMC) getSystemByUUID(ctx context.Context, systemUUID string) (*redfish.ComputerSystem, error) {
	if system, ok := UnitTestMockUps.ComputeSystemMock[systemUUID]; ok {
		return system, nil
	}
	system, err := r.RedfishBMC.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return nil, err
	}
	UnitTestMockUps.ComputeSystemMock[systemUUID] = system
	return system, nil
}

func (r *RedfishLocalBMC) WaitForServerPowerState(
	ctx context.Context,
	systemUUID string,
	powerState redfish.PowerState,
) error {
	if err := wait.PollUntilContextTimeout(
		ctx,
		r.options.PowerPollingInterval,
		r.options.PowerPollingTimeout,
		true,
		func(ctx context.Context) (done bool, err error) {
			sysInfo, err := r.getSystemByUUID(ctx, systemUUID)
			if err != nil {
				return false, fmt.Errorf("failed to get system info: %w", err)
			}
			return sysInfo.PowerState == powerState, nil
		}); err != nil {
		return fmt.Errorf("failed to wait for for server power state: %w", err)
	}
	return nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishLocalBMC) SetPXEBootOnce(ctx context.Context, systemUUID string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	var setBoot redfish.Boot

	// TODO: cover setting BootSourceOverrideMode with BIOS settings profile
	if system.Boot.BootSourceOverrideMode != redfish.UEFIBootSourceOverrideMode {
		setBoot = pxeBootWithSettingUEFIBootMode
	} else {
		setBoot = pxeBootWithoutSettingUEFIBootMode
	}
	system.Boot = setBoot
	UnitTestMockUps.ComputeSystemMock[systemUUID] = system
	return nil
}

func (r *RedfishLocalBMC) GetBootOrder(ctx context.Context, systemUUID string) ([]string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return []string{}, err
	}
	return r.getBootOrder(system)
}

func (r *RedfishLocalBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return "", err
	}
	return r.getBiosVersion(system)
}

func (r *RedfishLocalBMC) SetBootOrder(ctx context.Context, systemUUID string, bootOrder []string) error {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return err
	}

	system.Boot = redfish.Boot{
		BootSourceOverrideEnabled: redfish.ContinuousBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  redfish.NoneBootSourceOverrideTarget,
		BootOrder:                 bootOrder,
	}
	UnitTestMockUps.ComputeSystemMock[systemUUID] = system
	return nil
}

func (r *RedfishLocalBMC) GetSystemInfo(ctx context.Context, systemUUID string) (SystemInfo, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get systems: %w", err)
	}
	var systemProcessors []*redfish.Processor
	var ok bool
	if systemProcessors, ok = UnitTestMockUps.SystemProcessorMock[systemUUID]; !ok || systemProcessors == nil {
		systemProcessors, err = system.Processors()
		if err != nil {
			return SystemInfo{}, fmt.Errorf("failed to get processors: %w", err)
		}
	}
	return r.getSystemInfo(system, systemProcessors)
}

func (r *RedfishLocalBMC) GetStorages(ctx context.Context, systemUUID string) ([]Storage, error) {

	var systemStorage []*redfish.SimpleStorage
	var ok bool
	if systemStorage, ok = UnitTestMockUps.SystemStorageMock[systemUUID]; !ok || systemStorage == nil {
		system, err := r.RedfishBMC.getSystemByUUID(ctx, systemUUID)
		if err != nil {
			return nil, err
		}
		// if no storage is found, fall back to simpleStorage (outdated storage API)
		systemStorage, err = system.SimpleStorages()
		if err != nil {
			return nil, fmt.Errorf("failed to wait for for server Simplestorages to be ready: %w", err)
		}
		UnitTestMockUps.SystemStorageMock[systemUUID] = systemStorage
	}
	return r.getSimpleStorages(systemStorage)
}

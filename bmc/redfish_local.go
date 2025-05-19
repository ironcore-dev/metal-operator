// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/stmcginnis/gofish/redfish"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ BMC = (*RedfishLocalBMC)(nil)

var DefaultMockedBIOSSetting = map[string]map[string]any{
	"abc":       {"type": "string", "reboot": false, "value": "bar"},
	"fooreboot": {"type": "integer", "reboot": true, "value": 123},
}

var PendingMockedBIOSSetting = map[string]map[string]any{}

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	*RedfishBMC
	StoredBIOSSettingData map[string]map[string]any
	StoredBMCSettingData  map[string]map[string]any
}

var ComputeSystemMock = map[string]*redfish.ComputerSystem{}
var SystemProcessorMock = map[string][]*redfish.Processor{}

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
		if system, ok := ComputeSystemMock[systemUUID]; ok {
			time.Sleep(100 * time.Microsecond)
			system.PowerState = redfish.OnPowerState
			ComputeSystemMock[systemUUID] = system
			fmt.Printf("\npowered on system %v\n", time.Now())
		} else {
			system, _ := r.getSystemByUUID(ctx, systemUUID)
			system.PowerState = redfish.OnPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				fmt.Printf("/nfailed to set power state %s for system %s: %v. %v\n", redfish.OnPowerState, systemUUID, err, time.Now())
			}
			ComputeSystemMock[systemUUID] = system
		}

		// mock the bmc update here
		if len(PendingMockedBIOSSetting) > 0 {

			for key, data := range PendingMockedBIOSSetting {
				if _, ok := DefaultMockedBIOSSetting[key]; ok {
					DefaultMockedBIOSSetting[key] = data
				}
			}
			PendingMockedBIOSSetting = map[string]map[string]any{}
			r.StoredBIOSSettingData = DefaultMockedBIOSSetting
		}
	}()
	return nil
}

func (r RedfishLocalBMC) PowerOff(ctx context.Context, systemUUID string) error {

	go func() {

		if system, ok := ComputeSystemMock[systemUUID]; ok {
			time.Sleep(100 * time.Microsecond)
			system.PowerState = redfish.OffPowerState
			ComputeSystemMock[systemUUID] = system
			fmt.Printf("\npowered off system %v\n", time.Now())

		}
		system, _ := r.getSystemByUUID(ctx, systemUUID)
		system.PowerState = redfish.OffPowerState
		systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
		if err := system.Patch(systemURI, system); err != nil {
			fmt.Printf("failed to set power state %s for system %s: %v", redfish.OffPowerState, systemUUID, err)
		}
		ComputeSystemMock[systemUUID] = system
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
	if len(PendingMockedBIOSSetting) == 0 {
		return redfish.SettingsAttributes{}, nil
	}

	result := make(redfish.SettingsAttributes, len(PendingMockedBIOSSetting))

	for key, data := range PendingMockedBIOSSetting {
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
	if len(DefaultMockedBIOSSetting) == 0 {
		DefaultMockedBIOSSetting = map[string]map[string]any{}
	}

	PendingMockedBIOSSetting = map[string]map[string]any{}
	for key, attrData := range attributes {
		if AttributesData, ok := DefaultMockedBIOSSetting[key]; ok {
			if reboot, ok := AttributesData["reboot"]; ok && !reboot.(bool) {
				// if reboot not needed, set the attribute immediately.
				AttributesData["value"] = attrData
			} else {
				// if reboot needed, set the attribute at next power on.
				PendingMockedBIOSSetting[key] = map[string]any{
					"type":   AttributesData["type"],
					"reboot": AttributesData["reboot"],
					"value":  attrData,
				}
			}
		}
	}
	r.StoredBIOSSettingData = DefaultMockedBIOSSetting

	return nil
}

func (r *RedfishLocalBMC) getMockedBIOSSettingData() map[string]map[string]any {

	if len(r.StoredBIOSSettingData) > 0 {
		return r.StoredBIOSSettingData
	}
	return DefaultMockedBIOSSetting

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

	mockedAttributes := r.getMockedBIOSSettingData()

	filteredAttr, err := r.getFilteredBiosRegistryAttributes(false, false)
	if err != nil {
		return nil, err
	}
	result := make(redfish.SettingsAttributes, len(attributes))
	for _, name := range attributes {
		if _, ok := filteredAttr[name]; ok {
			if AttributesData, ok := mockedAttributes[name]; ok {
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
	mockedAttributes := r.getMockedBIOSSettingData()
	filtered := make(map[string]RegistryEntryAttributes)
	if len(mockedAttributes) == 0 {
		return filtered, fmt.Errorf("no bmc setting attributes found")
	}
	for name, AttributesData := range mockedAttributes {
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
	if system, ok := ComputeSystemMock[systemUUID]; ok {
		return system, nil
	}
	system, err := r.RedfishBMC.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return nil, err
	}
	ComputeSystemMock[systemUUID] = system
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
			fmt.Printf("current power state %v", sysInfo.PowerState)
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
	ComputeSystemMock[systemUUID] = system
	return nil
}

func (r *RedfishLocalBMC) GetBootOrder(ctx context.Context, systemUUID string) ([]string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return []string{}, err
	}
	return system.Boot.BootOrder, nil
}

func (r *RedfishLocalBMC) GetBiosVersion(ctx context.Context, systemUUID string) (string, error) {
	system, err := r.getSystemByUUID(ctx, systemUUID)
	if err != nil {
		return "", err
	}
	return system.BIOSVersion, nil
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
	ComputeSystemMock[systemUUID] = system
	return nil
}

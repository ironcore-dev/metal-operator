// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

var _ BMC = (*RedfishBMC)(nil)

// RedfishBMC is an implementation of the BMC interface for Redfish.
type RedfishBMC struct {
	client *gofish.APIClient
}

// NewRedfishBMCClient creates a new RedfishBMC with the given connection details.
func NewRedfishBMCClient(
	ctx context.Context,
	endpoint, username, password string,
	basicAuth bool,
) (*RedfishBMC, error) {
	clientConfig := gofish.ClientConfig{
		Endpoint:  endpoint,
		Username:  username,
		Password:  password,
		Insecure:  true,
		BasicAuth: basicAuth,
	}
	client, err := gofish.ConnectContext(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redfish endpoint: %w", err)
	}
	return &RedfishBMC{client: client}, nil
}

// Logout closes the BMC client connection by logging out
func (r *RedfishBMC) Logout() {
	if r.client != nil {
		r.client.Logout()
	}
}

// PowerOn powers on the system using Redfish.
func (r *RedfishBMC) PowerOn(systemUUID string) error {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	powerState := system.PowerState
	if powerState != redfish.OnPowerState {
		if err := system.Reset(redfish.OnResetType); err != nil {
			return fmt.Errorf("failed to reset system to power on state: %w", err)
		}
	}
	return nil
}

// PowerOff powers off the system using Redfish.
func (r *RedfishBMC) PowerOff(systemUUID string) error {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if err := system.Reset(redfish.GracefulShutdownResetType); err != nil {
		return fmt.Errorf("failed to reset system to power on state: %w", err)
	}
	return nil
}

// Reset performs a reset on the system using Redfish.
func (r *RedfishBMC) Reset() error {
	// Implementation details...
	return nil
}

// GetSystems get managed systems
func (r *RedfishBMC) GetSystems() ([]Server, error) {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return nil, fmt.Errorf("failed to get systems: %w", err)
	}
	servers := make([]Server, 0, len(systems))
	for _, s := range systems {
		servers = append(servers, Server{
			UUID:         s.UUID,
			Model:        s.Model,
			Manufacturer: s.Manufacturer,
			PowerState:   PowerState(s.PowerState),
			SerialNumber: s.SerialNumber,
		})
	}
	return servers, nil
}

// SetPXEBootOnce sets the boot device for the next system boot using Redfish.
func (r *RedfishBMC) SetPXEBootOnce(systemUUID string) error {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}
	if err := system.SetBoot(redfish.Boot{
		BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
		BootSourceOverrideMode:    redfish.UEFIBootSourceOverrideMode,
		BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
	}); err != nil {
		return fmt.Errorf("failed to set the boot order: %w", err)
	}
	return nil
}

func (r *RedfishBMC) GetManager() (*Manager, error) {
	if r.client == nil {
		return nil, fmt.Errorf("no client found")
	}
	managers, err := r.client.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("failed to get managers: %w", err)
	}
	for _, m := range managers {
		// TODO: always take the first for now.
		return &Manager{
			UUID:            m.UUID,
			Manufacturer:    m.Manufacturer,
			State:           string(m.Status.State),
			PowerState:      string(m.PowerState),
			SerialNumber:    m.SerialNumber,
			FirmwareVersion: m.FirmwareVersion,
			SKU:             m.PartNumber,
			Model:           m.Model,
		}, nil
	}

	return nil, err
}

// GetSystemInfo retrieves information about the system using Redfish.
func (r *RedfishBMC) GetSystemInfo(systemUUID string) (SystemInfo, error) {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get systems: %w", err)
	}

	return SystemInfo{
		SystemUUID:   system.UUID,
		Manufacturer: system.Manufacturer,
		Model:        system.Model,
		Status:       system.Status,
		PowerState:   system.PowerState,
		SerialNumber: system.SerialNumber,
		SKU:          system.SKU,
		IndicatorLED: string(system.IndicatorLED),
	}, nil
}

func (r *RedfishBMC) GetBootOrder(systemUUID string) ([]string, error) {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return []string{}, err
	}
	return system.Boot.BootOrder, nil
}

func (r *RedfishBMC) GetBiosVersion(systemUUID string) (string, error) {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return "", err
	}
	return system.BIOSVersion, nil
}

func (r *RedfishBMC) GetBiosSettings(systemUUID string, attributes map[string]string) (Bios, error) {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return Bios{}, err
	}
	if err := r.checkBiosAttributes(attributes); err != nil {
		return Bios{}, err
	}
	bios, err := system.Bios()
	if err != nil {
		return Bios{}, err
	}
	settings := make(map[string]string, len(bios.Attributes))
	for name := range bios.Attributes {
		settings[name] = bios.Attributes.String(name)
	}
	return Bios{
		Version:  system.BIOSVersion,
		Settings: settings,
	}, nil
}

func (r *RedfishBMC) SetBiosSettings(systemUUID string, attributes map[string]string) error {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return err
	}
	bios, err := system.Bios()
	if err != nil {
		return nil
	}
	if err := r.checkBiosAttributes(attributes); err != nil {
		return err
	}
	attrs := make(map[string]interface{}, len(attributes))
	for name, value := range attributes {
		attrs[name] = value
	}
	return bios.UpdateBiosAttributes(attrs)
}

func (r *RedfishBMC) SetBootOrder(systemUUID string, bootOrder []string) error {
	system, err := r.getSystemByID(systemUUID)
	if err != nil {
		return err
	}
	return system.SetBoot(
		redfish.Boot{
			BootSourceOverrideEnabled: redfish.ContinuousBootSourceOverrideEnabled,
			BootSourceOverrideTarget:  redfish.NoneBootSourceOverrideTarget,
			BootOrder:                 bootOrder,
		},
	)
}

func (r *RedfishBMC) checkBiosAttributes(attrs map[string]string) (err error) {
	registries, err := r.client.Service.Registries()
	biosRegistry := &BiosRegistry{}
	for _, registry := range registries {
		if strings.Contains(registry.ID, "BiosAttributeRegistry") {
			err = registry.Get(r.client, registry.Location[0].URI, biosRegistry)
			if err != nil {
				return err
			}
		}
	}
	// filter out immutable, readonly and hidden attributes
	filtered := make(map[string]RegistryEntryAttributes)
	for _, entry := range biosRegistry.RegistryEntries.Attributes {
		if !entry.Immutable && !entry.ReadOnly && !entry.Hidden {
			filtered[entry.AttributeName] = entry
		}
	}
	//TODO: add more tyes like maps and Enumerations
	for name, value := range attrs {
		entryAttribute, ok := filtered[name]
		if !ok {
			err = errors.Join(err, fmt.Errorf("attribute %s not found or immutable/hidden", name))
			continue
		}
		switch strings.ToLower(entryAttribute.Type) {
		case "integer":
			_, Aerr := strconv.Atoi(value)
			if Aerr != nil {
				err = errors.Join(err, fmt.Errorf("attribute %s value has wrong type", name))
			}
		case "string":
			continue
		default:
			err = errors.Join(err, fmt.Errorf("attribute %s value has wrong type", name))
		}
	}
	return err
}

func (r *RedfishBMC) getSystemByID(systemUUID string) (*redfish.ComputerSystem, error) {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return nil, err
	}
	for _, system := range systems {
		if system.ID == systemUUID {
			return system, nil
		}
	}
	return nil, errors.New("no system found")
}

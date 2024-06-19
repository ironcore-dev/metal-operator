/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bmc

import (
	"context"
	"fmt"

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
func (r *RedfishBMC) PowerOn(systemID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemID {
			powerState := system.PowerState
			if powerState != redfish.OnPowerState {
				if err := system.Reset(redfish.OnResetType); err != nil {
					return fmt.Errorf("failed to reset system to power on state: %w", err)
				}
			} else {
				fmt.Printf("System %s is already powered on.\n", systemID)
			}
			break
		}
	}

	return nil
}

// PowerOff powers off the system using Redfish.
func (r *RedfishBMC) PowerOff(systemID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemID {
			if err := system.Reset(redfish.GracefulShutdownResetType); err != nil {
				return fmt.Errorf("failed to reset system to power on state: %w", err)
			}
			break
		}
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
	service := r.client.GetService()

	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
			if err := system.SetBoot(redfish.Boot{
				BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
				BootSourceOverrideMode:    redfish.UEFIBootSourceOverrideMode,
				BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
			}); err != nil {
				return fmt.Errorf("failed to set the boot order: %w", err)
			}
		}
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
	service := r.client.GetService()

	systems, err := service.Systems()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
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
	}

	return SystemInfo{}, nil
}

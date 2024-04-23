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

var _ BMC = (*RedfishLocalBMC)(nil)

// RedfishLocalBMC is an implementation of the BMC interface for Redfish.
type RedfishLocalBMC struct {
	client *gofish.APIClient
}

// NewRedfishLocalBMCClient creates a new RedfishLocalBMC with the given connection details.
func NewRedfishLocalBMCClient(
	ctx context.Context,
	endpoint, username, password string,
	basicAuth bool,
) (*RedfishLocalBMC, error) {
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
	return &RedfishLocalBMC{client: client}, nil
}

func (r RedfishLocalBMC) PowerOn(systemUUID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
			system.PowerState = redfish.OnPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OnPowerState, systemUUID, err)
			}
			break
		}
	}
	return nil
}

func (r RedfishLocalBMC) PowerOff(systemUUID string) error {
	service := r.client.GetService()
	systems, err := service.Systems()
	if err != nil {
		return fmt.Errorf("failed to get systems: %w", err)
	}

	for _, system := range systems {
		if system.UUID == systemUUID {
			system.PowerState = redfish.OffPowerState
			systemURI := fmt.Sprintf("/redfish/v1/Systems/%s", system.ID)
			if err := system.Patch(systemURI, system); err != nil {
				return fmt.Errorf("failed to set power state %s for system %s: %w", redfish.OffPowerState, systemUUID, err)
			}
			break
		}
	}
	return nil
}

func (r RedfishLocalBMC) Reset() error {
	//TODO implement me
	panic("implement me")
}

func (r RedfishLocalBMC) SetPXEBootOnce(systemUUID string) error {
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

func (r RedfishLocalBMC) GetSystemInfo(systemUUID string) (SystemInfo, error) {
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

func (r RedfishLocalBMC) Logout() {
	if r.client != nil {
		r.client.Logout()
	}
}

func (r RedfishLocalBMC) GetSystems() ([]Server, error) {
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

func (r RedfishLocalBMC) GetManager() (*Manager, error) {
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

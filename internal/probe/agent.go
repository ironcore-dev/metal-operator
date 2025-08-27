// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jaypipes/ghw"
	"log"
	"net/http"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Agent struct {
	SystemUUID       string
	RegistryURL      string
	Duration         time.Duration
	Server           *registry.Server // Pointer to Server for late initialization.
	LLDPSyncInterval time.Duration
	LLDPSyncDuration time.Duration
}

// NewAgent creates a new Agent with the specified system UUID and registry URL.
func NewAgent(systemUUID, registryURL string, duration, LLDPSyncInterval, LLDPSyncDuration time.Duration) *Agent {
	return &Agent{
		SystemUUID:       systemUUID,
		RegistryURL:      registryURL,
		Duration:         duration,
		LLDPSyncInterval: LLDPSyncInterval,
		LLDPSyncDuration: LLDPSyncDuration,
	}
}

// Init initializes the Agent's Server field with network interface data.
func (a *Agent) Init(ctx context.Context) error {
	interfaces, err := collectNetworkData()
	if err != nil {
		return err
	}
	systeminfo, err := collectSystemInfoData()
	if err != nil {
		return err
	}

	cpuInfos, err := collectCPUInfoData()
	if err != nil {
		return err
	}

	LLDPInfo, err := collectLLDPInfo(ctx, a.LLDPSyncInterval, a.LLDPSyncDuration)
	if err != nil {
		return err
	}

	BlockDevices, err := collectStorageInfoData()
	if err != nil {
		return err
	}

	a.Server = &registry.Server{
		SystemInfo:        systeminfo,
		CPU:               cpuInfos,
		NetworkInterfaces: interfaces,
		LLDP:              LLDPInfo,
		Storage:           BlockDevices,
	}
	return nil
}

func collectCPUInfoData() ([]registry.CPUInfo, error) {
	var cpuInfos []registry.CPUInfo
	cpuInfo, err := ghw.CPU()
	if err != nil {
		return cpuInfos, fmt.Errorf("failed to get CPU info: %w", err)
	}
	for _, processor := range cpuInfo.Processors {
		cpuInfos = append(cpuInfos, registry.CPUInfo{
			ID:                   processor.ID,
			TotalCores:           processor.TotalCores,
			TotalHardwareThreads: processor.TotalHardwareThreads,
			Vendor:               processor.Vendor,
			Model:                processor.Model,
			Capabilities:         processor.Capabilities,
		})
	}
	return cpuInfos, nil
}

// Start begins the periodic registration process.
func (a *Agent) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Ensure the Agent is initialized.
	if a.Server == nil {
		if err := a.Init(ctx); err != nil {
			log.Printf("Error initializing agent: %v", err)
			return err
		}
	}

	// Run the registration immediately before starting the ticker loop.
	log.Println("Registering server ...")
	if err := a.registerServer(ctx); err != nil {
		log.Printf("Error during initial registration: %v", err)
		return err
	}
	log.Printf("Server with UUID: %s registered.", a.SystemUUID)

	for {
		select {
		case <-ctx.Done():
			log.Println("Probe agent stopped.")
			return nil
		case <-ticker.C:
			log.Println("Registering server ...")
			if err := a.registerServer(ctx); err != nil {
				log.Printf("Error during periodic registration: %v", err)
			}
			log.Printf("Server with UUID: %s re-registered.", a.SystemUUID)
		}
	}
}

// registerServer handles the server registration with exponential backoff on failure.
func (a *Agent) registerServer(ctx context.Context) error {
	payload := registry.RegistrationPayload{
		SystemUUID: a.SystemUUID,
		Data:       *a.Server, // Dereference the pointer to Server.
	}

	return wait.ExponentialBackoffWithContext(
		ctx,
		wait.Backoff{
			Steps:    1,
			Duration: a.Duration,
			Factor:   2.0,
			Jitter:   0.1,
		},
		func(ctx context.Context) (bool, error) {
			jsonData, err := json.Marshal(payload)
			if err != nil {
				return false, err
			}

			resp, err := http.Post(a.RegistryURL+"/register", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				log.Printf("Error posting data: %v", err)
				return false, nil
			}
			defer func() {
				err := resp.Body.Close()
				if err != nil {
					log.Printf("failed to close response body: %v", err)
				}
			}()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				fmt.Printf("Failed to register server: %s. Retrying...\n", resp.Status)
				return false, nil
			}

			log.Println("Server registered successfully.")
			return true, nil
		},
	)
}

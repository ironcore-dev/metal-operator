// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Agent struct {
	SystemUUID       string
	RegistryURL      string
	Duration         time.Duration
	Server           *registry.Server // Pointer to Server for late initialization.
	log              logr.Logger
	LLDPSyncInterval time.Duration
	LLDPSyncDuration time.Duration
}

// NewAgent creates a new Agent with the specified system UUID and registry URL.
func NewAgent(log logr.Logger, systemUUID, registryURL string, duration, LLDPSyncInterval, LLDPSyncDuration time.Duration) *Agent {
	return &Agent{
		log:              log,
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
		a.log.Error(err, "failed to collect LLDP info")
		return err
	}
	a.log.Info("Collected LLDP info", "interfaces", len(LLDPInfo.Interfaces))

	blockDevices, err := collectStorageInfoData()
	if err != nil {
		return err
	}

	memoryDevices, err := collectMemoryInfoData()
	if err != nil {
		return err
	}

	nics, err := collectNICInfoData()
	if err != nil {
		return err
	}

	pciDevices, err := collectPCIDevicesInfoData()
	if err != nil {
		return err
	}

	a.Server = &registry.Server{
		SystemInfo:        systeminfo,
		CPU:               cpuInfos,
		NetworkInterfaces: interfaces,
		LLDP:              LLDPInfo.Interfaces,
		Storage:           blockDevices,
		Memory:            memoryDevices,
		NICs:              nics,
		PCIDevices:        pciDevices,
	}
	return nil
}

// Start begins the periodic registration process.
func (a *Agent) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Ensure the Agent is initialized.
	if a.Server == nil {
		a.log.Info("Initializing probe agent...")
		if err := a.Init(ctx); err != nil {
			a.log.Error(err, "failed to initialize agent")
			return err
		}
	}

	// Run the registration immediately before starting the ticker loop.
	a.log.Info("Registering server ...")
	if err := a.registerServer(ctx); err != nil {
		a.log.Error(err, "failed to initially register server")
		return err
	}
	a.log.Info("Server registered", "uuid", a.SystemUUID)

	for {
		select {
		case <-ctx.Done():
			a.log.Info("Probe agent stopped.")
			return nil
		case <-ticker.C:
			// Only refresh LLDP info on subsequent runs, rest is static
			if a.Server == nil {
				a.log.Info("Server uninitialized; initializing probe agent...")
				if err := a.Init(ctx); err != nil {
					a.log.Error(err, "failed to initialize agent on tick")
					// don't stop the agent on transient errors; continue to next tick
					continue
				}
			} else {
				a.log.Info("Refreshing LLDP info...")
				if err := a.RefreshLLDP(ctx); err != nil {
					a.log.Error(err, "failed to refresh LLDP info; continuing with previous data")
				}
			}
			a.log.Info("Re-registering Server...")
			if err := a.registerServer(ctx); err != nil {
				a.log.Error(err, "failed to re-register server")
			}
			a.log.Info("Server registered", "uuid", a.SystemUUID)
		}
	}
}

// RefreshLLDP updates only the LLDP portion of the Agent's Server data.
// If LLDP collection fails, the previous LLDP data is retained.
func (a *Agent) RefreshLLDP(ctx context.Context) error {
	if a == nil || a.Server == nil {
		return nil
	}
	lldp, err := collectLLDPInfo(ctx, a.LLDPSyncInterval, a.LLDPSyncDuration)
	if err != nil {
		a.log.Error(err, "collectLLDPInfo failed")
		return err
	}
	a.Server.LLDP = lldp.Interfaces
	a.log.Info("Refreshed LLDP info", "interfaces", len(a.Server.LLDP))
	return nil
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
				a.log.Error(err, "failed to post registration data", "url", a.RegistryURL)
				return false, nil
			}
			defer func() {
				err := resp.Body.Close()
				if err != nil {
					a.log.Error(err, "failed to close response body")
				}
			}()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				a.log.Error(err, "failed to register server", "url", a.RegistryURL)
				return false, nil
			}

			a.log.Info("Server registered")
			return true, nil
		},
	)
}

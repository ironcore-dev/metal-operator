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
	SystemUUID  string
	RegistryURL string
	Duration    time.Duration
	Server      *registry.Server // Pointer to Server for late initialization.
	log         logr.Logger
}

// NewAgent creates a new Agent with the specified system UUID and registry URL.
func NewAgent(log logr.Logger, systemUUID, registryURL string, duration time.Duration) *Agent {
	return &Agent{
		SystemUUID:  systemUUID,
		RegistryURL: registryURL,
		Duration:    duration,
	}
}

// Init initializes the Agent's Server field with network interface data.
func (a *Agent) Init() error {
	ndd, err := NewNetworkDeviceData()
	if err != nil {
		log.Printf("Error loading network device data: %v", err)
		return err
	}

	interfaces, err := NewNetworkDataCollector(NewNIC(), ndd).CollectNetworkData()
	if err != nil {
		log.Printf("Error collecting network data: %v", err)
		return err
	}

	a.Server = &registry.Server{NetworkInterfaces: interfaces}
	return nil
}

// Start begins the periodic registration process.
func (a *Agent) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Ensure the Agent is initialized.
	if a.Server == nil {
		if err := a.Init(); err != nil {
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
			a.log.Info("Registering server ...")
			if err := a.registerServer(ctx); err != nil {
				a.log.Error(err, "failed to register server")
			}
			a.log.Info("Server registered", "uuid", a.SystemUUID)
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

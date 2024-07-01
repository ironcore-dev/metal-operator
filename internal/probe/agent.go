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

package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ironcore-dev/metal-operator/internal/api/registry"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Agent struct {
	SystemUUID  string
	RegistryURL string
	Server      *registry.Server // Pointer to Server for late initialization.
}

// NewAgent creates a new Agent with the specified system UUID and registry URL.
func NewAgent(systemUUID, registryURL string) *Agent {
	return &Agent{
		SystemUUID:  systemUUID,
		RegistryURL: registryURL,
	}
}

// Init initializes the Agent's Server field with network interface data.
func (a *Agent) Init() error {
	interfaces, err := collectNetworkData()
	if err != nil {
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
			Duration: 5 * time.Second,
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

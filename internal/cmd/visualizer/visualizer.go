// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package visualizer

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/metal-operator/internal/cmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed index.html
var indexHTML string

type Visualizer struct {
	Client  client.Client
	Address string
}

// NewVisualizer creates and returns a new Visualizer instance.
func NewVisualizer(c client.Client, port int) *Visualizer {
	return &Visualizer{
		Client:  c,
		Address: fmt.Sprintf("localhost:%d", port),
	}
}

// StartAndServe initializes the HTTP routes and starts the server.
// It also handles graceful shutdown on interrupt signals.
func (v *Visualizer) StartAndServe() error {
	url := fmt.Sprintf("http://%s", v.Address)

	http.HandleFunc("/", serveFrontend)
	http.HandleFunc("/api/servers", v.handleGetServers())

	srv := &http.Server{
		Addr:    v.Address,
		Handler: nil,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on %s. Press Ctrl+C to stop.", url)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-stopChan

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server gracefully stopped.")
	return nil
}

// handleGetServers creates an HTTP handler to list Server resources.
func (v *Visualizer) handleGetServers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		serverList := &metalv1alpha1.ServerList{}
		if err := v.Client.List(ctx, serverList); err != nil {
			http.Error(w, fmt.Sprintf("Failed to list servers: %v", err), http.StatusInternalServerError)
			log.Printf("Error listing servers: %v", err)
			return
		}

		var servers []api.ServerInfo
		for _, item := range serverList.Items {
			server, err := parseServerToServerInfo(&item)
			if err != nil {
				log.Printf("Skipping server due to parsing error: %v", err)
				continue
			}
			servers = append(servers, server)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(servers); err != nil {
			http.Error(w, "Failed to encode servers to JSON", http.StatusInternalServerError)
			log.Printf("Error encoding JSON: %v", err)
		}
	}
}

// serveFrontend handles serving the embedded index.html.
func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, err := fmt.Fprint(w, indexHTML)
	if err != nil {
		http.Error(w, "Failed to serve frontend", http.StatusInternalServerError)
		return
	}
}

// parseServerToServerInfo extracts the required fields from a Server object.
func parseServerToServerInfo(server *metalv1alpha1.Server) (api.ServerInfo, error) {
	var info api.ServerInfo
	info.Name = server.GetName()

	// Extract labels
	labels := server.GetLabels()
	rack, ok := labels[metalv1alpha1.TopologyRack]
	if ok {
		info.Rack = rack
	}

	heightUnitStr, ok := labels[metalv1alpha1.TopologyHeightUnit]
	if ok {
		heightUnit, err := strconv.Atoi(heightUnitStr)
		if err != nil {
			return info, fmt.Errorf("could not parse heightunit label for server %s: %w", info.Name, err)
		}
		info.HeightUnit = heightUnit
	}

	info.Power = string(server.Status.PowerState)
	info.IndicatorLED = string(server.Spec.IndicatorLED)
	info.State = string(server.Status.State)

	return info, nil
}

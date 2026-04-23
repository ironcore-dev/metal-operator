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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
			server, err := v.parseServerToServerInfo(&item)
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
func (v *Visualizer) parseServerToServerInfo(server *metalv1alpha1.Server) (api.ServerInfo, error) {
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

	// Fetch ServerMetadata for enrichment and hardware data
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	serverMetadata := &metalv1alpha1.ServerMetadata{}
	metadataKey := client.ObjectKey{Name: server.Name}

	if err := v.Client.Get(ctx, metadataKey, serverMetadata); err == nil {
		// Extract enrichment data
		if len(serverMetadata.Enrichment) > 0 {
			info.Enrichment = serverMetadata.Enrichment
			info.Location = parseLocationFromEnrichment(serverMetadata.Enrichment)
		}

		// Extract hardware metadata
		info.Hardware = extractHardwareInfo(serverMetadata)
	} else if !apierrors.IsNotFound(err) {
		log.Printf("Warning: Failed to fetch ServerMetadata for %s: %v", info.Name, err)
	}

	return info, nil
}

// parseLocationFromEnrichment extracts location hierarchy from enrichment map using well-known keys.
func parseLocationFromEnrichment(enrichment map[string]string) *api.LocationInfo {
	loc := &api.LocationInfo{
		Site:     enrichment[metalv1alpha1.EnrichmentLocationSite],
		Building: enrichment[metalv1alpha1.EnrichmentLocationBuilding],
		Room:     enrichment[metalv1alpha1.EnrichmentLocationRoom],
		RackName: enrichment[metalv1alpha1.EnrichmentLocationRack],
		Position: enrichment[metalv1alpha1.EnrichmentLocationPosition],
	}

	// Build hierarchical path for drill-down navigation
	if loc.Site != "" {
		loc.HierarchyPath = append(loc.HierarchyPath, loc.Site)
	}
	if loc.Building != "" {
		loc.HierarchyPath = append(loc.HierarchyPath, loc.Building)
	}
	if loc.Room != "" {
		loc.HierarchyPath = append(loc.HierarchyPath, loc.Room)
	}

	// Return nil if no location data found
	if len(loc.HierarchyPath) == 0 {
		return nil
	}

	return loc
}

// extractHardwareInfo extracts aggregated hardware summary from ServerMetadata.
func extractHardwareInfo(metadata *metalv1alpha1.ServerMetadata) *api.HardwareInfo {
	hw := &api.HardwareInfo{}

	// System information
	if metadata.SystemInfo.SystemInformation.Manufacturer != "" {
		hw.Manufacturer = metadata.SystemInfo.SystemInformation.Manufacturer
		hw.Model = metadata.SystemInfo.SystemInformation.ProductName
		hw.SerialNumber = metadata.SystemInfo.SystemInformation.SerialNumber
		hw.UUID = metadata.SystemInfo.SystemInformation.UUID
	}

	// BIOS information
	if metadata.SystemInfo.BIOSInformation.Vendor != "" {
		hw.BIOSVendor = metadata.SystemInfo.BIOSInformation.Vendor
		hw.BIOSVersion = metadata.SystemInfo.BIOSInformation.Version
	}

	// CPU summary
	hw.TotalCPUs = len(metadata.CPU)
	if len(metadata.CPU) > 0 {
		hw.CPUModel = metadata.CPU[0].Model
		for _, cpu := range metadata.CPU {
			hw.TotalCores += cpu.TotalCores
			hw.TotalThreads += cpu.TotalHardwareThreads
		}
	}

	// Memory summary
	hw.MemoryModules = len(metadata.Memory)
	var totalMemoryBytes int64
	for _, mem := range metadata.Memory {
		totalMemoryBytes += mem.SizeBytes
	}
	hw.TotalMemoryGB = int(totalMemoryBytes / (1024 * 1024 * 1024))

	// Storage summary
	hw.StorageDevices = len(metadata.Storage)
	var totalStorageBytes uint64
	for _, storage := range metadata.Storage {
		totalStorageBytes += storage.SizeBytes
	}
	hw.TotalStorageGB = int(totalStorageBytes / (1024 * 1024 * 1024))

	// Network summary
	hw.NetworkInterfaces = len(metadata.NetworkInterfaces)

	// PCI devices
	hw.PCIDeviceCount = len(metadata.PCIDevices)

	// Return nil if no hardware data found
	if hw.Manufacturer == "" && hw.TotalCPUs == 0 {
		return nil
	}

	return hw
}

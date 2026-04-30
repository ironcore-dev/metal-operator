// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newTestRedfishBMC creates a RedfishBaseBMC backed by the given httptest server.
// It connects using basic auth to avoid session creation requests.
func newTestRedfishBMC(server *httptest.Server) *RedfishBaseBMC {
	client, err := gofish.ConnectContext(context.Background(),
		gofish.ClientConfig{
			Endpoint:   server.URL,
			Username:   "admin",
			Password:   "admin",
			BasicAuth:  true,
			HTTPClient: server.Client(),
		})
	Expect(err).NotTo(HaveOccurred())
	return &RedfishBaseBMC{client: client}
}

// managerJSON returns JSON bytes for a Manager with the given fields.
func managerJSON(id string, maxSessions uint, connectTypes []string) []byte {
	type graphicalConsole struct {
		MaxConcurrentSessions uint     `json:"MaxConcurrentSessions"`
		ConnectTypesSupported []string `json:"ConnectTypesSupported"`
		ServiceEnabled        bool     `json:"ServiceEnabled"`
	}
	m := struct {
		ODataID          string           `json:"@odata.id"`
		ID               string           `json:"Id"`
		Name             string           `json:"Name"`
		GraphicalConsole graphicalConsole `json:"GraphicalConsole"`
	}{
		ODataID: "/redfish/v1/Managers/" + id,
		ID:      id,
		Name:    "Manager " + id,
		GraphicalConsole: graphicalConsole{
			MaxConcurrentSessions: maxSessions,
			ConnectTypesSupported: connectTypes,
			ServiceEnabled:        true,
		},
	}
	b, _ := json.Marshal(m)
	return b
}

// serviceRootJSON returns JSON for a minimal Redfish service root.
func serviceRootJSON() []byte {
	root := map[string]any{
		"@odata.id":      "/redfish/v1/",
		"Id":             "ServiceRoot",
		"Name":           "Service Root",
		"RedfishVersion": "1.0.0",
		"Managers":       map[string]string{"@odata.id": "/redfish/v1/Managers"},
	}
	b, _ := json.Marshal(root)
	return b
}

// managersCollectionJSON returns JSON for a managers collection with the given member links.
func managersCollectionJSON(memberLinks []string) []byte {
	type member struct {
		ODataID string `json:"@odata.id"`
	}
	members := make([]member, len(memberLinks))
	for i, link := range memberLinks {
		members[i] = member{ODataID: link}
	}
	col := map[string]any{
		"@odata.id":           "/redfish/v1/Managers",
		"Name":                "Manager Collection",
		"Members@odata.count": len(memberLinks),
		"Members":             members,
	}
	b, _ := json.Marshal(col)
	return b
}

var _ = Describe("RedfishBaseBMC DiscoverManager", func() {
	ctx := context.Background()

	It("should return the first manager with MaxConcurrentSessions set", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(serviceRootJSON()) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managersCollectionJSON([]string{"/redfish/v1/Managers/BMC1"})) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers/BMC1", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managerJSON("BMC1", 2, nil)) //nolint:errcheck
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestRedfishBMC(server)
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		Expect(manager.GraphicalConsole.MaxConcurrentSessions).To(Equal(uint(2)))
	})

	It("should return the first manager with ConnectTypesSupported set", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(serviceRootJSON()) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managersCollectionJSON([]string{"/redfish/v1/Managers/BMC1"})) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers/BMC1", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managerJSON("BMC1", 0, []string{"KVMIP"})) //nolint:errcheck
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestRedfishBMC(server)
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		Expect(manager.GraphicalConsole.ConnectTypesSupported).To(HaveLen(1))
		Expect(manager.GraphicalConsole.ConnectTypesSupported[0]).To(Equal(schemas.GraphicalConnectTypesSupported("KVMIP")))
	})

	It("should skip managers without graphical console and return a suitable one", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(serviceRootJSON()) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managersCollectionJSON([]string{"/redfish/v1/Managers/NoConsole", "/redfish/v1/Managers/WithConsole"})) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers/NoConsole", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managerJSON("NoConsole", 0, nil)) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers/WithConsole", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managerJSON("WithConsole", 4, []string{"KVMIP"})) //nolint:errcheck
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestRedfishBMC(server)
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		// Should have found the one with graphical console capabilities
		Expect(manager.GraphicalConsole.MaxConcurrentSessions).To(BeNumerically(">", 0))
	})

	It("should return an error when no managers have graphical console capabilities", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(serviceRootJSON()) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managersCollectionJSON([]string{"/redfish/v1/Managers/Plain"})) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers/Plain", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(managerJSON("Plain", 0, nil)) //nolint:errcheck
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestRedfishBMC(server)
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no manager found with graphical console capabilities"))
		Expect(manager).To(BeNil())
	})

	It("should return an error when the managers endpoint fails", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(serviceRootJSON()) //nolint:errcheck
		})
		mux.HandleFunc("/redfish/v1/Managers", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "internal error"}}`)) //nolint:errcheck
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestRedfishBMC(server)
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).To(HaveOccurred())
		Expect(manager).To(BeNil())
	})

	It("should return an error when the client is nil", func() {
		bmc := &RedfishBaseBMC{client: nil}
		manager, err := bmc.DiscoverManager(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no client found"))
		Expect(manager).To(BeNil())
	})
})

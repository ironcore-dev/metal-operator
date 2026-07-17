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

// updateServiceWithETag returns a minimal UpdateService JSON body that includes
// an @odata.etag field, mirroring Dell iDRAC v1_15_0+ behaviour.
func updateServiceWithETag(etag string) []byte {
	body := map[string]any{
		"@odata.type":    "#UpdateService.v1_15_0.UpdateService",
		"@odata.id":      "/redfish/v1/UpdateService",
		"@odata.etag":    etag,
		"Id":             "UpdateService",
		"Name":           "Update Service",
		"ServiceEnabled": true,
		"Actions": map[string]any{
			"#UpdateService.SimpleUpdate": map[string]any{
				"target": "/redfish/v1/UpdateService/Actions/SimpleUpdate",
			},
		},
	}
	b, _ := json.Marshal(body)
	return b
}

var _ = Describe("upgradeVersion ETag handling", func() {
	// Regression test for: Dell iDRAC v1_15_0+ includes @odata.etag on the
	// UpdateService resource. gofish's Entity.PostWithResponse automatically
	// sends that ETag as If-Match on any POST. SimpleUpdate is an action
	// endpoint — it does not accept If-Match — so the BMC returns 412.
	// Fix: call DisableEtagMatch(true) on the UpdateService entity before posting.
	Describe("SimpleUpdate action POST", func() {
		It("must NOT send If-Match even when UpdateService carries @odata.etag", func() {
			const etag = `W/"gen-1"`
			var capturedIfMatch string

			// Minimal Redfish server: serves the service root, UpdateService
			// (with @odata.etag), and the SimpleUpdate action endpoint.
			mux := http.NewServeMux()

			mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, r *http.Request) {
				root := map[string]any{
					"@odata.id":      "/redfish/v1/",
					"Id":             "ServiceRoot",
					"RedfishVersion": "1.0.0",
					"UpdateService":  map[string]string{"@odata.id": "/redfish/v1/UpdateService"},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(root)
			})

			mux.HandleFunc("/redfish/v1/UpdateService", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(updateServiceWithETag(etag))
			})

			mux.HandleFunc("/redfish/v1/UpdateService/Actions/SimpleUpdate", func(w http.ResponseWriter, r *http.Request) {
				// Capture the If-Match header so the test can assert it is absent.
				capturedIfMatch = r.Header.Get("If-Match")

				// Mirror real iDRAC: reject If-Match on action POSTs with 412.
				if capturedIfMatch != "" {
					w.WriteHeader(http.StatusPreconditionFailed)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"error": map[string]any{
							"code":    "Base.1.18.PreconditionFailed",
							"message": "The ETag supplied did not match the ETag required to change this resource.",
						},
					})
					return
				}
				w.Header().Set("Location", "/redfish/v1/TaskService/Tasks/1")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"status":"Accepted"}`))
			})

			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)

			client, err := gofish.ConnectContext(context.Background(), gofish.ClientConfig{
				Endpoint:   server.URL,
				Username:   "admin",
				Password:   "admin",
				BasicAuth:  true,
				HTTPClient: server.Client(),
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(client.Logout)

			base := &RedfishBaseBMC{client: client}
			params := &schemas.UpdateServiceSimpleUpdateParameters{
				ImageURI: "http://example.com/bios.bin",
			}

			// Use the local (generic) request body builder — vendor doesn't
			// matter here; we are testing the shared upgradeVersion path.
			taskURI, isFatal, err := upgradeVersion(
				context.Background(),
				base,
				params,
				localBuildBiosRequestBody,
				localExtractTaskMonitorURI,
			)

			// The upgrade must succeed — no 412.
			Expect(err).NotTo(HaveOccurred(), "upgrade failed; If-Match header may have been sent")
			Expect(isFatal).To(BeFalse())
			Expect(taskURI).To(Equal("/redfish/v1/TaskService/Tasks/1"))

			// Explicitly assert that no If-Match header was sent.
			Expect(capturedIfMatch).To(BeEmpty(),
				"If-Match header must not be sent to a SimpleUpdate action endpoint")
		})
	})
})

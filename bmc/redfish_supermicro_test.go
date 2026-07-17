// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/stmcginnis/gofish/schemas"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func systemJSON(includeBootMode bool) []byte {
	boot := map[string]any{
		"BootSourceOverrideEnabled": "Disabled",
		"BootSourceOverrideTarget":  "None",
		"BootSourceOverrideTarget@Redfish.AllowableValues": []string{
			"None", "Pxe", "Hdd",
		},
	}
	if includeBootMode {
		boot["BootSourceOverrideMode"] = "UEFI"
	}
	sys := map[string]any{
		"@odata.id":   "/redfish/v1/Systems/1",
		"@odata.type": "#ComputerSystem.v1_3_0.ComputerSystem",
		"Id":          "1",
		"Name":        "System 1",
		"UUID":        "00000000-0000-0000-0000-000000000001",
		"Boot":        boot,
	}
	b, _ := json.Marshal(sys)
	return b
}

func supermicroSystemMux(includeBootMode bool) (*http.ServeMux, *json.RawMessage, *sync.Mutex) {
	var (
		mu      sync.Mutex
		patched json.RawMessage
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(serviceRootJSON()) //nolint:errcheck
	})
	mux.HandleFunc("/redfish/v1/Systems/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			patched = append(json.RawMessage(nil), body...)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(systemJSON(includeBootMode)) //nolint:errcheck
	})
	return mux, &patched, &mu
}

func newTestSupermicroBMC(server *httptest.Server) *SupermicroRedfishBMC {
	base := newTestRedfishBMC(server)
	base.options.ResourcePollingInterval = 10 * time.Millisecond
	base.options.ResourcePollingTimeout = 5 * time.Second
	return &SupermicroRedfishBMC{RedfishBaseBMC: base}
}

var _ = Describe("SupermicroRedfishBMC SetBootOverride", func() {
	const systemURI = "/redfish/v1/Systems/1"

	It("should omit BootSourceOverrideMode when the BMC does not advertise it", func(ctx SpecContext) {
		mux, patched, mu := supermicroSystemMux(false)
		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestSupermicroBMC(server)
		Expect(bmc.SetBootOverride(ctx, systemURI)).To(Succeed())

		mu.Lock()
		body := append(json.RawMessage(nil), *patched...)
		mu.Unlock()
		Expect(body).NotTo(BeEmpty(), "expected a PATCH to /redfish/v1/Systems/1")

		var payload struct {
			Boot map[string]json.RawMessage `json:"Boot"`
		}
		Expect(json.Unmarshal(body, &payload)).To(Succeed())
		Expect(payload.Boot).To(HaveKey("BootSourceOverrideEnabled"))
		Expect(payload.Boot).To(HaveKey("BootSourceOverrideTarget"))
		Expect(payload.Boot).NotTo(HaveKey("BootSourceOverrideMode"),
			"BootSourceOverrideMode must not be sent on BMCs that don't advertise it — regression guard for #982")

		var typed struct {
			Boot schemas.Boot `json:"Boot"`
		}
		Expect(json.Unmarshal(body, &typed)).To(Succeed())
		Expect(typed.Boot.BootSourceOverrideEnabled).To(Equal(schemas.OnceBootSourceOverrideEnabled))
		Expect(typed.Boot.BootSourceOverrideTarget).To(Equal(schemas.PxeBootSource))
	})

	It("should include BootSourceOverrideMode=UEFI when the BMC advertises it", func(ctx SpecContext) {
		mux, patched, mu := supermicroSystemMux(true)
		server := httptest.NewServer(mux)
		defer server.Close()

		bmc := newTestSupermicroBMC(server)
		Expect(bmc.SetBootOverride(ctx, systemURI)).To(Succeed())

		mu.Lock()
		body := append(json.RawMessage(nil), *patched...)
		mu.Unlock()
		Expect(body).NotTo(BeEmpty(), "expected a PATCH to /redfish/v1/Systems/1")

		var typed struct {
			Boot schemas.Boot `json:"Boot"`
		}
		Expect(json.Unmarshal(body, &typed)).To(Succeed())
		Expect(typed.Boot.BootSourceOverrideMode).To(Equal(schemas.UEFIBootSourceOverrideMode))
		Expect(typed.Boot.BootSourceOverrideEnabled).To(Equal(schemas.OnceBootSourceOverrideEnabled))
		Expect(typed.Boot.BootSourceOverrideTarget).To(Equal(schemas.PxeBootSource))
	})
})

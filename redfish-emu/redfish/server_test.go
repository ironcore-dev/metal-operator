package redfish_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ironcore-dev/metal-operator/redfish-emu/driver/fake"
	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
	"github.com/ironcore-dev/metal-operator/redfish-emu/redfish"
)

// newTestServer wires the Redfish server to a single fake-backed system.
func newTestServer(t *testing.T) (*httptest.Server, hypervisor.Hypervisor) {
	t.Helper()
	hyp := fake.New()
	t.Cleanup(func() { _ = hyp.Close() })
	srv := redfish.NewServer(redfish.Config{
		Systems: []redfish.System{{ID: "1", Name: "Test System", Hyp: hyp}},
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return ts, hyp
}

func do(t *testing.T, ts *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body != "" {
		r, err = http.NewRequest(method, ts.URL+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r, err = http.NewRequest(method, ts.URL+path, nil)
	}
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return v
}

func TestServiceRoot(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	m := decodeBody[map[string]any](t, resp)
	if m["@odata.id"] != "/redfish/v1" {
		t.Errorf("@odata.id = %v", m["@odata.id"])
	}
	sys, _ := m["Systems"].(map[string]any)
	if sys["@odata.id"] != "/redfish/v1/Systems" {
		t.Errorf("Systems link = %v", sys)
	}
}

func TestSystemsCollection(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1/Systems", "")
	m := decodeBody[map[string]any](t, resp)
	if got := m["Members@odata.count"]; got != float64(1) {
		t.Errorf("count = %v, want 1", got)
	}
}

func TestSystemGet_DefaultState(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1/Systems/1", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	m := decodeBody[map[string]any](t, resp)
	if m["PowerState"] != "Off" {
		t.Errorf("PowerState = %v, want Off", m["PowerState"])
	}
	boot := m["Boot"].(map[string]any)
	allow, _ := boot["BootSourceOverrideTarget@Redfish.AllowableValues"].([]any)
	found := false
	for _, v := range allow {
		if v == "UefiHttp" {
			found = true
		}
	}
	if !found {
		t.Errorf("UefiHttp not in allowable targets: %v", allow)
	}
}

func TestSystemGet_NotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1/Systems/99", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	m := decodeBody[map[string]any](t, resp)
	if _, ok := m["error"]; !ok {
		t.Errorf("expected DMTF error envelope, got %v", m)
	}
}

func TestPatchBoot_UefiHttp(t *testing.T) {
	ts, hyp := newTestServer(t)
	body := `{"Boot":{"BootSourceOverrideEnabled":"Once","BootSourceOverrideTarget":"UefiHttp","HttpBootUri":"http://10.0.2.2:8080/boot.efi"}}`
	resp := do(t, ts, "PATCH", "/redfish/v1/Systems/1", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, err := hyp.GetBootOverride(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != hypervisor.BootUefiHttp || got.Enabled != hypervisor.OverrideOnce ||
		got.HTTPBootURI != "http://10.0.2.2:8080/boot.efi" {
		t.Errorf("recorded override = %+v", got)
	}
}

func TestPatchBoot_UefiHttpMissingURI(t *testing.T) {
	ts, _ := newTestServer(t)
	body := `{"Boot":{"BootSourceOverrideTarget":"UefiHttp"}}`
	resp := do(t, ts, "PATCH", "/redfish/v1/Systems/1", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPatchBoot_InvalidTarget(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "PATCH", "/redfish/v1/Systems/1", `{"Boot":{"BootSourceOverrideTarget":"Banana"}}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestReset_PowerOnOff(t *testing.T) {
	ts, hyp := newTestServer(t)

	resp := do(t, ts, "POST", "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", `{"ResetType":"On"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("On status = %d, want 204", resp.StatusCode)
	}
	if st, _ := hyp.GetPowerState(t.Context()); st != hypervisor.PowerOn {
		t.Errorf("power = %v, want On", st)
	}

	resp = do(t, ts, "POST", "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", `{"ResetType":"ForceOff"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ForceOff status = %d", resp.StatusCode)
	}
	if st, _ := hyp.GetPowerState(t.Context()); st != hypervisor.PowerOff {
		t.Errorf("power = %v, want Off", st)
	}
}

func TestReset_InvalidType(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "POST", "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", `{"ResetType":"Explode"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestVirtualMedia_InsertEject(t *testing.T) {
	ts, hyp := newTestServer(t)

	// Insert.
	resp := do(t, ts, "POST",
		"/redfish/v1/Managers/1/VirtualMedia/Cd/Actions/VirtualMedia.InsertMedia",
		`{"Image":"http://10.0.2.2:8080/boot.iso"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("insert status = %d", resp.StatusCode)
	}
	list, _ := hyp.ListMedia(t.Context())
	if len(list) != 1 || list[0].Image != "http://10.0.2.2:8080/boot.iso" || !list[0].Inserted {
		t.Fatalf("media after insert = %+v", list)
	}

	// GET reflects it.
	resp = do(t, ts, "GET", "/redfish/v1/Managers/1/VirtualMedia/Cd", "")
	m := decodeBody[map[string]any](t, resp)
	if m["Inserted"] != true || m["Image"] != "http://10.0.2.2:8080/boot.iso" {
		t.Errorf("media resource = %v", m)
	}

	// Eject.
	resp = do(t, ts, "POST",
		"/redfish/v1/Managers/1/VirtualMedia/Cd/Actions/VirtualMedia.EjectMedia", `{}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("eject status = %d", resp.StatusCode)
	}
	if list, _ := hyp.ListMedia(t.Context()); len(list) != 0 {
		t.Errorf("media after eject = %+v", list)
	}
}

func TestVirtualMedia_UnknownDevice(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1/Managers/1/VirtualMedia/Floppy", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestPatch_MalformedJSON(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "PATCH", "/redfish/v1/Systems/1", `{"Boot": `)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// ensure the response is valid JSON with the expected content type.
func TestContentType(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/redfish/v1", "")
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Errorf("body is not valid JSON: %s", buf.String())
	}
}

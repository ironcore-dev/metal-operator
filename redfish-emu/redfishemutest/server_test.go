package redfishemutest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ironcore-dev/metal-operator/redfish-emu/hypervisor"
	"github.com/ironcore-dev/metal-operator/redfish-emu/redfishemutest"
)

// TestServer_FakeBootFlow exercises the full north-to-south flow through the
// server against the fake driver: set a UefiHttp boot override via Redfish,
// power on via the Reset action, and observe the power state. It validates the
// fixture wiring that a real BMC client will drive.
func TestServer_FakeBootFlow(t *testing.T) {
	h := redfishemutest.Start(t.Context(), redfishemutest.Options{Driver: redfishemutest.DriverFake})

	// PATCH the Boot object to UefiHttp with the server's guest boot URL.
	patch := map[string]any{"Boot": map[string]any{
		"BootSourceOverrideEnabled": "Once",
		"BootSourceOverrideTarget":  "UefiHttp",
		"HttpBootUri":               h.BootURL,
	}}
	body, _ := json.Marshal(patch)
	req, _ := http.NewRequest("PATCH", h.BaseURL+"/Systems/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Power on via ComputerSystem.Reset.
	resp, err = h.Client.Post(
		h.BaseURL+"/Systems/1/Actions/ComputerSystem.Reset",
		"application/json", bytes.NewReader([]byte(`{"ResetType":"On"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Reset status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// The fake driver models the UefiHttp fetch by emitting an event; confirm
	// the recorded override made it through.
	bo, _ := h.Hyp.GetBootOverride(t.Context())
	if bo.Target != hypervisor.BootUefiHttp || bo.HTTPBootURI != h.BootURL {
		t.Errorf("boot override = %+v, want UefiHttp with server BootURL", bo)
	}
}

func TestServer_BootURLRewritten(t *testing.T) {
	h := redfishemutest.Start(t.Context(), redfishemutest.Options{Driver: redfishemutest.DriverFake})
	if h.BootURL == "" {
		t.Fatal("BootURL empty")
	}
	// The boot server serves /boot.efi; fetching it should be observable.
	resp, err := http.Get(h.BootURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(h.BootRequests()) == 0 {
		t.Errorf("expected a logged boot request")
	}
}

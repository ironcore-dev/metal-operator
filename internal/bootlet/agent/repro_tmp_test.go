package agent

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestReproRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	issuer := NewEdDSATokenIssuer(priv)
	tok, err := issuer.Issue("op-1")
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(tok, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("payload: %s", payload)
	var pretty any
	if err := json.Unmarshal(payload, &pretty); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(pub)
	if _, err := v.Verify(tok, AudienceRegistry); err != nil {
		t.Logf("Verify error: %v", err)
		t.Fail()
	}
}

package agent

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/ed25519"
)

const (
	IssuerName = "sbt/agents"

	AudienceRegistry    = "registry"
	AudienceAgentServer = "agent-server"

	ActionRegistryPull = "registry.pull"
	ActionAgentReport  = "agent.report"
)

type Claims struct {
	jwt.RegisteredClaims
	Scope Scope `json:"scope,omitempty"`
}

type Scope []ScopeItem

// MarshalJSON must stay on the value receiver: jwt marshals Claims as a
// non-addressable value, where a pointer-receiver MarshalJSON would be
// silently skipped and Scope would be encoded as a JSON array instead of a
// string. UnmarshalJSON, conversely, needs the pointer receiver to mutate.
func (s Scope) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Scope) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = ParseScope(str)
	return nil
}

func ParseScope(s string) Scope {
	var scope Scope
	for segment := range strings.SplitSeq(s, " ") {
		item := ParseScopeItem(segment)
		scope = append(scope, item)
	}
	return scope
}

func (s *Scope) String() string {
	var sb strings.Builder
	for i, item := range *s {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(item.String())
	}
	return sb.String()
}

type ScopeItem struct {
	Action    string
	Namespace string
}

func (i ScopeItem) String() string {
	if i.Namespace == "" {
		return i.Action
	}
	return fmt.Sprintf("%s:%s", i.Action, i.Namespace)
}

func ParseScopeItem(s string) ScopeItem {
	action, namespace, ok := strings.Cut(s, ":")
	if ok {
		return ScopeItem{Action: action, Namespace: namespace}
	}
	return ScopeItem{Action: action}
}

type TokenIssuer interface {
	// Issue issues a new token for the given server ID.
	Issue(serverID string) (string, error)
	// Validate validates whether the given token is still valid for the given server ID.
	Validate(serverID, token string) (bool, error)
}

type TokenVerifier interface {
	// Verify verifies the token, returning the claims.
	Verify(token, audience string) (*Claims, error)
}

type EdDSATokenIssuer struct {
	key             ed25519.PrivateKey
	operationWindow time.Duration
	minValidity     time.Duration
}

func NewEdDSATokenIssuer(key ed25519.PrivateKey) *EdDSATokenIssuer {
	return &EdDSATokenIssuer{
		key:             key,
		operationWindow: 4 * time.Hour,
	}
}

func operationScope(operationID string) Scope {
	return Scope{
		{Action: ActionRegistryPull, Namespace: operationID},
		{Action: ActionAgentReport, Namespace: operationID},
	}
}

func (i *EdDSATokenIssuer) Issue(operationID string) (string, error) {
	now := time.Now()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}

	claims := Claims{
		Issuer:    IssuerName,
		Subject:   operationID,
		Audience:  jwt.ClaimStrings{AudienceRegistry, AudienceAgentServer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.operationWindow)),
		ID:        base64.RawURLEncoding.EncodeToString(jti),
		Scope:     operationScope(operationID),
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(i.key)
}

func (i *EdDSATokenIssuer) Validate(serverID string, token string) (ok bool, err error) {
	var claims Claims
	if _, err := jwt.ParseWithClaims(token, &claims,
		func(t *jwt.Token) (any, error) { return i.key.Public(), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(IssuerName),
		jwt.WithSubject(serverID),
		jwt.WithAudience(AudienceRegistry, AudienceAgentServer),
	); err != nil {
		return false, err
	}
	if time.Until(claims.ExpiresAt.Time) < i.minValidity {
		return false, nil
	}
	if !slices.Equal(claims.Scope, operationScope(serverID)) {
		return false, nil
	}
	return true, nil
}

type Verifier struct {
	pub ed25519.PublicKey
}

func NewVerifier(pub ed25519.PublicKey) *Verifier {
	return &Verifier{
		pub: pub,
	}
}

func (v *Verifier) Verify(token, audience string) (*Claims, error) {
	var claims Claims
	if _, err := jwt.ParseWithClaims(token, &claims,
		func(t *jwt.Token) (any, error) { return v.pub, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(IssuerName),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
	); err != nil {
		return nil, err
	}
	return &claims, nil
}

package server

import (
	"github.com/ironcore-dev/metal-operator/internal/bootlet/agent"
	"github.com/ironcore-dev/metal-operator/internal/httpserver"
)

func DefaultAuthenticator(verifier *agent.Verifier) Authenticator {
	return httpserver.NewBearerTokenAuthenticator(&AgentTokenAuthenticator{
		verifier: verifier,
	})
}

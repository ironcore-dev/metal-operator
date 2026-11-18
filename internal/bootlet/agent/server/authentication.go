package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ironcore-dev/metal-operator/internal/bootlet/agent"
	"github.com/ironcore-dev/metal-operator/internal/httpserver"
)

type AuthenticationResponse struct {
	User User
}

type User interface {
	Name() string
}

type AgentTokenAuthenticator struct {
	verifier *agent.Verifier
}

func (a *AgentTokenAuthenticator) AuthenticateToken(_ context.Context, token string) (*AuthenticationResponse, bool, error) {
	claims, err := a.verifier.Verify(token, agent.AudienceAgentServer)
	if err != nil {
		if !errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, false, fmt.Errorf("verifying token: %w", err)
		}
		return nil, false, nil
	}

	return &AuthenticationResponse{
		User: &AgentUser{
			Claims: claims,
		},
	}, true, nil
}

type AgentUser struct {
	Claims *agent.Claims
}

func (a *AgentUser) Name() string {
	return a.Claims.Subject
}

type userKey struct{}

func UserInto(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

type Authenticator = httpserver.Authenticator[*AuthenticationResponse]

func WithAuthentication(handler http.Handler, auth Authenticator, errorHandler httpserver.ErrorHandler) http.Handler {
	return httpserver.WithAuthentication(
		auth,
		errorHandler,
		func(w http.ResponseWriter, req *http.Request, res *AuthenticationResponse) {
			req = req.WithContext(UserInto(req.Context(), res.User))
			handler.ServeHTTP(w, req)
		},
	)
}

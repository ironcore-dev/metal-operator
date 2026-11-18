package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type AuthenticationResponse struct {
	User User
}

type User interface {
	Name() string
}

type Authenticator[response any] interface {
	AuthenticateRequest(req *http.Request) (response, bool, error)
}

type TokenAuthenticator[response any] interface {
	AuthenticateToken(ctx context.Context, token string) (response, bool, error)
}

type BearerTokenAuthenticator[response any] struct {
	tokenAuthenticator TokenAuthenticator[response]
}

func NewBearerTokenAuthenticator[response any](tokenAuth TokenAuthenticator[response]) *BearerTokenAuthenticator[response] {
	return &BearerTokenAuthenticator[response]{tokenAuthenticator: tokenAuth}
}

func (a *BearerTokenAuthenticator[response]) AuthenticateRequest(req *http.Request) (response, bool, error) {
	token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !ok || len(token) == 0 {
		var zero response
		return zero, false, nil
	}

	return a.tokenAuthenticator.AuthenticateToken(req.Context(), token)
}

func WithAuthentication[response any](
	auth Authenticator[response],
	errorHandler ErrorHandler,
	handler func(w http.ResponseWriter, req *http.Request, res response),
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		resp, ok, err := auth.AuthenticateRequest(req)
		if err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("could not authenticate request: %w", ErrForbidden)
			}

			errorHandler.Handle(w, req, err)
			return
		}

		handler(w, req, resp)
	})
}

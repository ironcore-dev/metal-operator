// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/stmcginnis/gofish"
)

// Dialer creates BMC connections.
type Dialer interface {
	Dial(ctx context.Context, o Options) (BMC, error)
}

// DirectDialer creates a fresh basic-auth BMC connection on every Dial call.
type DirectDialer struct{}

func (DirectDialer) Dial(ctx context.Context, o Options) (BMC, error) {
	return NewRedfishBMCClient(ctx, o)
}

// SessionDialer dials using a shared SessionCache, reusing tokens across calls.
// On 401/403 it invalidates the cached session and retries once.
type SessionDialer struct {
	cache *SessionCache
}

func NewSessionDialer(cache *SessionCache) *SessionDialer {
	return &SessionDialer{cache: cache}
}

type redfishClientHolder interface {
	Client() *gofish.APIClient
}

func (p *SessionDialer) Dial(ctx context.Context, o Options) (BMC, error) {
	o.SessionCache = p.cache

	bmcClient, err := NewRedfishBMCClient(ctx, o)
	if err != nil {
		if IsSessionExpiredError(err) {
			p.cache.Invalidate(SessionCacheKey{Endpoint: o.Endpoint, Username: o.Username})
			bmcClient, err = NewRedfishBMCClient(ctx, o)
		}
		if err != nil {
			return nil, err
		}
	}

	if holder, ok := bmcClient.(redfishClientHolder); ok {
		httpClient := holder.Client().HTTPClient
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		httpClient.Transport = &retryRoundTripper{
			base:  base,
			cache: p.cache,
			key:   SessionCacheKey{Endpoint: o.Endpoint, Username: o.Username},
			opts:  o,
		}
	}

	return bmcClient, nil
}

// retryRoundTripper recovers from BMC-side session eviction (e.g. BMC restart)
// without surfacing a 401/403 to the caller.
type retryRoundTripper struct {
	base    http.RoundTripper
	cache   *SessionCache
	key     SessionCacheKey
	opts    Options
	retried atomic.Bool
}

func (r *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	if !r.retried.CompareAndSwap(false, true) {
		return resp, nil
	}
	_ = resp.Body.Close()

	r.cache.Invalidate(r.key)
	session, sessionErr := r.cache.GetOrCreate(req.Context(), r.opts)
	if sessionErr != nil {
		return nil, sessionErr
	}

	retryReq := req.Clone(req.Context())
	retryReq.Header.Set("X-Auth-Token", session.Token)
	return r.base.RoundTrip(retryReq)
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"context"
	"crypto/tls"
	"errors"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// SessionCacheKey identifies a cached Redfish session by endpoint and username.
type SessionCacheKey struct {
	Endpoint string
	Username string
}

type sessionCacheEntry struct {
	mu          sync.Mutex
	session     *gofish.Session
	expiresAt   time.Time
	insecureTLS bool
}

// SessionCache holds live Redfish session tokens keyed by endpoint+username.
type SessionCache struct {
	mu      sync.Mutex
	entries map[SessionCacheKey]*sessionCacheEntry
	ttl     time.Duration
}

// NewSessionCache returns a SessionCache with the given idle TTL.
// ttl must be positive.
func NewSessionCache(ttl time.Duration) *SessionCache {
	if ttl <= 0 {
		panic("bmc: NewSessionCache requires a positive TTL")
	}
	return &SessionCache{
		entries: make(map[SessionCacheKey]*sessionCacheEntry),
		ttl:     ttl,
	}
}

// GetOrCreate returns a valid Redfish session for the given options, reusing a
// cached token if one exists and has not expired.
func (c *SessionCache) GetOrCreate(ctx context.Context, opts Options) (*gofish.Session, error) {
	key := SessionCacheKey{Endpoint: opts.Endpoint, Username: opts.Username}

	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &sessionCacheEntry{}
		c.entries[key] = entry
	}
	c.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.session != nil && time.Now().Before(entry.expiresAt) {
		return entry.session, nil
	}

	session, bmcTTL, err := c.createSession(ctx, opts)
	if err != nil {
		return nil, err
	}
	ttl := c.ttl
	if bmcTTL > 0 && bmcTTL < ttl {
		ttl = bmcTTL
	}
	entry.session = session
	entry.expiresAt = time.Now().Add(ttl)
	entry.insecureTLS = opts.InsecureTLS
	return session, nil
}

// Invalidate evicts the cached session for the given key so the next call to
// GetOrCreate creates a fresh one.
func (c *SessionCache) Invalidate(key SessionCacheKey) {
	if c == nil {
		return
	}
	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()
	if !ok {
		return
	}
	entry.mu.Lock()
	entry.session = nil
	entry.expiresAt = time.Time{}
	entry.mu.Unlock()
}

// Close deletes all live server-side Redfish sessions and clears the cache.
// Should be called from the manager shutdown hook.
func (c *SessionCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	entries := make(map[SessionCacheKey]*sessionCacheEntry, len(c.entries))
	maps.Copy(entries, c.entries)
	c.entries = make(map[SessionCacheKey]*sessionCacheEntry)
	c.mu.Unlock()

	for key, entry := range entries {
		entry.mu.Lock()
		sess := entry.session
		insecureTLS := entry.insecureTLS
		entry.session = nil
		entry.mu.Unlock()

		if sess == nil || sess.ID == "" {
			continue
		}
		//nolint:gosec
		httpClient := &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureTLS},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, key.Endpoint+sess.ID, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("X-Auth-Token", sess.Token)
		resp, err := httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		cancel()
	}
}

// createSession establishes a new Redfish session, queries the BMC's advertised
// SessionTimeout to cap the cache TTL, then drops the transient client without
// calling Logout (which would immediately delete the session we just created).
func (c *SessionCache) createSession(ctx context.Context, opts Options) (*gofish.Session, time.Duration, error) {
	client, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint: opts.Endpoint,
		Username: opts.Username,
		Password: opts.Password,
		Insecure: opts.InsecureTLS,
	})
	if err != nil {
		return nil, 0, err
	}
	session, err := client.GetSession()
	if err != nil {
		client.Logout()
		return nil, 0, err
	}

	var bmcTTL time.Duration
	if ss, err := client.Service.SessionService(); err == nil && ss.SessionTimeout > 0 {
		bmcTTL = time.Duration(ss.SessionTimeout) * time.Second
	}

	client.HTTPClient.CloseIdleConnections()
	return session, bmcTTL, nil
}

// IsSessionExpiredError reports whether err is an HTTP 401 from the BMC,
// indicating the cached session token was invalidated server-side.
func IsSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	var redfishErr *schemas.Error
	if !errors.As(err, &redfishErr) {
		return false
	}
	return redfishErr.HTTPReturnedStatusCode == http.StatusUnauthorized
}

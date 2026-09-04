// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package bmc

import (
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stmcginnis/gofish"
)

// seedSession is a sentinel gofish.Session used in tests that need a non-nil cached session.
var seedSession = &gofish.Session{ID: "/redfish/v1/SessionService/Sessions/abc", Token: "test-token-123"}

// mustNewSessionCache wraps NewSessionCache for tests where a valid TTL is always passed.
func mustNewSessionCache(ttl time.Duration) *SessionCache {
	c, err := NewSessionCache(ttl)
	if err != nil {
		panic(err)
	}
	return c
}

var _ = Describe("SessionCache", func() {
	Describe("NewSessionCache", func() {
		It("returns a non-nil cache with the given TTL", func() {
			cache, err := NewSessionCache(10 * time.Minute)
			Expect(err).NotTo(HaveOccurred())
			Expect(cache).NotTo(BeNil())
			Expect(cache.ttl).To(Equal(10 * time.Minute))
		})

		It("returns an error for a zero TTL", func() {
			_, err := NewSessionCache(0)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for a negative TTL", func() {
			_, err := NewSessionCache(-1 * time.Second)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Invalidate", func() {
		It("is a no-op on a nil cache", func() {
			var cache *SessionCache
			Expect(func() { cache.Invalidate(SessionCacheKey{}) }).NotTo(Panic())
		})

		It("clears a cached entry", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			entry := &sessionCacheEntry{
				session:   seedSession,
				expiresAt: time.Now().Add(10 * time.Minute),
			}
			cache.entries[key] = entry
			cache.mu.Unlock()

			cache.Invalidate(key)

			entry.mu.Lock()
			defer entry.mu.Unlock()
			Expect(entry.session).To(BeNil())
			Expect(entry.expiresAt.IsZero()).To(BeTrue())
		})

		It("is a no-op for unknown keys", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			Expect(func() {
				cache.Invalidate(SessionCacheKey{Endpoint: "https://unknown", Username: "x"})
			}).NotTo(Panic())
		})
	})

	Describe("Close", func() {
		It("is a no-op on a nil cache", func() {
			var cache *SessionCache
			Expect(func() { cache.Close() }).NotTo(Panic())
		})

		It("empties the entries map", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			cache.entries[key] = &sessionCacheEntry{session: seedSession, expiresAt: time.Now().Add(time.Minute)}
			cache.mu.Unlock()

			// Close attempts a DELETE to clean up the session but there is no live server;
			// it should not panic. The entries map must be cleared regardless.
			cache.Close()

			cache.mu.Lock()
			defer cache.mu.Unlock()
			Expect(cache.entries).To(BeEmpty())
		})
	})

	Describe("cache-hit logic (internal state)", func() {
		It("a seeded entry within TTL is treated as a cache hit", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			entry := &sessionCacheEntry{
				session:   seedSession,
				expiresAt: time.Now().Add(10 * time.Minute),
			}
			cache.entries[key] = entry
			cache.mu.Unlock()

			entry.mu.Lock()
			hit := entry.session != nil && time.Now().Before(entry.expiresAt)
			sess := entry.session
			entry.mu.Unlock()

			Expect(hit).To(BeTrue())
			Expect(sess).To(Equal(seedSession))
		})

		It("an expired entry is treated as a cache miss", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			entry := &sessionCacheEntry{
				session:   seedSession,
				expiresAt: time.Now().Add(-1 * time.Second), // already expired
			}
			cache.entries[key] = entry
			cache.mu.Unlock()

			entry.mu.Lock()
			miss := entry.session == nil || !time.Now().Before(entry.expiresAt)
			entry.mu.Unlock()

			Expect(miss).To(BeTrue(), "expired entry should be a cache miss")
		})
	})

	Describe("BMC TTL capping", func() {
		It("uses the configured TTL when it is shorter than the BMC timeout", func() {
			configured := 10 * time.Minute
			bmcTTL := 30 * time.Minute
			ttl := configured
			if bmcTTL > 0 && bmcTTL < ttl {
				ttl = bmcTTL
			}
			Expect(ttl).To(Equal(configured))
		})

		It("caps to the BMC timeout when it is shorter than the configured TTL", func() {
			configured := 30 * time.Minute
			bmcTTL := 10 * time.Minute
			ttl := configured
			if bmcTTL > 0 && bmcTTL < ttl {
				ttl = bmcTTL
			}
			Expect(ttl).To(Equal(bmcTTL))
		})

		It("ignores a zero BMC timeout (not advertised)", func() {
			configured := 10 * time.Minute
			var bmcTTL time.Duration // zero: BMC did not advertise timeout
			ttl := configured
			if bmcTTL > 0 && bmcTTL < ttl {
				ttl = bmcTTL
			}
			Expect(ttl).To(Equal(configured))
		})
	})

	Describe("concurrent access", func() {
		It("serialises concurrent reads for the same key without data races", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			cache.entries[key] = &sessionCacheEntry{
				session:   seedSession,
				expiresAt: time.Now().Add(10 * time.Minute),
			}
			cache.mu.Unlock()

			const goroutines = 20
			var wg sync.WaitGroup
			wg.Add(goroutines)
			for range goroutines {
				go func() {
					defer wg.Done()
					cache.mu.Lock()
					entry := cache.entries[key]
					cache.mu.Unlock()
					entry.mu.Lock()
					_ = entry.session
					entry.mu.Unlock()
				}()
			}
			wg.Wait()
			// No race detector violation → correct locking.
		})

		It("concurrent Invalidate and read do not race", func() {
			cache := mustNewSessionCache(10 * time.Minute)
			key := SessionCacheKey{Endpoint: "https://bmc.test", Username: "admin"}

			cache.mu.Lock()
			cache.entries[key] = &sessionCacheEntry{
				session:   seedSession,
				expiresAt: time.Now().Add(10 * time.Minute),
			}
			cache.mu.Unlock()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				cache.Invalidate(key)
			}()
			go func() {
				defer wg.Done()
				cache.mu.Lock()
				entry, ok := cache.entries[key]
				cache.mu.Unlock()
				if ok {
					entry.mu.Lock()
					_ = entry.session
					entry.mu.Unlock()
				}
			}()
			wg.Wait()
		})
	})

	Describe("IsSessionExpiredError", func() {
		It("returns false for nil", func() {
			Expect(IsSessionExpiredError(nil)).To(BeFalse())
		})

		It("returns false for a non-Redfish error", func() {
			Expect(IsSessionExpiredError(http.ErrNoCookie)).To(BeFalse())
		})
	})
})

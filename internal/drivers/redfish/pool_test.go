package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBMC struct {
	mu          sync.Mutex
	nextSession int
	logins      []login
	deletes     []sessionDelete
	failToken1  bool
}

type login struct {
	username string
	password string
}

type sessionDelete struct {
	path  string
	token string
}

func (f *fakeBMC) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /redfish/v1/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail := f.failToken1 && r.Header.Get("X-Auth-Token") == "token-1"
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{
			"@odata.id":      "/redfish/v1/",
			"Systems":        map[string]string{"@odata.id": "/redfish/v1/Systems"},
			"SessionService": map[string]string{"@odata.id": "/redfish/v1/SessionService"},
			"Links": map[string]any{
				"Sessions": map[string]string{"@odata.id": "/redfish/v1/SessionService/Sessions"},
			},
		})
	})
	mux.HandleFunc("GET /redfish/v1/SessionService", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":      "/redfish/v1/SessionService",
			"SessionTimeout": 120,
			"Sessions":       map[string]string{"@odata.id": "/redfish/v1/SessionService/Sessions"},
		})
	})
	mux.HandleFunc("POST /redfish/v1/SessionService/Sessions", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			UserName string
			Password string
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		f.mu.Lock()
		f.nextSession++
		id := f.nextSession
		f.logins = append(f.logins, login{payload.UserName, payload.Password})
		f.mu.Unlock()
		w.Header().Set("X-Auth-Token", fmt.Sprintf("token-%d", id))
		w.Header().Set("Location", fmt.Sprintf("/redfish/v1/SessionService/Sessions/%d", id))
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("DELETE /redfish/v1/SessionService/Sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.deletes = append(f.deletes, sessionDelete{r.URL.Path, r.Header.Get("X-Auth-Token")})
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /redfish/v1/Systems", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{"Members": []any{}, "Members@odata.count": 0})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestPoolSharesSessionPerIdentity(t *testing.T) {
	bmc := &fakeBMC{}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	pool := NewPool()
	defer pool.Close()
	ctx := context.Background()
	opts := DialOptions{Username: "user", Password: "pass"}

	for range 3 {
		c, err := pool.Dial(ctx, srv.URL+"/", opts)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		if _, err := c.Systems(ctx); err != nil {
			t.Fatalf("Systems: %v", err)
		}
		_ = c.Close()
	}

	if got := len(bmc.logins); got != 1 {
		t.Fatalf("expected 1 login for shared identity, got %d", got)
	}

	c, err := pool.Dial(ctx, srv.URL, DialOptions{Username: "user", Password: "rotated"})
	if err != nil {
		t.Fatalf("Dial with rotated password: %v", err)
	}
	_ = c.Close()

	if got := len(bmc.logins); got != 2 {
		t.Fatalf("expected 2nd login for new password, got %d logins", got)
	}
	if got := bmc.logins[1]; got != (login{"user", "rotated"}) {
		t.Errorf("rotated login used wrong credentials: %+v", got)
	}
}

func TestPoolReauthenticatesOnUnauthorized(t *testing.T) {
	bmc := &fakeBMC{failToken1: true}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	pool := NewPool()
	defer pool.Close()

	c, err := pool.Dial(context.Background(), srv.URL, DialOptions{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	if got := len(bmc.logins); got != 2 {
		t.Fatalf("expected relogin after 401, got %d logins", got)
	}
}

func TestPoolSessionExpiry(t *testing.T) {
	bmc := &fakeBMC{}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	pool := NewPool(WithTTL(100 * time.Millisecond))
	defer pool.Close()
	ctx := context.Background()

	c, err := pool.Dial(ctx, srv.URL, DialOptions{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	time.Sleep(300 * time.Millisecond)

	if _, err := c.Systems(ctx); err != nil {
		t.Fatalf("Systems after expiry: %v", err)
	}
	if got := len(bmc.logins); got != 2 {
		t.Fatalf("expected relogin after TTL expiry, got %d logins", got)
	}
}

func TestPoolClose(t *testing.T) {
	bmc := &fakeBMC{}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	pool := NewPool()
	ctx := context.Background()

	c, err := pool.Dial(ctx, srv.URL, DialOptions{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	pool.Close()
	pool.Close() // idempotent

	if got := len(bmc.deletes); got != 1 {
		t.Fatalf("expected 1 session DELETE on close, got %d", got)
	}
	if d := bmc.deletes[0]; d.token != "token-1" || d.path != "/redfish/v1/SessionService/Sessions/1" {
		t.Errorf("unexpected DELETE: %+v", d)
	}

	if _, err := pool.Dial(ctx, srv.URL, DialOptions{Username: "user", Password: "pass"}); !errors.Is(err, ErrPoolClosed) {
		t.Errorf("Dial after close: expected ErrPoolClosed, got %v", err)
	}

	loginsBefore := len(bmc.logins)
	if _, err := c.Systems(ctx); err == nil || !strings.Contains(err.Error(), ErrPoolClosed.Error()) {
		t.Errorf("Systems on stale lease after close: expected ErrPoolClosed, got %v", err)
	}
	if got := len(bmc.logins); got != loginsBefore {
		t.Errorf("stale lease relogged in after close: %d additional logins", got-loginsBefore)
	}
}

func TestPoolSweepEvictsExpiredEntries(t *testing.T) {
	bmc := &fakeBMC{}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	ttl := 100 * time.Millisecond
	pool := NewPool(WithTTL(ttl))
	ctx := context.Background()
	opts := DialOptions{Username: "user", Password: "pass"}

	c, err := pool.Dial(ctx, srv.URL, opts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = c.Close()

	time.Sleep(3 * ttl)
	pool.sweep()

	if got := len(bmc.deletes); got != 1 {
		t.Fatalf("expected 1 session DELETE from sweep, got %d", got)
	}
	if len(pool.entries) != 0 {
		t.Errorf("expected entry eviction, got %d entries", len(pool.entries))
	}

	if _, err := pool.Dial(ctx, srv.URL, opts); err != nil {
		t.Fatalf("Dial after sweep: %v", err)
	}
	if got := len(bmc.logins); got != 2 {
		t.Errorf("expected fresh login after eviction, got %d logins", got)
	}
	pool.Close()
}

func TestPoolConcurrentDial(t *testing.T) {
	bmc := &fakeBMC{}
	srv := httptest.NewServer(bmc.handler())
	defer srv.Close()

	pool := NewPool()
	defer pool.Close()
	ctx := context.Background()
	opts := DialOptions{Username: "user", Password: "pass"}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	var logins atomic.Int32
	for range 16 {
		wg.Go(func() {
			c, err := pool.Dial(ctx, srv.URL, opts)
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = c.Close() }()
			if _, err := c.Systems(ctx); err != nil {
				errs <- err
			}
			logins.Store(int32(len(bmc.logins))) //nolint:gosec
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent use: %v", err)
	}
	if got := len(bmc.logins); got != 1 {
		t.Errorf("expected 1 login under concurrency, got %d", got)
	}
}

const relSession = "/redfish/v1/SessionService/Sessions/1"

func TestSessionDeleteURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr string
		link string
		want string
		err  bool
	}{
		{name: "relative path", addr: "https://bmc", link: relSession,
			want: "https://bmc" + relSession},
		{name: "trailing slash addr", addr: "https://bmc/", link: relSession,
			want: "https://bmc" + relSession},
		{name: "rootless link", addr: "https://bmc", link: strings.TrimPrefix(relSession, "/"),
			want: "https://bmc" + relSession},
		{name: "absolute same host", addr: "https://bmc", link: "https://bmc" + relSession,
			want: "https://bmc" + relSession},
		{name: "absolute other host", addr: "https://bmc", link: "https://evil" + relSession, err: true},
		{name: "no path", addr: "https://bmc", link: "https://bmc", err: true},
		{name: "query stripped", addr: "https://bmc", link: relSession + "?foo=bar",
			want: "https://bmc" + relSession},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sessionDeleteURL(tc.addr, tc.link)
			if (err != nil) != tc.err {
				t.Fatalf("error = %v, want error = %v", err, tc.err)
			}
			if err == nil && !strings.HasSuffix(got, "/Sessions/1") {
				t.Errorf("unexpected path in %q", got)
			}
			if err == nil && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

package redfish

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/stmcginnis/gofish"
	ctrl "sigs.k8s.io/controller-runtime"
)

var ErrPoolClosed = errors.New("redfish: pool is closed")

type Pool struct {
	mu      sync.Mutex
	entries map[poolKey]*poolEntry
	ttl     time.Duration
	closed  bool
	log     logr.Logger
}

// poolKey scopes tokens and keep-alive transports to one identity at one BMC.
type poolKey struct {
	Addr         string
	Username     string
	PasswordHash [32]byte
	Insecure     bool
}

const defaultTTL = 15 * time.Minute

func NewPool(opts ...PoolOption) *Pool {
	p := &Pool{
		entries: make(map[poolKey]*poolEntry),
	}
	for _, o := range opts {
		o(p)
	}
	if p.ttl == 0 {
		p.ttl = defaultTTL
	}
	if p.log.IsZero() {
		p.log = ctrl.Log.WithName("bmc").WithName("pool")
	}
	return p
}

type PoolOption func(*Pool)

func WithTTL(ttl time.Duration) PoolOption {
	return func(p *Pool) { p.ttl = ttl }
}

func WithLogger(log logr.Logger) PoolOption {
	return func(p *Pool) { p.log = log }
}

func (p *Pool) Dial(ctx context.Context, addr string, opts DialOptions) (Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	hc, err := p.authClient(addr, opts)
	if err != nil {
		cancel()
		return nil, err
	}

	apiClient, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:   normalizeAddr(addr),
		Insecure:   opts.InsecureTLS,
		HTTPClient: hc,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	return &lease{
		Client: &client{apiClient},
		cancel: cancel,
	}, nil
}

func (p *Pool) authClient(addr string, opts DialOptions) (*http.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, ErrPoolClosed
	}

	addr = normalizeAddr(addr)
	key := poolKey{
		Addr:         addr,
		Username:     opts.Username,
		PasswordHash: sha256.Sum256([]byte(opts.Password)),
		Insecure:     opts.InsecureTLS,
	}
	entry, ok := p.entries[key]
	if !ok {
		entry = &poolEntry{base: newBaseTransport(opts.InsecureTLS)}
		entry.src = &sessionSource{
			ttl: p.ttl,
			login: func(ctx context.Context) (*gofish.Session, time.Duration, error) {
				return loginRedfish(ctx, addr, opts, entry.base)
			},
		}
		p.entries[key] = entry
	}

	return &http.Client{Transport: &sessionTransport{
		base: entry.base,
		src:  entry.src,
	}}, nil
}

type lease struct {
	Client
	cancel context.CancelFunc
	once   sync.Once
}

func (l *lease) Close() error {
	l.once.Do(l.cancel)
	return l.Client.Close()
}

func (p *Pool) Start(ctx context.Context) error {
	ticker := time.NewTicker(p.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.Close()
			return nil
		case <-ticker.C:
			p.sweep()
		}
	}
}

func (p *Pool) sweep() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	expired := make(map[poolKey]*poolEntry)
	now := time.Now()
	for key, entry := range p.entries {
		entry.src.mu.Lock()
		expiredEntry := entry.src.session != nil && !now.Before(entry.src.expires)
		entry.src.mu.Unlock()
		if expiredEntry {
			expired[key] = entry
			delete(p.entries, key)
		}
	}
	p.mu.Unlock()

	p.deleteSessions(expired)
}

func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	entries := p.entries
	p.entries = make(map[poolKey]*poolEntry)
	p.mu.Unlock()

	p.deleteSessions(entries)
}

func (p *Pool) deleteSessions(entries map[poolKey]*poolEntry) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for key, entry := range entries {
		sess := entry.src.close()
		if sess == nil || sess.Token == "" {
			continue
		}
		wg.Add(1)
		go func(key poolKey, entry *poolEntry, sess *gofish.Session) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p.deleteSession(ctx, key, entry.base, sess)
		}(key, entry, sess)
	}
	wg.Wait()
}

func (p *Pool) deleteSession(ctx context.Context, key poolKey, base http.RoundTripper, sess *gofish.Session) {
	deleteURL, err := sessionDeleteURL(key.Addr, sess.ID)
	if err != nil {
		p.log.V(1).Info("Failed to build Redfish session delete URL", "endpoint", key.Addr, "error", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		p.log.V(1).Info("Failed to build Redfish session delete request", "endpoint", key.Addr, "error", err)
		return
	}
	req.Header.Set("X-Auth-Token", sess.Token)

	// gofish cannot delete a session it does not own. Do a bounded raw DELETE
	// instead. Redirects are refused and the session link is pinned to the
	// entry host, so a BMC cannot forward the token cross-host.
	httpClient := &http.Client{
		Transport: base,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		p.log.V(1).Info("Failed to delete Redfish session", "endpoint", key.Addr, "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound && (resp.StatusCode < 200 || resp.StatusCode > 299) {
		p.log.V(1).Info("Failed to delete Redfish session", "endpoint", key.Addr, "status", resp.StatusCode)
	}
}

func sessionDeleteURL(addr, sessionLink string) (string, error) {
	base, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("parsing endpoint: %w", err)
	}
	link, err := url.Parse(sessionLink)
	if err != nil {
		return "", fmt.Errorf("parsing session link: %w", err)
	}
	if link.IsAbs() && link.Host != base.Host {
		return "", fmt.Errorf("session link %q points to different host", sessionLink)
	}
	path := link.EscapedPath()
	if path == "" {
		return "", fmt.Errorf("session link %q has no path", sessionLink)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base.Path = path
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

type poolEntry struct {
	base *http.Transport
	src  *sessionSource
}

type sessionSource struct {
	mu      sync.Mutex
	session *gofish.Session
	expires time.Time
	ttl     time.Duration
	closed  bool
	login   func(context.Context) (*gofish.Session, time.Duration, error)
}

func (s *sessionSource) token(ctx context.Context) (*gofish.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrPoolClosed
	}
	if s.session != nil && time.Now().Before(s.expires) {
		return s.session, nil
	}
	sess, bmcTTL, err := s.login(ctx)
	if err != nil {
		return nil, err
	}
	s.session = sess
	s.expires = time.Now().Add(effectiveTTL(s.ttl, bmcTTL))
	return sess, nil
}

func (s *sessionSource) invalidateIfSame(used *gofish.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil && used != nil && s.session.Token == used.Token {
		s.session = nil
		s.expires = time.Time{}
	}
}

// close prevents any further logins and returns the session for cleanup.
func (s *sessionSource) close() *gofish.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	sess := s.session
	s.session = nil
	s.expires = time.Time{}
	return sess
}

func effectiveTTL(configured, bmcTTL time.Duration) time.Duration {
	ttl := configured
	if bmcTTL > 0 && bmcTTL < configured {
		ttl = bmcTTL
	}
	// Renew early so a session still valid locally cannot expire mid-flight.
	return ttl * 9 / 10
}

func newBaseTransport(insecure bool) *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecure,
		},
		ForceAttemptHTTP2: true,
		IdleConnTimeout:   90 * time.Second,
	}
}

type sessionTransport struct {
	base http.RoundTripper
	src  *sessionSource
}

func (t *sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	tok, err := t.src.token(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(withToken(req, tok))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	t.src.invalidateIfSame(tok)

	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	fresh, err := t.src.token(req.Context())
	if err != nil {
		return nil, err
	}
	retry := req.Clone(req.Context())
	if req.Body != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retry.Body = body
	}
	return t.base.RoundTrip(withToken(retry, fresh))
}

func withToken(req *http.Request, tok *gofish.Session) *http.Request {
	r := req.Clone(req.Context())
	r.Header.Set("X-Auth-Token", tok.Token)
	return r
}

func normalizeAddr(addr string) string {
	return strings.TrimRight(addr, "/")
}

func loginRedfish(
	ctx context.Context,
	addr string,
	opts DialOptions,
	base *http.Transport,
) (*gofish.Session, time.Duration, error) {
	apiClient, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:   addr,
		Username:   opts.Username,
		Password:   opts.Password,
		Insecure:   opts.InsecureTLS,
		HTTPClient: &http.Client{Transport: base},
	})
	if err != nil {
		return nil, 0, err
	}
	session, err := apiClient.GetSession()
	if err != nil {
		apiClient.Logout()
		return nil, 0, err
	}
	var bmcTTL time.Duration
	if ss, serr := apiClient.Service.SessionService(); serr == nil && ss.SessionTimeout > 0 {
		bmcTTL = time.Duration(ss.SessionTimeout) * time.Second
	}
	return session, bmcTTL, nil
}

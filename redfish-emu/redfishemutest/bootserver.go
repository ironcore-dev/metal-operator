package redfishemutest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// BootRequest records a single HTTP request the guest firmware (or boot loader)
// made against the boot server.
type BootRequest struct {
	Method    string
	Path      string
	UserAgent string
}

// bootServer is an HTTP file server over a set of in-memory artifacts that logs
// every request. Observing a request for the boot artifact is the definitive
// proof that the guest firmware performed a UEFI HTTP boot fetch.
type bootServer struct {
	ts        *httptest.Server
	artifacts map[string][]byte

	mu       sync.Mutex
	requests []BootRequest
}

// newBootServer starts a boot server serving the given artifacts, keyed by URL
// path (e.g. "/boot.efi").
func newBootServer(artifacts map[string][]byte) *bootServer {
	bs := &bootServer{artifacts: artifacts}
	bs.ts = httptest.NewServer(http.HandlerFunc(bs.handle))
	return bs
}

func (bs *bootServer) handle(w http.ResponseWriter, r *http.Request) {
	bs.mu.Lock()
	bs.requests = append(bs.requests, BootRequest{
		Method:    r.Method,
		Path:      r.URL.Path,
		UserAgent: r.UserAgent(),
	})
	bs.mu.Unlock()

	data, ok := bs.artifacts[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

// URL returns the host-facing base URL of the boot server (loopback).
func (bs *bootServer) URL() string { return bs.ts.URL }

// requestsFor returns the logged requests whose path equals p.
func (bs *bootServer) requestsFor(p string) []BootRequest {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	var out []BootRequest
	for _, req := range bs.requests {
		if req.Path == p {
			out = append(out, req)
		}
	}
	return out
}

// fetched reports whether any GET/HEAD request has been made for path p.
func (bs *bootServer) fetched(p string) bool {
	return len(bs.requestsFor(p)) > 0
}

func (bs *bootServer) close() { bs.ts.Close() }

// looksLikeFirmwareFetch reports whether the request's User-Agent identifies the
// UEFI HTTP boot client, distinguishing a genuine firmware fetch from an OS-level
// download.
func looksLikeFirmwareFetch(req BootRequest) bool {
	return strings.HasPrefix(req.UserAgent, "UefiHttpBoot")
}

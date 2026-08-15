package store

import (
	"context"
	"fmt"

	"github.com/ironcore-dev/metal-operator/sbt/server"
)

type serverEntry struct {
	server *server.Server
}

type Memory struct {
	entries map[string]*serverEntry
}

func NewMemory() *Memory {
	return &Memory{
		entries: make(map[string]*serverEntry),
	}
}

func (m *Memory) CreateServer(_ context.Context, srv *server.Server) error {
	if _, ok := m.entries[srv.ID]; ok {
		return fmt.Errorf("server %q %w", srv.ID, ErrAlreadyExists)
	}

	m.entries[srv.ID] = &serverEntry{
		server: srv,
	}
	return nil
}

func (m *Memory) GetServer(_ context.Context, id string) (*server.Server, error) {
	srv, ok := m.entries[id]
	if !ok {
		return nil, fmt.Errorf("server %q %w", id, ErrNotFound)
	}

	return srv.server, nil
}

func (m *Memory) ListServers(_ context.Context, filter *ServerFilter) ([]server.Server, error) {
	var pred func(*server.Server) bool
	if filter != nil {
		pred = func(server *server.Server) bool {
			if filter.HostID != "" && server.HostID != filter.HostID {
				return false
			}
			return true
		}
	}

	var res []server.Server
	for _, srv := range m.entries {
		if pred != nil && !pred(srv.server) {
			continue
		}

		res = append(res, *srv.server)
	}
	return res, nil
}

func (m *Memory) UpdateServer(_ context.Context, srv *server.Server) error {
	_, ok := m.entries[srv.ID]
	if !ok {
		return fmt.Errorf("server %q %w", srv.ID, ErrNotFound)
	}
	m.entries[srv.ID].server = srv
	return nil
}

func (m *Memory) DeleteServer(_ context.Context, id string) error {
	_, ok := m.entries[id]
	if !ok {
		return fmt.Errorf("server %q %w", id, ErrNotFound)
	}
	delete(m.entries, id)
	return nil
}

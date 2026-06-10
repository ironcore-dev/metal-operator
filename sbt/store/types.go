package store

import (
	"context"
	"errors"

	"github.com/ironcore-dev/metal-operator/sbt/server"
)

var (
	ErrAlreadyExists = errors.New("already exists")
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
)

type Store interface {
	CreateServer(ctx context.Context, srv *server.Server) error
	GetServer(ctx context.Context, id string) (*server.Server, error)
	ListServers(ctx context.Context, filter *ServerFilter) ([]server.Server, error)
	UpdateServer(ctx context.Context, srv *server.Server) error
	DeleteServer(ctx context.Context, id string) error
}

type ServerFilter struct {
	HostID string
}

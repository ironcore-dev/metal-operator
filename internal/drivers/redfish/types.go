package redfish

import (
	"context"

	"github.com/stmcginnis/gofish/schemas"
)

type DialOptions struct {
	Username    string
	Password    string
	InsecureTLS bool
}

type Dialer interface {
	Dial(ctx context.Context, addr string, opts DialOptions) (Client, error)
}

type Client interface {
	Systems(ctx context.Context) ([]*schemas.ComputerSystem, error)
	Close() error
}

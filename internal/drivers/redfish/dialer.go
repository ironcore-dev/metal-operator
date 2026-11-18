package redfish

import (
	"context"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type dialer struct{}

func (dialer) Dial(ctx context.Context, addr string, opts DialOptions) (Client, error) {
	apiClient, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint: addr,
		Username: opts.Username,
		Password: opts.Password,
		Insecure: opts.InsecureTLS,
	})
	if err != nil {
		return nil, err
	}
	return &client{apiClient}, nil
}

var defaultDialer = dialer{}

func Dial(ctx context.Context, addr string, opts DialOptions) (Client, error) {
	return defaultDialer.Dial(ctx, addr, opts)
}

type client struct {
	apiClient *gofish.APIClient
}

func (c *client) Systems(ctx context.Context) ([]*schemas.ComputerSystem, error) {
	return c.apiClient.WithContext(ctx).Service.Systems()
}

func (c *client) Close() error {
	c.apiClient.Logout()
	return nil
}

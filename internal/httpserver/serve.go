package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type ServeOptions struct {
	Listener net.Listener
}

type ServeOption func(*ServeOptions)

func WithListener(ln net.Listener) ServeOption {
	return func(o *ServeOptions) {
		o.Listener = ln
	}
}

func Serve(ctx context.Context, srv *http.Server, opts ...ServeOption) error {
	sOpts := &ServeOptions{}
	for _, opt := range opts {
		opt(sOpts)
	}

	resChan := make(chan error, 1)

	go func() {
		defer close(resChan)

		serve := srv.ListenAndServe
		if sOpts.Listener == nil {
			serve = func() error {
				return srv.Serve(sOpts.Listener)
			}
		}

		err := serve()
		if !errors.Is(err, http.ErrServerClosed) {
			resChan <- err
		} else {
			resChan <- nil
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-resChan
		return nil
	case err := <-resChan:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

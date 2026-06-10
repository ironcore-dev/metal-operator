package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/u-root/u-root/pkg/cmdline"
	"github.com/u-root/u-root/pkg/dhclient"
)

type Flags struct {
	ReportURL string
}

func parseFlags() (*Flags, error) {
	reportURL, ok := cmdline.Flag("reportURL")
	if !ok {
		return nil, fmt.Errorf("missing required flag --reportURL")
	}

	return &Flags{ReportURL: reportURL}, nil
}

func main() {
	flags, err := parseFlags()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	if err := run(flags); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func dhConfigure(ctx context.Context) error {
	ifs, err := dhclient.Interfaces("^e.*")
	if err != nil {
		allIfaces, _ := dhclient.Interfaces(".*")
		allIfaceNames := make([]string, len(allIfaces))
		for _, iface := range allIfaces {
			allIfaceNames = append(allIfaceNames, fmt.Sprintf("%#+v", iface.Attrs()))
		}
		return fmt.Errorf("getting interfaces: %w (available interfaces %v)", err, allIfaceNames)
	}

	var (
		errs       []error
		done       = make(chan struct{})
		closeOnce  sync.Once
		notifyDone = func() {
			closeOnce.Do(func() { close(done) })
		}
	)
	go func() {
		defer notifyDone()

		for res := range dhclient.SendRequests(ctx, ifs, true, true, dhclient.Config{
			Timeout: 15 * time.Second,
			Retries: 5,
			V4ServerAddr: &net.UDPAddr{
				IP:   net.IPv4bcast,
				Port: dhcpv4.ServerPort,
			},
			V6ServerAddr: &net.UDPAddr{
				IP:   net.ParseIP("ff02::1:2"),
				Port: dhcpv6.DefaultServerPort,
			},
		}, 30*time.Second) {
			if res.Err != nil {
				errs = append(errs, res.Err)
				continue
			}

			if err := res.Lease.Configure(); err != nil {
				errs = append(errs, err)
				continue
			}

			notifyDone()
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return errors.Join(errs...)
	}
}

func postBooted(ctx context.Context, reportURL string) error {
	body := struct {
		Booted bool
	}{Booted: true}
	reqData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reportURL, bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	resData, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d (%s)", res.StatusCode, string(resData))
	}
	return nil
}

func run(flags *Flags) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("DHConfigure")
	if err := dhConfigure(ctx); err != nil {
		return fmt.Errorf("dhconfigure: %w", err)
	}

	slog.Info("Posting booted", "ReportURL", flags.ReportURL)

	t := time.NewTicker(1 * time.Second)
Loop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := postBooted(ctx, flags.ReportURL); err != nil {
				slog.Error("Posting booted", "error", err)
				continue
			}

			break Loop
		}
	}

	slog.Info("Posted booted", "ReportURL", flags.ReportURL)
	slog.Info("Parking")
	select {
	case <-ctx.Done():
		return nil
	}
}

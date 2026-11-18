package load

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ironcore-dev/metal-operator/internal/bootlet/config"
	"github.com/ironcore-dev/metal-operator/internal/configdrive"
	"github.com/ironcore-dev/metal-operator/internal/configdrive/reader"
)

func Config(ctx context.Context) (*config.Config, error) {
	data, err := os.ReadFile(configdrive.InitrdUserdataPath)
	if err == nil {
		cfg := &config.Config{}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("unmarshalling config: %w", err)
		}
		return cfg, nil
	}

	rd, err := reader.Probe(ctx)
	if err != nil {
		return nil, fmt.Errorf("probing for config drive reader: %w", err)
	}
	defer func() { _ = rd.Close() }()

	rc, err := rd.FileSystem().Open(rd.UserdataPath())
	if err != nil {
		return nil, fmt.Errorf("opening userdata: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err = io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading userdata: %w", err)
	}
	cfg := &config.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	cfg.SourceDevice = rd.Device()
	return cfg, nil
}

package configdrive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cavaliergopher/cpio"
)

const (
	InitrdInstanceMetadataPath = "/agent/metadata.json"
	InitrdUserdataPath         = "/agent/userdata"
	InitrdNetworkDataPath      = "/agent/network_data.json"
)

type initrdMetadata struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

func BuildInitrd(
	args Args,
) ([]byte, error) {
	metadata, err := json.Marshal(&initrdMetadata{
		ID:       args.Metadata.ServerID,
		Hostname: args.Metadata.Hostname,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling instance metadata: %w", err)
	}

	userData, err := io.ReadAll(args.UserData)
	if err != nil {
		return nil, fmt.Errorf("reading user data: %w", err)
	}

	type initrdItem struct {
		name string
		data []byte
	}

	items := []initrdItem{
		{
			name: InitrdUserdataPath,
			data: userData,
		},
		{
			name: InitrdInstanceMetadataPath,
			data: metadata,
		},
	}
	if args.NetworkData != nil {
		networkData, err := io.ReadAll(args.NetworkData)
		if err != nil {
			return nil, fmt.Errorf("reading network data: %w", err)
		}

		items = append(items, initrdItem{name: InitrdNetworkDataPath, data: networkData})
	}

	var buf bytes.Buffer
	w := cpio.NewWriter(&buf)

	for _, item := range items {
		if err := w.WriteHeader(&cpio.Header{
			Name: item.name,
			Mode: 0600,
			Size: int64(len(item.data)),
		}); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("writing %s header: %w", item.name, err)
		}

		if _, err := w.Write(item.data); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("writing %s data: %w", item.name, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing initrd config drive writer: %w", err)
	}
	return buf.Bytes(), nil
}

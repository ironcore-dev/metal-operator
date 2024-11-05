package fmi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	*http.Client
	serverURL             string
	scanEndpoint          string
	settingsApplyEndpoint string
	versionUpdateEndpoint string
}

func (c *Client) Scan(_ context.Context, serverBIOSRef string) (ScanResult, error) {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return ScanResult{}, err
	}

	resp, err := c.Post(fmt.Sprintf("%s/%s", c.serverURL, c.scanEndpoint), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return ScanResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ScanResult{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var result ScanResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ScanResult{}, err
	}
	return result, nil
}

func (c *Client) SettingsApply(_ context.Context, serverBIOSRef string) error {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return err
	}

	resp, err := c.Post(
		fmt.Sprintf("%s/%s", c.serverURL, c.settingsApplyEndpoint), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return nil
}

func (c *Client) VersionUpdate(_ context.Context, serverBIOSRef string) error {
	jsonBody, err := json.Marshal(TaskPayload{ServerBIOSRef: serverBIOSRef})
	if err != nil {
		return err
	}

	resp, err := c.Post(
		fmt.Sprintf("%s/%s", c.serverURL, c.versionUpdateEndpoint), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return nil
}

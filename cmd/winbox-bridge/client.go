package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type TheiaClient struct {
	BaseURL    string
	Secret     string
	HTTPClient *http.Client
}

func (c *TheiaClient) ResolveLaunch(ctx context.Context, launchToken string) (launchCredentials, error) {
	var creds launchCredentials
	if strings.TrimSpace(c.BaseURL) == "" {
		return creds, fmt.Errorf("theia base URL not configured")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return creds, fmt.Errorf("bridge secret not configured")
	}
	body, err := json.Marshal(map[string]string{"launch_token": launchToken})
	if err != nil {
		return creds, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/v1/bridge/connector/launch", bytes.NewReader(body))
	if err != nil {
		return creds, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bridge "+strings.TrimSpace(c.Secret))

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return creds, fmt.Errorf("resolve launch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		message := strings.TrimSpace(payload["error"])
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return creds, fmt.Errorf("Theia bridge launch failed (%d): %s", resp.StatusCode, message)
	}
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return creds, fmt.Errorf("decode Theia launch response: %w", err)
	}
	if creds.IP == "" || creds.Username == "" || creds.Password == "" {
		return creds, fmt.Errorf("Theia launch response missing required credentials")
	}
	return creds, nil
}

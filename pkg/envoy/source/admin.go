package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// Admin returns a [Source] that fetches /config_dump from the Envoy admin API at
// baseURL, e.g. "http://localhost:19000". The request is bound to the ctx passed
// to ConfigDump.
func Admin(baseURL string) Source {
	return adminSource(baseURL)
}

type adminSource string

// ConfigDump issues GET <baseURL>/config_dump and returns the response body.
// Non-200 responses are returned as errors and their body is closed.
func (a adminSource) ConfigDump(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(a)+"/config_dump", nil)
	if err != nil {
		return nil, fmt.Errorf("building /config_dump request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /config_dump failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Envoy admin returned HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

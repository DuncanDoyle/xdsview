package source

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Admin returns a [Source] that fetches /config_dump from the Envoy admin API at
// baseURL, e.g. "http://localhost:19000". The request is bound to the ctx passed
// to ConfigDump.
func Admin(baseURL string) Source {
	return adminSource(baseURL)
}

type adminSource string

// noRedirects returns redirects as responses: the target may serve a filtered
// dump, which would break the complete capture.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

// ConfigDump issues GET <baseURL>/config_dump and returns the response body.
// Non-200 responses, redirects included, are returned as errors and their
// body is closed.
func (a adminSource) ConfigDump(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(a)+"/config_dump", nil)
	if err != nil {
		return nil, fmt.Errorf("building /config_dump request: %w", err)
	}
	resp, err := noRedirects.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /config_dump failed: %w", err)
	}
	if loc := resp.Header.Get("Location"); loc != "" && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("Envoy admin returned HTTP %d redirect to %q; not following it", resp.StatusCode, loc)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Envoy admin returned HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Capture reports [model.CaptureComplete]: the request has no filter or mask,
// and ConfigDump does not follow redirects.
func (a adminSource) Capture() model.Capture { return model.CaptureComplete }

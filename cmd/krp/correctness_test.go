package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source/kube"
)

// fixture returns the path of a hand-built fixture dump.
func fixture(name string) string {
	return filepath.Join(testdataDir, "fixtures", name, "envoy", "config_dump.json")
}

// wantOutput runs krp dump on args and checks that text and JSON output hold
// the given lines.
func wantOutput(t *testing.T, args []string, text, json []string) {
	t.Helper()
	out, _, err := runKrp(t, append([]string{"dump"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range text {
		if !strings.Contains(out, w) {
			t.Errorf("text lacks %q:\n%s", w, out)
		}
	}
	out, _, err = runKrp(t, append([]string{"dump", "-o", "json"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range json {
		if !strings.Contains(out, w) {
			t.Errorf("JSON lacks %s", w)
		}
	}
}

// TestDump_StaticBootstrap: a static listener with an inline route config
// is shown, marked static, with its routes.
func TestDump_StaticBootstrap(t *testing.T) {
	wantOutput(t, []string{"--file", fixture("static-bootstrap")},
		[]string{"health_listener [::]:9902 · active · static", "route /ready (exact)", "backend: local_service"},
		[]string{`"static": true`, `"id": "L0/C0/N0/RCI"`, `"reported": "RS0"`})
}

// TestDump_NetworkFilters: a chain without an HCM lists its network filters;
// so does an HCM chain with another network filter.
func TestDump_NetworkFilters(t *testing.T) {
	wantOutput(t, []string{"--file", fixture("tcp-proxy")},
		[]string{
			"└─ network filters: envoy.filters.network.tcp_proxy (TcpProxy) · no HTTP connection manager",
			"network filters: envoy.filters.network.rbac (RBAC), envoy.filters.network.http_connection_manager (HttpConnectionManager)",
			"backend: web_backend",
		},
		[]string{`"networkFilters"`, `"id": "L0/C0/N0"`, `"id": "L1/C0/N1"`})
}

// complete is a file source that reports a complete capture, as Admin does.
type complete struct{ source.Source }

func (complete) Capture() model.Capture { return model.CaptureComplete }

// TestDump_MissingRDS: scenario 00 without its RDS config shows the route
// config as missing, with a warning, when the capture is complete (live), and
// as not found from a file.
func TestDump_MissingRDS(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json"))
	if err != nil {
		t.Fatal(err)
	}
	var dump struct {
		Configs []map[string]any `json:"configs"`
	}
	if err := json.Unmarshal(data, &dump); err != nil {
		t.Fatal(err)
	}
	for _, s := range dump.Configs {
		delete(s, "dynamic_route_configs")
	}
	data, _ = json.Marshal(dump)
	path := filepath.Join(t.TempDir(), "config_dump.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	defer func(g func(string, string, kube.Options) source.Source, h func(context.Context, kube.Options) ([]model.Ref, error)) {
		kubeGateway, kubeHTTPRoutes = g, h
	}(kubeGateway, kubeHTTPRoutes)
	kubeGateway = func(string, string, kube.Options) source.Source { return complete{source.File(path)} }
	kubeHTTPRoutes = func(context.Context, kube.Options) ([]model.Ref, error) { return nil, nil }

	out, stderr, err := runKrp(t, "dump", "--gateway", "gw", "-n", "ns")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "route config listener~80 missing: no route config has this name in a complete dump") {
		t.Errorf("text:\n%s", out)
	}
	if !strings.Contains(stderr, `warning: listener "listener~80" filter chain "listener~80": route config "listener~80" is missing`) {
		t.Errorf("stderr = %q", stderr)
	}
	out, _, _ = runKrp(t, "dump", "--gateway", "gw", "-n", "ns", "-o", "json")
	for _, w := range []string{`"state": "missing"`, `"code": "rds-missing"`, `"subject": "L0/C0/N0/RC"`} {
		if !strings.Contains(out, w) {
			t.Errorf("JSON lacks %s", w)
		}
	}

	out, stderr, _ = runKrp(t, "dump", "--file", path)
	if !strings.Contains(out, "route config listener~80 not found") || strings.Contains(stderr, "missing") {
		t.Errorf("file input: want not found and no warning; stdout:\n%s\nstderr %q", out, stderr)
	}
}

// TestDump_DefaultFilterChain:the default chain is shown last, labelled,
// with its routes and their chains.
func TestDump_DefaultFilterChain(t *testing.T) {
	wantOutput(t, []string{"--file", fixture("default-chain")},
		[]string{"├─ chain known · TLS known.example.test", "└─ default chain fallback", "backend: fallback_backend", "1 envoy.filters.http.router  on"},
		[]string{`"id": "L0/Cd"`, `"default": true`, `"L0/Cd/N0/RC/V0/R0"`})
}

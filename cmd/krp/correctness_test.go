package main

import (
	"path/filepath"
	"strings"
	"testing"
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

// TestDump_DefaultFilterChain: the default chain is shown last, labelled,
// with its routes and their chains.
func TestDump_DefaultFilterChain(t *testing.T) {
	wantOutput(t, []string{"--file", fixture("default-chain")},
		[]string{"├─ chain known · TLS known.example.test", "└─ default chain fallback", "backend: fallback_backend", "1 envoy.filters.http.router  on"},
		[]string{`"id": "L0/Cd"`, `"default": true`, `"L0/Cd/N0/RC/V0/R0"`})
}

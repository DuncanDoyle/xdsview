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

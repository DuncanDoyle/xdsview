package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/DuncanDoyle/krp/pkg/envoy/model"
	"github.com/DuncanDoyle/krp/pkg/envoy/origin"
	"github.com/DuncanDoyle/krp/pkg/envoy/source"
	"github.com/DuncanDoyle/krp/pkg/envoy/source/kube"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

const testdataDir = "../../testdata"

// runKrp runs the root command in-process and returns stdout, stderr and the error.
func runKrp(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := newRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// goldenPath maps testdata/<corpus...>/<scenario>/envoy/[<version>/]config_dump.json
// to testdata/golden/<corpus...>/<scenario>[/<version>]<suffix>.json.
func goldenPath(t *testing.T, dump, suffix string) string {
	t.Helper()
	rel, err := filepath.Rel(testdataDir, filepath.Dir(dump))
	if err != nil {
		t.Fatal(err)
	}
	rel = strings.Replace(rel, string(filepath.Separator)+"envoy", "", 1)
	return filepath.Join(testdataDir, "golden", rel+suffix+".json")
}

// allDumps returns every config_dump.json under testdata/, excluding golden files.
func allDumps(t *testing.T) []string {
	t.Helper()
	var dumps []string
	err := filepath.WalkDir(testdataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "golden" {
			return filepath.SkipDir
		}
		if d.Name() == "config_dump.json" {
			dumps = append(dumps, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dumps) == 0 {
		t.Fatal("no config_dump.json files under testdata/")
	}
	return dumps
}

// checkGolden compares got with the golden file, or rewrites it with -update.
func checkGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file (run go test ./cmd/krp -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s (run go test ./cmd/krp -update and review the diff)", path)
	}
}

// checkEnvelope asserts the documented envelope shape independently of the golden file.
func checkEnvelope(t *testing.T, out string) {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if string(env["schemaVersion"]) != `"v1alpha1"` {
		t.Errorf("schemaVersion = %s, want \"v1alpha1\"", env["schemaVersion"])
	}
	if _, ok := env["snapshot"]; !ok {
		t.Error("missing snapshot")
	}
	if w := string(env["warnings"]); !strings.HasPrefix(w, "[") {
		t.Errorf("warnings = %s, want a JSON array", w)
	}
}

func TestDumpJSON_Golden(t *testing.T) {
	for _, dump := range allDumps(t) {
		t.Run(dump, func(t *testing.T) {
			out, _, err := runKrp(t, "dump", "--file", dump, "-o", "json")
			if err != nil {
				t.Fatalf("krp dump -o json: %v", err)
			}
			checkEnvelope(t, out)
			checkGolden(t, goldenPath(t, dump, ""), out)
		})
	}
}

func TestDumpJSON_Filtered(t *testing.T) {
	dump := filepath.Join(testdataDir, "scenarios", "06-multi-extproc", "envoy", "kgateway-v2.5.0", "config_dump.json")
	cases := []struct {
		suffix string
		args   []string
	}{
		{".route", []string{"--route", "api-example-com", "--route-ns", "kgateway-system"}},
		{".rule0", []string{"--route", "api-example-com", "--route-ns", "kgateway-system", "--rule", "0"}},
		{".nomatch", []string{"--route", "does-not-exist", "--route-ns", "kgateway-system"}},
	}
	for _, c := range cases {
		t.Run(c.suffix, func(t *testing.T) {
			args := append([]string{"dump", "--file", dump, "-o", "json"}, c.args...)
			out, _, err := runKrp(t, args...)
			if err != nil {
				t.Fatalf("krp %v: %v", args, err)
			}
			checkEnvelope(t, out)
			checkGolden(t, goldenPath(t, dump, c.suffix), out)
		})
	}
}

// TestDumpJSON_Warnings verifies that skipped sections reach both the JSON and stderr.
func TestDumpJSON_Warnings(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "config_dump.json")
	bad := `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.ListenersConfigDump","dynamic_listeners":"not-an-array"}]}`
	if err := os.WriteFile(dump, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runKrp(t, "dump", "--file", dump, "-o", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", env.Warnings)
	}
	if !strings.Contains(stderr, "warning: "+env.Warnings[0]) {
		t.Errorf("stderr %q does not repeat the warning %q", stderr, env.Warnings[0])
	}
}

func TestDumpOutput_Rejected(t *testing.T) {
	dump := filepath.Join(testdataDir, "scenarios", "00-simple", "envoy", "kgateway-v2.5.0", "config_dump.json")
	cases := []struct {
		name, want string
		args       []string
	}{
		{"json with interactive", "--interactive", []string{"-o", "json", "--interactive"}},
		{"unknown format", "text, json", []string{"-o", "yaml"}},
		{"negative rule", "rule index >= 0", []string{"--route", "r", "--route-ns", "n", "--rule=-2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := runKrp(t, append([]string{"dump", "--file", dump}, c.args...)...)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want an error mentioning %q", err, c.want)
			}
			// cobra prints usage to the Out writer, so check only that no dump was written.
			if strings.Contains(out, "schemaVersion") || strings.Contains(out, "listener") {
				t.Errorf("stdout has dump output: %q", out)
			}
		})
	}
}

// TestDumpText_Origins checks the origin lines in the text view: with --route,
// route names resolve against that HTTPRoute; route_source metadata needs nothing.
func TestDumpText_Origins(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"known route", []string{"--file", filepath.Join(testdataDir, "scenarios/02_7-single-policy/envoy/kgateway-v2.5.0/config_dump.json"),
			"--route", "api-example-com", "--route-ns", "kgateway-system"},
			[]string{"HTTPRoute kgateway-system/api-example-com · rule 0 · match 0",
				"ext_auth/kgateway-system/ext-authz ← GatewayExtension kgateway-system/ext-authz",
				"TrafficPolicy kgateway-system/ext-auth (extAuth)"}},
		{"route_source metadata", []string{"--file", filepath.Join(testdataDir, "scenarios/04-multi-rule/envoy/kgateway-v2.4.7-route-source/config_dump.json")},
			[]string{"HTTPRoute kgateway-system/api-example-com · rule 1 · match 1"}},
		{"rule by name", []string{"--file", filepath.Join(testdataDir, "scenarios/08-named-rules/envoy/kgateway-v2.5.0/config_dump.json"),
			"--route", "api-example-com", "--route-ns", "kgateway-system", "--rule", "a-b"},
			[]string{"HTTPRoute kgateway-system/api-example-com · rule 1 (a-b) · match 0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := runKrp(t, append([]string{"dump"}, c.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
		})
	}
	// Rule a-b selects one route, not the a or a-matcher-b rules.
	out, _, _ := runKrp(t, "dump", "--file", filepath.Join(testdataDir, "scenarios/08-named-rules/envoy/kgateway-v2.5.0/config_dump.json"),
		"--route", "api-example-com", "--route-ns", "kgateway-system", "--rule", "a-b")
	if n := strings.Count(out, "Route: "); n != 1 {
		t.Errorf("--rule a-b printed %d routes, want 1", n)
	}
}

// waf is a test plugin mapper that claims every ext_auth filter.
type waf struct{ origin.Mapper }

func (waf) HTTPFilter(f model.HTTPFilter) model.FilterOrigin {
	if !strings.HasPrefix(f.Name, "ext_auth/") {
		return model.FilterOrigin{}
	}
	return model.FilterOrigin{Certainty: model.Inferred, Kind: "waf", Extension: &model.Ref{Group: "acme.example", Kind: "WafPolicy", Namespace: "acme", Name: "edge"}}
}

// TestDumpJSON_PluginOrigin checks that a plugged-in mapper's origin reaches
// the JSON output, and the kgateway mapper still answers everything else.
func TestDumpJSON_PluginOrigin(t *testing.T) {
	extraMappers = []origin.Mapper{waf{origin.Chain()}}
	defer func() { extraMappers = nil }()
	dump := filepath.Join(testdataDir, "scenarios", "02_7-single-policy", "envoy", "kgateway-v2.5.0", "config_dump.json")
	out, _, err := runKrp(t, "dump", "--file", dump, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	checkEnvelope(t, out)
	checkGolden(t, goldenPath(t, dump, ".plugin"), out)
	if !strings.Contains(out, `"WafPolicy"`) || !strings.Contains(out, `"set_metadata"`) {
		t.Error("expected the plugin origin and an OSS filter origin in the output")
	}
}

// TestDumpLive_ListsHTTPRoutes runs the live path with the cluster replaced:
// the HTTPRoutes listed from it resolve route origins without --route, and a
// listing error only costs the origins (D-006).
func TestDumpLive_ListsHTTPRoutes(t *testing.T) {
	dump := filepath.Join(testdataDir, "scenarios", "04-multi-rule", "envoy", "kgateway-v2.5.0", "config_dump.json")
	defer func(g func(string, string, kube.Options) source.Source, h func(context.Context, kube.Options) ([]model.Ref, error)) {
		kubeGateway, kubeHTTPRoutes = g, h
	}(kubeGateway, kubeHTTPRoutes)
	kubeGateway = func(string, string, kube.Options) source.Source { return source.File(dump) }

	kubeHTTPRoutes = func(context.Context, kube.Options) ([]model.Ref, error) {
		return []model.Ref{{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com"}}, nil
	}
	out, _, err := runKrp(t, "dump", "--gateway", "gw", "-n", "kgateway-system")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"HTTPRoute kgateway-system/api-example-com · rule 0 · match 0",
		"HTTPRoute kgateway-system/api-example-com · rule 1 · match 1",
		"HTTPRoute kgateway-system/api-example-com · rule 2 · match 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	kubeHTTPRoutes = func(context.Context, kube.Options) ([]model.Ref, error) {
		return nil, errors.New(`httproutes.gateway.networking.k8s.io is forbidden`)
	}
	out, stderr, err := runKrp(t, "dump", "--gateway", "gw", "-n", "kgateway-system")
	if err != nil {
		t.Fatalf("a listing error must not fail the dump: %v", err)
	}
	if !strings.Contains(stderr, "warning: route origins unavailable") {
		t.Errorf("stderr lacks the warning: %q", stderr)
	}
	if strings.Contains(out, "HTTPRoute ") || !strings.Contains(out, "Route: ") {
		t.Errorf("want routes without origins, got:\n%s", out)
	}

	// A listing that never answers is cut off, and the dump still prints.
	defer func(d time.Duration) { listTimeout = d }(listTimeout)
	listTimeout = 50 * time.Millisecond
	kubeHTTPRoutes = func(ctx context.Context, _ kube.Options) ([]model.Ref, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	out, stderr, err = runKrp(t, "dump", "--gateway", "gw", "-n", "kgateway-system")
	if err != nil || !strings.Contains(out, "Route: ") || !strings.Contains(stderr, "deadline exceeded") {
		t.Errorf("stalled listing: err=%v stderr=%q", err, stderr)
	}
}

// TestDumpLive_RouteSkipsListing checks that --route names the HTTPRoute itself:
// no listing, and a lookalike HTTPRoute in the cluster can't make it ambiguous.
func TestDumpLive_RouteSkipsListing(t *testing.T) {
	dump := filepath.Join(testdataDir, "scenarios", "04-multi-rule", "envoy", "kgateway-v2.5.0", "config_dump.json")
	defer func(d func(string, string, kube.Options) source.Source, h func(context.Context, kube.Options) ([]model.Ref, error)) {
		kubeDeployment, kubeHTTPRoutes = d, h
	}(kubeDeployment, kubeHTTPRoutes)
	kubeDeployment = func(string, string, kube.Options) source.Source { return source.File(dump) }
	kubeHTTPRoutes = func(context.Context, kube.Options) ([]model.Ref, error) {
		t.Error("HTTPRoutes listed despite --route")
		return nil, nil
	}
	out, _, err := runKrp(t, "dump", "--deployment", "gw", "-n", "kgateway-system",
		"--route", "api-example-com", "--route-ns", "kgateway-system")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "HTTPRoute kgateway-system/api-example-com · rule 1 · match 1") {
		t.Errorf("missing route origin in:\n%s", out)
	}
}

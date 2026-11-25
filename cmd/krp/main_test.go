package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source/kube"
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
	if string(env["schemaVersion"]) != `"v1alpha2"` {
		t.Errorf("schemaVersion = %s, want \"v1alpha2\"", env["schemaVersion"])
	}
	for _, k := range []string{"snapshot", "chains"} {
		if s := string(env[k]); !strings.HasPrefix(s, "{") {
			t.Errorf("%s = %s, want a JSON object", k, s)
		}
	}
	for _, k := range []string{"claims", "warnings"} {
		if s := string(env[k]); !strings.HasPrefix(s, "[") {
			t.Errorf("%s = %s, want a JSON array", k, s)
		}
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

// TestDumpJSON_Projection (plan §9.3 test 11): with --route, no field outside
// selection refers to an ID absent from snapshot, and selection entries carry
// route and vhost names.
func TestDumpJSON_Projection(t *testing.T) {
	queries := [][]string{
		{"--route", "api-example-com", "--route-ns", "kgateway-system"},
		{"--route", "api-example-com", "--route-ns", "kgateway-system", "--rule", "1"},
		{"--route", "api-example-com-admin", "--route-ns", "kgateway-system"},
		{"--route", "orders", "--route-ns", "kgwctl-proxy"},
		{"--route", "does-not-exist", "--route-ns", "kgateway-system"},
	}
	var projected, named, selected, narrowedSelected int
	for _, dump := range allDumps(t) {
		for _, q := range queries {
			out, _, err := runKrp(t, append([]string{"dump", "--file", dump, "-o", "json"}, q...)...)
			if err != nil {
				t.Fatal(err)
			}
			var env struct {
				Snapshot  *model.Snapshot                   `json:"snapshot"`
				Chains    map[model.ID]model.EffectiveChain `json:"chains"`
				Claims    []provenance.Claim                `json:"claims"`
				Selection selection                         `json:"selection"`
				Warnings  []model.Warning                   `json:"warnings"`
			}
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatal(err)
			}
			present := ids(env.Snapshot)
			check := func(where string, id model.ID) {
				if id != "" && !present[id] {
					t.Errorf("%s %v: %s refers to %s, absent from snapshot", dump, q, where, id)
				}
			}
			for _, s := range env.Snapshot.RouteConfigs {
				check("route config reported", s.Reported)
			}
			for _, l := range env.Snapshot.Listeners {
				for _, fc := range l.FilterChains {
					if fc.HCM != nil {
						check("route config use", fc.HCM.RouteConfigUse.Source)
					}
				}
			}
			for id, c := range env.Chains {
				check("chains key", id)
				for _, e := range c.Filters {
					check("chain entry", e.Slot)
				}
			}
			for _, c := range env.Claims {
				check("claim subject", c.Subject)
				for _, e := range c.Evidence {
					check("evidence subject", e.Subject)
				}
				if l, ok := c.Assertion.(provenance.FilterLink); ok {
					check("filter link slot", l.Slot)
				}
			}
			for _, w := range env.Warnings {
				check("warning subject", w.Subject)
			}
			for _, id := range env.Selection.Routes {
				check("selected route", id)
				selected++
			}
			res, _ := build(parseFile(t, dump), request{})
			projected += len(res.ix.Claims()) - len(env.Claims)
			for _, r := range append(append([]routeName{}, env.Selection.Ambiguous...), env.Selection.Unresolved...) {
				if r.Name == "" || r.VirtualHost == "" {
					t.Errorf("%s %v: selection entry %+v lacks names", dump, q, r)
				}
				named++
			}
			for _, n := range env.Selection.Narrowed {
				if n.Name == "" || n.VirtualHost == "" || len(n.Kept) == 0 {
					t.Errorf("%s %v: narrowing %+v lacks names or kept readings", dump, q, n)
				}
				// D-022: a narrowing krp selects is printed, and only then.
				if n.Selected != slices.Contains(env.Selection.Routes, n.ID) {
					t.Errorf("%s %v: narrowing %s selected=%v, in routes=%v", dump, q, n.ID, n.Selected, !n.Selected)
				}
				if n.Selected {
					narrowedSelected++
				}
				named++
			}
		}
	}
	if projected == 0 || named == 0 || selected == 0 || narrowedSelected == 0 {
		t.Errorf("projected %d claims, checked %d named entries, %d selected routes, %d by D-022: the test checks nothing", projected, named, selected, narrowedSelected)
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
		Snapshot struct {
			Warnings []model.Warning `json:"warnings"`
		} `json:"snapshot"`
		Warnings []model.Warning `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Warnings) != 1 || env.Warnings[0].Code != model.WarnSectionSkipped || len(env.Snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %+v, snapshot warnings %+v; want one section-skipped, on the envelope only", env.Warnings, env.Snapshot.Warnings)
	}
	if !strings.Contains(stderr, "warning: "+env.Warnings[0].Message) {
		t.Errorf("stderr %q does not repeat the warning %q", stderr, env.Warnings[0].Message)
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
				"ext_auth/kgateway-system/ext-authz  enabled by route · GatewayExtension kgateway-system/ext-authz",
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
	if n := strings.Count(out, "─ route "); n != 1 {
		t.Errorf("--rule a-b printed %d routes, want 1", n)
	}
}

// TestDump_NoRoutesWarning warns when --route matches nothing.
func TestDump_NoRoutesWarning(t *testing.T) {
	_, stderr, err := runKrp(t, "dump", "--file", filepath.Join(testdataDir, "scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json"),
		"--route", "missing", "--route-ns", "nowhere")
	if err != nil || !strings.Contains(stderr, "warning: no routes found for HTTPRoute nowhere/missing") {
		t.Errorf("err=%v stderr=%q, want the no-routes warning", err, stderr)
	}
}

// TestDumpInteractive_NothingSelectable: with only a rejected listener there
// is no row to select, so -i prints the tree, including Envoy's error.
func TestDumpInteractive_NothingSelectable(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "config_dump.json")
	const rejected = `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
 "dynamic_listeners": [{"name": "listener~80", "error_state": {"details": "duplicate filter chain match"}}]}]}`
	if err := os.WriteFile(dump, []byte(rejected), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runKrp(t, "dump", "-i", "--file", dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "listener~80 · rejected · port 80 (inferred) · error: duplicate filter chain match") {
		t.Errorf("stdout lacks the rejected listener:\n%s", out)
	}
}

// waf is a test vendor module that claims every ext_auth filter slot, exactly,
// from its config.
type waf struct{}

func (waf) Name() string { return "acme" }

func (waf) Claims(snap *model.Snapshot) []provenance.Claim {
	var out []provenance.Claim
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil {
				continue
			}
			for _, f := range fc.HCM.HTTPFilters {
				if !strings.HasPrefix(f.Name, "ext_auth/") {
					continue
				}
				ref := &model.Ref{Group: "acme.example", Kind: "WafPolicy", Namespace: "acme", Name: "edge"}
				out = append(out, provenance.Claim{Subject: f.ID, Certainty: model.Exact,
					Assertion: provenance.ExtensionSource{Extension: ref, FilterKind: "waf"},
					Evidence:  []provenance.Evidence{{Kind: provenance.EvidenceStructure, Subject: f.ID, Field: "typedConfig", Object: ref}}})
			}
		}
	}
	return out
}

// TestDumpJSON_PluginOrigin checks that a plugged-in module's claims reach
// the JSON and text output beside the kgateway module's, and win where they
// are more certain.
func TestDumpJSON_PluginOrigin(t *testing.T) {
	extraModules = []provenance.Module{waf{}}
	defer func() { extraModules = nil }()
	dump := filepath.Join(testdataDir, "scenarios", "02_7-single-policy", "envoy", "kgateway-v2.5.0", "config_dump.json")
	out, _, err := runKrp(t, "dump", "--file", dump, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	checkEnvelope(t, out)
	checkGolden(t, goldenPath(t, dump, ".plugin"), out)
	if !strings.Contains(out, `"module": "acme"`) || !strings.Contains(out, `"filterKind": "set_metadata"`) {
		t.Error("expected the plugin's claim and a kgateway filter claim in the output")
	}
	text, _, err := runKrp(t, "dump", "--file", dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "ext_auth/kgateway-system/ext-authz  enabled by route · WafPolicy acme/edge · TrafficPolicy") {
		t.Errorf("text lacks the plugin's exact extension:\n%s", text)
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
	// Without the listing, route names stay ambiguous: no origin is resolved.
	if strings.Contains(out, "HTTPRoute kgateway-system/api-example-com · rule") || !strings.Contains(out, "─ route ") {
		t.Errorf("want routes without resolved origins, got:\n%s", out)
	}

	// A listing that never answers is cut off, and the dump still prints.
	defer func(d time.Duration) { listTimeout = d }(listTimeout)
	listTimeout = 50 * time.Millisecond
	kubeHTTPRoutes = func(ctx context.Context, _ kube.Options) ([]model.Ref, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	out, stderr, err = runKrp(t, "dump", "--gateway", "gw", "-n", "kgateway-system")
	if err != nil || !strings.Contains(out, "─ route ") || !strings.Contains(stderr, "deadline exceeded") {
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

// TestDump_DuplicateRDSName: an RDS name two dynamic route configs share is
// ambiguous in text and JSON, with a warning, and neither config is shown.
func TestDump_DuplicateRDSName(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "config_dump.json")
	const dup = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [{"name": "listener~80",
  "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "dup"}, "http_filters": [
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "dup", "virtual_hosts": [{"name": "first", "domains": ["*"]}]}},
  {"route_config": {"name": "dup", "virtual_hosts": [{"name": "second", "domains": ["*"]}]}}]}]}`
	if err := os.WriteFile(dump, []byte(dup), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runKrp(t, "dump", "--file", dump)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "route config dup ambiguous: several route configs have this name") || strings.Contains(out, "vhost") {
		t.Errorf("text output:\n%s", out)
	}
	if !strings.Contains(stderr, `warning: listener "listener~80" filter chain #0: route config "dup" is ambiguous`) {
		t.Errorf("stderr = %q", stderr)
	}
	out, _, err = runKrp(t, "dump", "--file", dump, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"state": "ambiguous"`, `"code": "rds-ambiguous"`, `"subject": "L0/C0/N0/RC"`, `"id": "RD1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON lacks %s:\n%s", want, out)
		}
	}
}

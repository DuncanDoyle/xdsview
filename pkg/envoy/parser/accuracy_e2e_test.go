package parser_test

import (
	"bytes"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/renderer"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// TestE2E_TrafficSplit checks the weighted backends of 07 on every version (#13).
func TestE2E_TrafficSplit(t *testing.T) {
	want := []model.WeightedCluster{
		{ID: "L0/C0/N0/RC/V0/R0/W0", Name: "kube_httpbin_httpbin_8000", Weight: 80},
		{ID: "L0/C0/N0/RC/V0/R0/W1", Name: "kube_httpbin_httpbin-v2_8000", Weight: 20},
	}
	for path, snap := range matcherSnapshots(t, "07-traffic-split") {
		r := firstRoute(t, snap)
		if !reflect.DeepEqual(r.WeightedClusters, want) || r.Cluster != "" {
			t.Errorf("%s: WeightedClusters = %v, Cluster = %q; want %v and no cluster", path, r.WeightedClusters, r.Cluster, want)
		}
	}
}

// filterType returns the Type of the HCM filter named name on listener~80.
func filterType(t *testing.T, snap *model.Snapshot, name string) string {
	t.Helper()
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil {
				continue
			}
			for _, f := range fc.HCM.HTTPFilters {
				if f.Name == name {
					return f.Type
				}
			}
		}
	}
	t.Fatalf("filter %s not found", name)
	return ""
}

// TestE2E_PolicyFilterTypes checks filter types on real OSS dumps, including an
// ext_proc filter wrapped in ExtensionWithMatcher.
func TestE2E_PolicyFilterTypes(t *testing.T) {
	cases := []struct{ scenario, filter, want string }{
		{"02_1-single-policy", "dynamic_modules/simple_mutations", "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"},
		{"02_7-single-policy", "ext_auth/kgateway-system/ext-authz", "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"},
		{"02_7-single-policy", "global_disable/ext_auth", "type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config"},
		{"02_8-single-policy", "ratelimit/kgateway-system/ratelimit", "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"},
		{"06-multi-extproc", "ext_proc/kgateway-system/ext-proc-a", "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"},
	}
	for _, c := range cases {
		for path, snap := range matcherSnapshots(t, c.scenario) {
			if got := filterType(t, snap, c.filter); got != c.want {
				t.Errorf("%s %s: Type = %q, want %q", path, c.filter, got, c.want)
			}
		}
	}
}

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestE2E_UpstreamFilters checks router upstream filters are parsed and
// rendered in order (#15). No recorded OSS dump has them, so the dump is the
// hand-built testdata/fixtures/upstream-extproc.
func TestE2E_UpstreamFilters(t *testing.T) {
	want := []string{"envoy.filters.http.upstream_ext_proc", "envoy.filters.http.upstream_codec"}
	data, err := os.ReadFile("../../../testdata/fixtures/upstream-extproc/envoy/config_dump.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := parser.Parse(bytes.NewReader(data), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	snap := res
	var got []string
	for _, f := range snap.Listeners[0].FilterChains[0].HCM.UpstreamFilters {
		got = append(got, f.Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("UpstreamFilters = %v, want %v", got, want)
	}
	out := ansiCodes.ReplaceAllString(renderer.Render(renderer.View{Snapshot: snap, Chains: chain.Effective(snap)}), "")
	if !strings.Contains(out, "router upstream filters (as configured): "+strings.Join(want, ", ")) {
		t.Errorf("rendered upstream filters missing or out of order:\n%s", out)
	}
}

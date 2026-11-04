package parser_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/pkg/envoy/model"
	"github.com/DuncanDoyle/krp/pkg/envoy/parser"
)

// accuracyDump has two listeners sharing route config "rc", a router with
// upstream filters, a matcher-wrapped ext_proc filter, a weighted route and a
// prefix_rewrite route. kgateway OSS emits none of the last three shapes.
const accuracyDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
  "dynamic_listeners": [` + listenerJSON + `,` + listenerJSONB + `]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump",
  "dynamic_route_configs": [{"route_config": {"name": "rc", "virtual_hosts": [{
   "name": "vh", "domains": ["*"], "routes": [
    {"name": "split", "match": {"prefix": "/"},
     "route": {"weighted_clusters": {"clusters": [
      {"name": "kube_ns_v1_8080", "weight": 80},
      {"name": "kube_ns_v2_8080", "weight": 20}]}}},
    {"name": "rewrite", "match": {"prefix": "/old"},
     "route": {"cluster": "kube_ns_svc_8080", "prefix_rewrite": "/new"}}
   ]}]}}]}
]}`

const listenerHCM = `"filters": [{"name": "envoy.filters.network.http_connection_manager",
 "typed_config": {"rds": {"route_config_name": "rc"}, "http_filters": [
  {"name": "ext_proc/ns/ep",
   "typed_config": {"@type": "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher",
    "extension_config": {"name": "ext_proc/ns/ep", "typed_config": {
     "@type": "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"}}}},
  {"name": "envoy.filters.http.router",
   "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
    "upstream_http_filters": [
     {"name": "late", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"}},
     {"name": "envoy.filters.http.upstream_codec", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.upstream_codec.v3.UpstreamCodec"}}]}}]}}]`

const listenerJSON = `{"name": "listener~80", "active_state": {"listener": {"name": "listener~80",
 "address": {"socket_address": {"address": "0.0.0.0", "port_value": 80}},
 "filter_chains": [{"name": "listener~80", ` + listenerHCM + `}]}}}`

const listenerJSONB = `{"name": "listener~81", "active_state": {"listener": {"name": "listener~81",
 "address": {"socket_address": {"address": "0.0.0.0", "port_value": 81}},
 "filter_chains": [{"name": "listener~81", ` + listenerHCM + `}]}}}`

func parseAccuracy(t *testing.T) *model.EnvoySnapshot {
	t.Helper()
	result, err := parser.Parse(strings.NewReader(accuracyDump))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings: %v", result.Warnings)
	}
	if len(result.Snapshot.Listeners) != 2 {
		t.Fatalf("listeners = %d, want 2", len(result.Snapshot.Listeners))
	}
	return result.Snapshot
}

func routesOf(l model.Listener) []model.Route {
	return l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
}

// TestParse_WeightedClusters covers traffic-split routes (#13).
func TestParse_WeightedClusters(t *testing.T) {
	split := routesOf(parseAccuracy(t).Listeners[0])[0]
	want := []model.WeightedCluster{{Name: "kube_ns_v1_8080", Weight: 80}, {Name: "kube_ns_v2_8080", Weight: 20}}
	if !slices.Equal(split.WeightedClusters, want) {
		t.Errorf("WeightedClusters = %v, want %v", split.WeightedClusters, want)
	}
	if split.Cluster != "" {
		t.Errorf("Cluster = %q, want empty for a split route", split.Cluster)
	}
}

// TestParse_PrefixRewrite covers prefix_rewrite (#11).
func TestParse_PrefixRewrite(t *testing.T) {
	r := routesOf(parseAccuracy(t).Listeners[0])[1]
	if r.Rewrite == nil || r.Rewrite.Prefix != "/new" || r.Rewrite.RegexPattern != "" {
		t.Errorf("Rewrite = %+v, want Prefix /new and no regex", r.Rewrite)
	}
}

// TestParse_UpstreamFiltersAndTypes covers router upstream filters (#15) and
// filter types, unwrapped from ExtensionWithMatcher.
func TestParse_UpstreamFiltersAndTypes(t *testing.T) {
	hcm := parseAccuracy(t).Listeners[0].FilterChains[0].HCM
	gotTypes := []string{hcm.HTTPFilters[0].Type, hcm.HTTPFilters[1].Type}
	wantTypes := []string{
		"type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor",
		"type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
	}
	if !slices.Equal(gotTypes, wantTypes) {
		t.Errorf("filter types = %v, want %v", gotTypes, wantTypes)
	}
	var names []string
	for _, f := range hcm.UpstreamFilters {
		names = append(names, f.Name)
	}
	if want := []string{"late", "envoy.filters.http.upstream_codec"}; !slices.Equal(names, want) {
		t.Errorf("UpstreamFilters = %v, want %v", names, want)
	}
	if hcm.UpstreamFilters[0].Type != wantTypes[0] {
		t.Errorf("upstream filter type = %q", hcm.UpstreamFilters[0].Type)
	}
}

// TestParse_SharedRouteConfigIsolated verifies that two listeners sharing a
// route config get independent copies of the new fields.
func TestParse_SharedRouteConfigIsolated(t *testing.T) {
	snap := parseAccuracy(t)
	a, b := routesOf(snap.Listeners[0]), routesOf(snap.Listeners[1])
	a[0].WeightedClusters[0].Weight = 1
	a[1].Rewrite.Prefix = "/mutated"
	if b[0].WeightedClusters[0].Weight != 80 {
		t.Error("mutating one listener's WeightedClusters changed the other's")
	}
	if b[1].Rewrite.Prefix != "/new" {
		t.Error("mutating one listener's Rewrite changed the other's")
	}
}

// compositeFilter is a matcher-wrapped Composite filter whose actions run the
// given filter types.
func compositeFilter(name string, actionTypes ...string) string {
	var matchers []string
	for _, t := range actionTypes {
		matchers = append(matchers, `{"on_match": {"action": {"typed_config": {
			"@type": "type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction",
			"typed_config": {"name": "x", "typed_config": {"@type": "`+t+`"}}}}}}`)
	}
	return `{"name": "` + name + `", "typed_config": {
		"@type": "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher",
		"extension_config": {"name": "c", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"}},
		"xds_matcher": {"matcher_list": {"matchers": [` + strings.Join(matchers, ",") + `]}}}}`
}

// TestParse_CompositeFilterType checks that a Composite filter takes its
// delegate's type only when every action runs the same type.
func TestParse_CompositeFilterType(t *testing.T) {
	const extProc = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"
	const extAuthz = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"
	dump := `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
	 {"name": "l", "active_state": {"listener": {"name": "l", "filter_chains": [{"filters": [
	  {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"http_filters": [` +
		compositeFilter("same", extProc, extProc) + `,` + compositeFilter("mixed", extProc, extAuthz) + `]}}]}]}}}]}]}`
	result, err := parser.Parse(strings.NewReader(dump))
	if err != nil {
		t.Fatal(err)
	}
	filters := result.Snapshot.Listeners[0].FilterChains[0].HCM.HTTPFilters
	if filters[0].Type != extProc {
		t.Errorf("same-type composite: Type = %q, want %q", filters[0].Type, extProc)
	}
	if want := "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"; filters[1].Type != want {
		t.Errorf("mixed composite: Type = %q, want %q", filters[1].Type, want)
	}
}

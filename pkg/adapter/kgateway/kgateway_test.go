package kgateway_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

const authzType = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"

func compose(t *testing.T, snap *model.Snapshot) *provenance.Index {
	t.Helper()
	ix, err := provenance.Compose(snap, kgateway.New())
	if err != nil {
		t.Fatal(err)
	}
	if w := ix.Warnings(); len(w) > 0 {
		t.Errorf("compose warnings: %v", w)
	}
	return ix
}

func parse(t *testing.T, dump string) *model.Snapshot {
	t.Helper()
	snap, err := parser.Parse(strings.NewReader(dump), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func mergeMD(field, name string) string {
	return `"metadata": {"filter_metadata": {"merge.TrafficPolicy.gateway.kgateway.dev": {"` + field + `": ["gateway.kgateway.dev/TrafficPolicy/ns/` + name + `"]}}}`
}

// levelsDump has merge metadata at listener, route config, vhost and route
// level, and two ext_authz filters; route r0 enables one, r1 both.
var levelsDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", ` + mergeMD("extAuth", "listener") + `, "filter_chains": [{"name": "listener~80", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
    {"name": "ext_auth/ns/one", "disabled": true, "typed_config": {"@type": "` + authzType + `"}},
    {"name": "ext_auth/ns/two", "disabled": true, "typed_config": {"@type": "` + authzType + `"}},
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "listener~80", ` + mergeMD("cors", "rc") + `, "virtual_hosts": [{"name": "listener~80~example_com", "domains": ["example.com"], ` + mergeMD("extProc", "vhost") + `, "routes": [
   {"name": "listener~80~example_com-route-0-httproute-web-ns-0-0-matcher-0", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_web_80"},
    "typed_per_filter_config": {"ext_auth/ns/one": {}}, ` + mergeMD("extAuth", "route") + `},
   {"name": "listener~80~example_com-route-1-httproute-web-ns-1-0-matcher-0", "match": {"prefix": "/b"}, "route": {"cluster": "kube_ns_web_80"},
    "typed_per_filter_config": {"ext_auth/ns/one": {}, "ext_auth/ns/two": {}}, ` + mergeMD("extAuth", "both") + `}]}]}}]}]}`

// TestPolicyOrder (Codex #4): two records that differ only by group keep one
// order, claim IDs and refs across parses (R7).
func TestPolicyOrder(t *testing.T) {
	md := func(group string) string {
		return `"merge.Policy.` + group + `": {"f": ["` + group + `/Policy/ns/p"]}`
	}
	dump := `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "rc", "metadata": {"filter_metadata": {` + md("b.example.dev") + `, ` + md("a.example.dev") + `}}, "virtual_hosts": []}}]}]}`
	var first []string
	for i := range 50 {
		var got []string
		for _, c := range compose(t, parse(t, dump)).Claims() {
			if p, ok := c.Assertion.(provenance.PolicyRecord); ok {
				got = append(got, c.ID+" "+c.Ref+" "+p.Policy.Group)
			}
		}
		if i == 0 {
			first = got
			if len(got) != 2 || !strings.HasSuffix(got[0], "a.example.dev") {
				t.Fatalf("records %v, want a.example.dev first", got)
			}
		} else if !slices.Equal(got, first) {
			t.Fatalf("parse %d: records %v, want %v", i, got, first)
		}
	}
}

// TestMergeLevels (R5): records at listener, route config and vhost level are
// PolicyRecord claims on those subjects, level-local. Scenario 10 is a real
// capture with records at every level.
func TestMergeLevels(t *testing.T) {
	snap := parse(t, levelsDump)
	ix := compose(t, snap)
	src := snap.Listeners[0].FilterChains[0].HCM.RouteConfigUse.Source
	cases := []struct {
		subject model.ID
		name    string
		level   provenance.Level
	}{
		{"L0", "listener", provenance.LevelListener},
		{src, "rc", provenance.LevelRouteConfig},
		{"L0/C0/N0/RC/V0", "vhost", provenance.LevelVirtualHost},
		{"L0/C0/N0/RC/V0/R0", "route", provenance.LevelRoute},
	}
	for _, c := range cases {
		res := provenance.Resolve[provenance.PolicyRecord](ix, c.subject)
		if len(res.Candidates) != 1 || res.Candidates[0].Policy.Name != c.name || res.Candidates[0].Level != c.level || res.Certainty != model.Exact {
			t.Errorf("%s records = %+v, want exact %s at %s", c.subject, res.Candidates, c.name, c.level)
		}
	}
}

// TestFilterLink (rule 4): one matching slot gives an inferred link naming the
// record and the slot; two give none and the record stays unplaced. A
// record at listener level links per route too.
func TestFilterLink(t *testing.T) {
	ix := compose(t, parse(t, levelsDump))
	links := func(route model.ID) map[string]model.ID {
		out := map[string]model.ID{}
		for _, c := range ix.About(route) {
			if l, ok := c.Assertion.(provenance.FilterLink); ok {
				if c.Certainty != model.Inferred {
					t.Errorf("%s certainty %s", c.ID, c.Certainty)
				}
				rec, _ := ix.Record(c)
				out[rec.Assertion.(provenance.PolicyRecord).Policy.Name] = l.Slot
			}
		}
		return out
	}
	r0 := links("L0/C0/N0/RC/V0/R0")
	if r0["route"] != "L0/C0/N0/F0" || r0["listener"] != "L0/C0/N0/F0" || len(r0) != 2 {
		t.Errorf("R0 links %v, want route and listener records on F0", r0)
	}
	if r1 := links("L0/C0/N0/RC/V0/R1"); len(r1) != 0 {
		t.Errorf("R1 links %v, want none: two ext_authz slots match", r1)
	}
	if res := provenance.Resolve[provenance.PolicyRecord](ix, "L0/C0/N0/RC/V0/R1"); len(res.Candidates) != 1 {
		t.Errorf("R1's record is gone: %+v", res)
	}
}

// TestFilterLink_HeaderModifiers: headerModifiers links to the header_mutation
// slot, whose HCM config kgateway writes as a HeaderMutationPerRoute.
func TestFilterLink_HeaderModifiers(t *testing.T) {
	dump := `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
    {"name": "envoy.extensions.filters.http.header_mutation", "disabled": true, "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute"}},
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "listener~80", "virtual_hosts": [{"name": "listener~80~example_com", "domains": ["example.com"], "routes": [
   {"name": "listener~80~example_com-route-0-httproute-web-ns-0-0-matcher-0", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_web_80"},
    "typed_per_filter_config": {"envoy.extensions.filters.http.header_mutation": {}}, ` + mergeMD("headerModifiers", "hm") + `}]}]}}]}]}`
	ix := compose(t, parse(t, dump))
	var slots []model.ID
	for _, c := range ix.About("L0/C0/N0/RC/V0/R0") {
		if l, ok := c.Assertion.(provenance.FilterLink); ok {
			slots = append(slots, l.Slot)
		}
	}
	if !slices.Equal(slots, []model.ID{"L0/C0/N0/F0"}) {
		t.Errorf("links to %v, want the header_mutation slot L0/C0/N0/F0", slots)
	}
}

// TestP1Fixes (R13): Phase 8 deferred #2, #4 and #5. Each fails on the
// replaced origin package (cmd/krp TestParity_P1Fixes).
func TestP1Fixes(t *testing.T) {
	snap := snaptest.P1()
	ix := compose(t, snap)
	web := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"}
	zero := 0
	// #2: --rule 0 selects the route whose rule name is api.v1.
	if sel := kgateway.Select(ix, snap, kgateway.Query{Route: web, RuleIndex: &zero}); !slices.Equal(sel.Routes, []model.ID{"L0/C0/N0/RC/V0/R0"}) {
		t.Errorf("#2: routes %v", sel.Routes)
	}
	if sel := kgateway.Select(ix, snap, kgateway.Query{Route: web, RuleName: "api.v1"}); !slices.Equal(sel.Routes, []model.ID{"L0/C0/N0/RC/V0/R0"}) {
		t.Errorf("#2 by name: routes %v", sel.Routes)
	}
	slotOf := func(route model.ID) []model.ID {
		var out []model.ID
		for _, c := range ix.About(route) {
			if l, ok := c.Assertion.(provenance.FilterLink); ok {
				out = append(out, l.Slot)
			}
		}
		return out
	}
	// #5: a Composite-wrapped rustformation filter gets its link.
	if got := slotOf("L0/C0/N0/RC/V0/R1"); !slices.Equal(got, []model.ID{"L0/C0/N0/F0"}) {
		t.Errorf("#5: links %v", got)
	}
	// #4: name and module disagree: no link.
	if got := slotOf("L0/C0/N0/RC/V0/R2"); len(got) != 0 {
		t.Errorf("#4: links %v", got)
	}
}

// markerVhost is a virtual host whose name holds a route-name marker.
const markerVhost = "listener~80~x-route-0-httproute-a"

// markerDump has one route of HTTPRoute ns/web under markerVhost.
var markerDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "listener~80", "virtual_hosts": [{"name": "` + markerVhost + `", "domains": ["*"], "routes": [
   {"name": "` + markerVhost + `-route-0-httproute-web-ns-0-0-matcher-0", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_web_80"}}]}]}}]}]}`

// TestRouteName_VhostMarker (Codex P1): a marker inside the virtual host name
// neither hides the route's source nor adds readings.
func TestRouteName_VhostMarker(t *testing.T) {
	ix := compose(t, parse(t, markerDump))
	web := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}
	res := provenance.Resolve[provenance.RouteSource](ix, "L0/C0/N0/RC/V0/R0")
	if res.Certainty != model.Inferred || len(res.Candidates) != 1 || res.Candidates[0].Route != web {
		t.Errorf("readings = %s %+v, want inferred %v only", res.Certainty, res.Candidates, web)
	}
	// A bare name has no virtual host, so its answer lists every reading.
	a := kgateway.Names().Name(provenance.NameRoute, markerVhost+"-route-0-httproute-web-ns-0-0-matcher-0")
	if !slices.Contains(a.Candidates, web) {
		t.Errorf("Names = %+v, want %v among the candidates", a, web)
	}
}

// TestRouteName_Unanchored: a route name without its virtual host's prefix
// still reads, but its candidate list is incomplete, so the route is a gap.
func TestRouteName_Unanchored(t *testing.T) {
	route := model.ID("L0/C0/N0/RC/V0/R0")
	for _, tc := range []struct {
		dump     string
		complete bool
		reason   provenance.GapReason
	}{
		{markerDump, true, ""},
		{strings.Replace(markerDump, markerVhost+"-route-0-", "other-route-0-", 1), false, provenance.ReasonIncomplete},
	} {
		ix := compose(t, parse(t, tc.dump))
		for _, in := range ix.Inputs(route) {
			if in.Subject != route || in.Kind != provenance.KindRouteSource {
				continue
			}
			if len(in.Candidates) == 0 || in.Complete != tc.complete || in.Reason != tc.reason {
				t.Errorf("route input = complete %t, %s/%s %d candidates; want complete %t, reason %q", in.Complete, in.Status, in.Reason, len(in.Candidates), tc.complete, tc.reason)
			}
		}
	}
}

// TestNames (R9): cluster and route names give certainty and candidates,
// each carrying its kind; an ambiguous route name lists every reading.
// Route config and listener names carry no object.
func TestNames(t *testing.T) {
	m := kgateway.Names()
	if a := m.Name(provenance.NameCluster, "kube_ns_web_8080"); a.Certainty != model.Inferred || !slices.Equal(a.Candidates, []model.Ref{{Kind: "Service", Namespace: "ns", Name: "web"}}) {
		t.Errorf("cluster = %+v", a)
	}
	for _, kind := range []provenance.NameKind{provenance.NameRouteConfig, provenance.NameListener} {
		if a := m.Name(kind, "listener~80"); a.Certainty != model.Unknown || a.Candidates != nil {
			t.Errorf("%s = %+v, want unknown", kind, a)
		}
	}
	a := m.Name(provenance.NameRoute, "listener~80~x-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0")
	var got []string
	for _, c := range a.Candidates {
		if c.Kind != "HTTPRoute" {
			t.Errorf("candidate kind %q", c.Kind)
		}
		got = append(got, c.Namespace+"/"+c.Name)
	}
	want := []string{"example-com-kgateway-system/api", "com-kgateway-system/api-example", "kgateway-system/api-example-com", "system/api-example-com-kgateway"}
	if a.Certainty != model.Ambiguous || !slices.Equal(got, want) {
		t.Errorf("route = %s %v, want ambiguous %v", a.Certainty, got, want)
	}
	if a := m.Name(provenance.NameCluster, "kube_ns_web"); a.Certainty != model.Unknown {
		t.Errorf("bad cluster = %+v", a)
	}
	if !slices.Equal(kgateway.CheckedMinors(), []string{"v2.4", "v2.5"}) {
		t.Errorf("minors %v", kgateway.CheckedMinors())
	}
	// A route name reads as a route only, so Guess passes its answer through.
	if g := provenance.Guess(m, "kube_ns_web_8080"); g.Certainty != model.Inferred {
		t.Errorf("Guess = %+v", g)
	}
}

// policyFieldExclusions are merge fields the table leaves out on purpose,
// with the reason. None today.
var policyFieldExclusions = map[string]string{}

// TestPolicyTable_CoversCorpus (R14, #30): every merge field in the corpus
// and fixtures is in the policy field table or a recorded exclusion, and the
// table names its kgateway version.
func TestPolicyTable_CoversCorpus(t *testing.T) {
	if kgateway.PolicyTableVersion == "" {
		t.Error("the policy table names no kgateway version")
	}
	var dumps []string
	for _, g := range []string{"scenarios/*/envoy/*/config_dump.json", "fixtures/*/envoy/config_dump.json"} {
		m, _ := filepath.Glob(filepath.Join("../../../testdata", g))
		dumps = append(dumps, m...)
	}
	seen := map[string]bool{}
	for _, dump := range dumps {
		f, err := os.Open(dump)
		if err != nil {
			t.Fatal(err)
		}
		snap, err := parser.Parse(f, parser.Options{})
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		ix := compose(t, snap)
		for _, c := range ix.Claims() {
			if p, ok := c.Assertion.(provenance.PolicyRecord); ok && p.Policy.Kind == "TrafficPolicy" {
				seen[p.Field] = true
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("no policy records in the corpus")
	}
	for field := range seen {
		if !slices.Contains(kgateway.PolicyFields(), field) && policyFieldExclusions[field] == "" {
			t.Errorf("merge field %q is neither in the table nor a recorded exclusion", field)
		}
	}
	t.Logf("fields in the corpus: %v", slices.Sorted(func(yield func(string) bool) {
		for f := range seen {
			if !yield(f) {
				return
			}
		}
	}))
}

// TestStaticListenerRDS (Codex r7 #1): route config a static listener fetches
// over RDS gets the claims a dynamic listener's does, and they cover its inputs.
func TestStaticListenerRDS(t *testing.T) {
	dump := `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "static_listeners": [
  {"listener": {"name": "boot", "filter_chains": [{"filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
    {"name": "ext_auth/ns/one", "disabled": true, "typed_config": {"@type": "` + authzType + `"}},
    {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "listener~80", ` + mergeMD("cors", "rc") + `, "virtual_hosts": [{"name": "listener~80~example_com", "domains": ["example.com"], "routes": [
   {"name": "listener~80~example_com-route-0-httproute-web-ns-0-0-matcher-0", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_web_80"},
    "typed_per_filter_config": {"ext_auth/ns/one": {}}, ` + mergeMD("extAuth", "route") + `}]}]}}]}]}`
	snap := parse(t, dump)
	ix := compose(t, snap)
	hcm := snap.Listeners[0].FilterChains[0].HCM
	vh := hcm.RouteConfig.VirtualHosts[0]
	route := vh.Routes[0].ID
	if r := provenance.Resolve[provenance.PolicyRecord](ix, hcm.RouteConfigUse.Source); len(r.Candidates) != 1 || r.Candidates[0].Policy.Name != "rc" {
		t.Errorf("source records = %+v, want rc", r.Candidates)
	}
	want := map[provenance.Gap]bool{}
	for _, g := range []provenance.Gap{
		{Subject: vh.ID, Kind: provenance.KindHostSource},
		{Subject: route, Kind: provenance.KindRouteSource},
		{Subject: route, Kind: provenance.KindBackendSource},
		{Subject: route, Kind: provenance.KindPolicyRecord, Slot: hcm.HTTPFilters[0].ID, Level: provenance.LevelRoute},
	} {
		want[g] = true
	}
	for _, in := range ix.Inputs(route) {
		if want[in.Gap] && in.Status != provenance.InputCovered {
			t.Errorf("input %+v is %s (%s), want covered", in.Gap, in.Status, in.Reason)
		}
		delete(want, in.Gap)
	}
	if len(want) > 0 {
		t.Errorf("missing inputs %v", want)
	}
}

// TestStaticListeners: bootstrap listeners and their route configs get no claims.
func TestStaticListeners(t *testing.T) {
	f, err := os.Open("../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	snap, err := parser.Parse(f, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ix := compose(t, snap)
	statics := 0
	for _, l := range snap.Listeners {
		if !l.Static {
			continue
		}
		statics++
		for _, c := range ix.Claims() {
			if c.Subject.Within(l.ID) {
				t.Errorf("claim %s on static %s", c.ID, c.Subject)
			}
		}
	}
	for _, s := range snap.RouteConfigs {
		if s.Kind == model.SourceStatic && len(ix.About(s.ID)) > 0 {
			t.Errorf("claims on static source %s", s.ID)
		}
	}
	if statics == 0 {
		t.Fatal("no static listeners")
	}
}

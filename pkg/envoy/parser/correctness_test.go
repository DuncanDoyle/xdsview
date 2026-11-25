package parser_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

const fixtures = "../../../testdata/fixtures/"

// TestStaticListeners: static listeners are parsed after the dynamic ones,
// marked Static and active. An inline route config records the static entry
// Envoy reports for it; a static entry no HCM uses stays a source.
func TestStaticListeners(t *testing.T) {
	snap := parseFile(t, fixtures+"static-bootstrap/envoy/config_dump.json")
	if len(snap.Listeners) != 1 {
		t.Fatalf("listeners = %d, want 1", len(snap.Listeners))
	}
	l := snap.Listeners[0]
	if l.ID != "L0" || l.Name != "health_listener" || !l.Static || l.State.Status != model.ListenerActive || l.Address != ":::9902" {
		t.Errorf("listener = %+v, want static active L0 health_listener on :::9902", l)
	}
	hcm := l.FilterChains[0].HCM
	if hcm == nil || hcm.RouteConfig == nil || hcm.RouteConfig.VirtualHosts[0].Routes[0].Cluster != "local_service" {
		t.Fatalf("hcm = %+v, want the inline route config joined", hcm)
	}
	// The reported static entry stays a source, so Reported never dangles.
	want := []model.RouteConfigSource{
		{ID: "L0/C0/N0/RCI", Kind: model.SourceInline, Name: "local_route", Reported: "RS0"},
		{ID: "RS0", Kind: model.SourceStatic, Name: "local_route"},
		{ID: "RS1", Kind: model.SourceStatic, Name: "spare_route"},
	}
	if !reflect.DeepEqual(snap.RouteConfigs, want) {
		t.Errorf("sources = %+v\nwant %+v", snap.RouteConfigs, want)
	}

	// Scenario 00: the dynamic listener keeps L0; the static ones follow.
	snap = parseFile(t, "../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
	var got []string
	for _, l := range snap.Listeners {
		got = append(got, string(l.ID)+" "+l.Name)
		if l.Static != (l.Name != "listener~80") {
			t.Errorf("%s: Static = %v", l.Name, l.Static)
		}
	}
	if want := []string{"L0 listener~80", "L1 readiness_listener", "L2 prometheus_listener"}; !reflect.DeepEqual(got, want) {
		t.Errorf("listeners = %v, want %v", got, want)
	}
}

// TestNetworkFilters: every network filter has an occurrence; a chain
// without an HCM is kept; the HCM has its network filter's ID.
func TestNetworkFilters(t *testing.T) {
	snap := parseFile(t, fixtures+"tcp-proxy/envoy/config_dump.json")
	tcp := snap.Listeners[0].FilterChains[0]
	want := []model.NetworkFilter{{ID: "L0/C0/N0", Name: "envoy.filters.network.tcp_proxy", Type: "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"}}
	if !reflect.DeepEqual(tcp.NetworkFilters, want) || tcp.HCM != nil {
		t.Errorf("tcp chain = %+v, want %+v and no HCM", tcp, want)
	}
	http := snap.Listeners[1].FilterChains[0]
	want = []model.NetworkFilter{
		{ID: "L1/C0/N0", Name: "envoy.filters.network.rbac", Type: "type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC"},
		{ID: "L1/C0/N1", Name: "envoy.filters.network.http_connection_manager", Type: "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"},
	}
	if !reflect.DeepEqual(http.NetworkFilters, want) || http.HCM == nil || http.HCM.ID != "L1/C0/N1" {
		t.Errorf("http chain filters = %+v, HCM %+v; want %+v with the HCM at N1", http.NetworkFilters, http.HCM, want)
	}

	// An HCM is found by its type, whatever its name.
	snap = parseString(t, `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
 {"name": "x", "active_state": {"listener": {"name": "x", "filter_chains": [{"filters": [{"name": "http",
  "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
   "rds": {"route_config_name": "rc"}, "http_filters": []}}]}]}}}]}]}`)
	if hcm := snap.Listeners[0].FilterChains[0].HCM; hcm == nil || hcm.RouteConfigName != "rc" {
		t.Errorf("HCM named http = %+v, want parsed", hcm)
	}

	// A skipped HCM still has its network filter occurrence (subject L0/C0).
	snap = parseString(t, `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [`+
		hcmListener("x", `"http_filters": 1`)+`]}]}`)
	if fc := snap.Listeners[0].FilterChains[0]; len(fc.NetworkFilters) != 1 || fc.NetworkFilters[0].ID != "L0/C0/N0" || fc.HCM != nil {
		t.Errorf("chain = %+v, want the skipped HCM listed as L0/C0/N0", fc)
	}
}

// scenario00 returns scenario 00 decoded, with edit applied to each section.
func scenario00(t *testing.T, edit func(section map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
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
		edit(s)
	}
	out, err := json.Marshal(dump)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

const routesType = "type.googleapis.com/envoy.admin.v3.RoutesConfigDump"

// TestMissingRDS: an active listener's RDS name with no dynamic source is
// missing, with a warning, only in a complete capture whose route configs
// were all read. Otherwise it is unknown (design §4 row 1, E14).
func TestMissingRDS(t *testing.T) {
	noRDS := func(s map[string]any) {
		if s["@type"] == routesType {
			delete(s, "dynamic_route_configs")
		}
	}
	cases := []struct {
		name    string
		capture model.Capture
		edit    func(map[string]any)
		want    model.RouteConfigState
	}{
		{"complete", model.CaptureComplete, noRDS, model.RouteConfigMissing},
		{"unverified", model.CaptureUnverified, noRDS, model.RouteConfigUnknown},
		{"no RoutesConfigDump", model.CaptureComplete, func(s map[string]any) {
			if s["@type"] == routesType {
				s["@type"] = "type.googleapis.com/example.Unknown"
			}
		}, model.RouteConfigUnknown},
		{"skipped entry", model.CaptureComplete, func(s map[string]any) {
			if s["@type"] == routesType {
				s["dynamic_route_configs"] = []any{map[string]any{"route_config": map[string]any{"name": "listener~80", "virtual_hosts": 1}}}
			}
		}, model.RouteConfigUnknown},
		{"draining", model.CaptureComplete, func(s map[string]any) {
			noRDS(s)
			moveState(s, "draining_state")
		}, model.RouteConfigUnknown},
		{"warming", model.CaptureComplete, func(s map[string]any) {
			noRDS(s)
			moveState(s, "warming_state")
		}, model.RouteConfigUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap, err := parser.Parse(strings.NewReader(scenario00(t, c.edit)), parser.Options{Capture: c.capture})
			if err != nil {
				t.Fatal(err)
			}
			hcm := snap.Listeners[0].FilterChains[0].HCM
			if got := hcm.RouteConfigUse.State; got != c.want {
				t.Errorf("state = %s, want %s", got, c.want)
			}
			var missing []model.Warning
			for _, w := range snap.Warnings {
				if w.Code == model.WarnRDSMissing {
					missing = append(missing, w)
				}
			}
			if wantWarn := c.want == model.RouteConfigMissing; (len(missing) == 1) != wantWarn ||
				wantWarn && missing[0].Subject != hcm.RouteConfigUse.ID {
				t.Errorf("rds-missing warnings = %+v, want one on %s: %v", missing, hcm.RouteConfigUse.ID, wantWarn)
			}
		})
	}
}

// TestMissingRDS_Static: Envoy dumps static listeners before its workers start
// (listener_manager_impl.cc, dumpListenerConfigs), so a static HCM's RDS name
// is missing only when an active dynamic listener proves they have started.
func TestMissingRDS_Static(t *testing.T) {
	static := `{"listener": {"name": "boot", "filter_chains": [{"filters": [
 {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "boot_route"}, "http_filters": [
  {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}`
	dynamic := `{"name": "d", "%s": {"listener": {"name": "d", "filter_chains": [{"filters": [
 {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "d_route"}, "http_filters": []}}]}]}}}`
	cases := []struct {
		name     string
		dynamics string
		want     model.RouteConfigState
	}{
		{"no dynamic listener", ``, model.RouteConfigUnknown},
		{"only warming", fmt.Sprintf(dynamic, "warming_state"), model.RouteConfigUnknown},
		{"active dynamic", fmt.Sprintf(dynamic, "active_state"), model.RouteConfigMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dump := `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "static_listeners": [` + static + `], "dynamic_listeners": [` + c.dynamics + `]},
 {"@type": "` + routesType + `", "dynamic_route_configs": [{"route_config": {"name": "d_route"}}]}]}`
			snap, err := parser.Parse(strings.NewReader(dump), parser.Options{Capture: model.CaptureComplete})
			if err != nil {
				t.Fatal(err)
			}
			var boot *model.HCMConfig
			for _, l := range snap.Listeners {
				if l.Static {
					boot = l.FilterChains[0].HCM
				}
			}
			if got := boot.RouteConfigUse.State; got != c.want {
				t.Errorf("static HCM state = %s, want %s", got, c.want)
			}
		})
	}
}

// moveState moves the dynamic listeners' active_state to field.
func moveState(s map[string]any, field string) {
	ls, _ := s["dynamic_listeners"].([]any)
	for _, l := range ls {
		m := l.(map[string]any)
		m[field] = m["active_state"]
		delete(m, "active_state")
	}
}

// TestECDS: a filter with config_discovery takes the ECDS config whose name is
// the filter's name and whose type is in type_urls (http_connection_manager.proto,
// HttpFilter.name: "also serves as a resource name in ExtensionConfigDS"). With
// none, an active listener runs default_config, whether or not it waits
// (config_source.proto, ExtensionConfigSource); otherwise it is unresolved.
func TestECDS(t *testing.T) {
	snap := parseFile(t, fixtures+"ecds/envoy/config_dump.json")
	const lua = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"
	want := map[string][2]string{ // name: source, type
		"ecds.found":              {model.ConfigECDS, lua},
		"ecds.wrong_type":         {"", ""},
		"ecds.default":            {model.ConfigECDSDefault, lua},
		"ecds.default_no_warming": {model.ConfigECDSDefault, lua},
		"ecds.none":               {"", ""},
		"ecds.matched":            {model.ConfigECDS, lua},
	}
	for _, f := range snap.Listeners[0].FilterChains[0].HCM.HTTPFilters {
		w, ok := want[f.Name]
		if !ok {
			if f.Discovery != nil {
				t.Errorf("%s: discovery %+v on an inline filter", f.Name, f.Discovery)
			}
			continue
		}
		delete(want, f.Name)
		if f.Discovery == nil || f.Discovery.Source != w[0] || f.Type != w[1] || (w[0] == "") != (f.TypedConfig == nil) {
			t.Errorf("%s: discovery %+v, type %q, config %v; want source %q, type %q", f.Name, f.Discovery, f.Type, f.TypedConfig, w[0], w[1])
		}
	}
	if len(want) != 0 {
		t.Errorf("filters not seen: %v", want)
	}
	// A warming listener isn't running its default yet.
	if f := snap.Listeners[1].FilterChains[0].HCM.HTTPFilters[0]; f.Discovery == nil || f.Discovery.Source != "" || !f.Discovery.DefaultConfig {
		t.Errorf("warming listener's ecds.default = %+v, want unresolved with a default", f.Discovery)
	}
}

// TestMatchers: every header and query matcher form is read
// (route_components.proto HeaderMatcher, QueryParameterMatcher; string.proto
// StringMatcher). A header matcher with no specifier is a presence match
// (header_utility.cc, HEADER_MATCH_SPECIFIER_NOT_SET); so is a query matcher
// (config_utility.cc). Unmodelled match fields are listed in Unparsed.
func TestMatchers(t *testing.T) {
	snap := parseFile(t, fixtures+"matchers/envoy/config_dump.json")
	routes := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	f := false
	h := func(name, value string, kind model.MatchKind) model.HeaderMatch {
		return model.HeaderMatch{Name: name, Value: value, Kind: kind, Regex: kind == model.MatchRegex}
	}
	not := h("x-not", "f", model.MatchExact)
	not.Invert = true
	want := model.RouteMatch{Prefix: "/top", CaseSensitive: &f, Headers: []model.HeaderMatch{
		h("x-exact", "a", model.MatchExact), h("x-prefix", "b", model.MatchPrefix), h("x-suffix", "c", model.MatchSuffix),
		h("x-contains", "d", model.MatchContains), h("x-regex", "e+", model.MatchRegex), h("x-present", "", model.MatchPresent),
		h("x-absent", "", model.MatchAbsent), h("x-range", "[1,5)", model.MatchRange), h("x-bare", "", model.MatchPresent), not,
		// A legacy empty exact_match matches any value (header_utility.h, HeaderDataExactMatch).
		h("x-any", "", model.MatchAnyValue),
	}}
	if !reflect.DeepEqual(routes[0].Match, want) {
		t.Errorf("top-level =\n%+v\nwant\n%+v", routes[0].Match, want)
	}

	exact := h("x-exact", "A", model.MatchExact)
	exact.IgnoreCase = true
	empty := h("x-empty", "", model.MatchExact)
	empty.MissingAsEmpty = true
	want = model.RouteMatch{Path: "/string", Headers: []model.HeaderMatch{
		exact, h("x-prefix", "b", model.MatchPrefix), h("x-suffix", "c", model.MatchSuffix), h("x-contains", "d", model.MatchContains),
		h("x-regex", "e+", model.MatchRegex), h("x-custom", "type.googleapis.com/example.CustomMatcher", model.MatchCustom), empty,
	}}
	if !reflect.DeepEqual(routes[1].Match, want) {
		t.Errorf("string_match =\n%+v\nwant\n%+v", routes[1].Match, want)
	}

	q := func(name, value string, kind model.MatchKind, ignore bool) model.QueryParamMatch {
		return model.QueryParamMatch{Name: name, Value: value, Kind: kind, Regex: kind == model.MatchRegex, IgnoreCase: ignore}
	}
	want = model.RouteMatch{Path: "/query", QueryParams: []model.QueryParamMatch{
		q("exact", "1", model.MatchExact, false), q("prefix", "Ab", model.MatchPrefix, true), q("regex", "[0-9]+", model.MatchRegex, false),
		q("present", "", model.MatchPresent, false), q("absent", "", model.MatchAbsent, false), q("bare", "", model.MatchPresent, false),
	}}
	if !reflect.DeepEqual(routes[2].Match, want) {
		t.Errorf("query =\n%+v\nwant\n%+v", routes[2].Match, want)
	}

	if want := (model.RouteMatch{Connect: true}); !reflect.DeepEqual(routes[3].Match, want) {
		t.Errorf("connect = %+v, want %+v", routes[3].Match, want)
	}
	if want := []string{"cookies", "dynamic_metadata", "grpc", "runtime_fraction", "tls_context"}; !reflect.DeepEqual(routes[4].Match.Unparsed, want) {
		t.Errorf("unparsed = %v, want %v", routes[4].Match.Unparsed, want)
	}
	if want := []string{"path_match_policy"}; !reflect.DeepEqual(routes[5].Match.Unparsed, want) {
		t.Errorf("unparsed = %v, want %v", routes[5].Match.Unparsed, want)
	}
}

// TestClusterSelectors: cluster_header and both cluster specifier plugins are
// read into Action, with no cluster name (route_components.proto,
// RouteAction.cluster_specifier).
func TestClusterSelectors(t *testing.T) {
	snap := parseFile(t, fixtures+"cluster-selectors/envoy/config_dump.json")
	routes := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	want := []model.RouteAction{
		{Kind: model.ActionRoute, ClusterHeader: "x-target-cluster"},
		{Kind: model.ActionRoute, ClusterSpecifierPlugin: "picker"},
		{Kind: model.ActionRoute, InlineClusterSpecifierPlugin: &model.Extension{Name: "inline-picker", Type: "type.googleapis.com/example.InlinePicker"}},
		{Kind: model.ActionRoute},
	}
	for i, r := range routes {
		if !reflect.DeepEqual(r.Action, want[i]) || (r.Cluster != "") != (i == 3) {
			t.Errorf("%s: action %+v, cluster %q; want %+v", r.Name, r.Action, r.Cluster, want[i])
		}
	}
}

// TestRouteActions: redirect and direct response keep their fields
// (route_components.proto RedirectAction, DirectResponseAction); a body
// keeps its size and source, never its content.
func TestRouteActions(t *testing.T) {
	snap := parseFile(t, fixtures+"route-actions/envoy/config_dump.json")
	size := func(n int) *int { return &n }
	redirect := func(r model.Redirect) model.RouteAction {
		return model.RouteAction{Kind: model.ActionRedirect, Redirect: &r}
	}
	direct := func(d model.DirectResponse) model.RouteAction {
		return model.RouteAction{Kind: model.ActionDirectResponse, DirectResponse: &d}
	}
	want := []model.RouteAction{
		redirect(model.Redirect{ResponseCode: 308, Scheme: "https", Host: "new.example.test", Port: 8443, Path: "/moved", StripQuery: true}),
		redirect(model.Redirect{ResponseCode: 301, PrefixRewrite: "/v2/"}),
		redirect(model.Redirect{ResponseCode: 302, Scheme: "http", RegexPattern: "^/a/(.*)", RegexSubstitution: `/b/\1`}),
		redirect(model.Redirect{ResponseCode: 301, PathRewrite: "/new/%REQ(x-version)%"}),
		direct(model.DirectResponse{Status: 503, BodySource: model.BodyInline, BodySize: size(24)}),
		direct(model.DirectResponse{Status: 200, BodySource: model.BodyInline, BodySize: size(5)}),
		direct(model.DirectResponse{Status: 404, BodySource: model.BodyFile}),
		direct(model.DirectResponse{Status: 200, BodyFormat: true}),
		direct(model.DirectResponse{Status: 204}),
		{Kind: model.ActionFilter, FilterActionType: "type.googleapis.com/example.Action"},
		{Kind: model.ActionNonForwarding},
	}
	routes := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	if len(routes) != len(want) {
		t.Fatalf("%d routes, want %d", len(routes), len(want))
	}
	for i, r := range routes {
		if !reflect.DeepEqual(r.Action, want[i]) || r.Cluster != "" {
			t.Errorf("%s: action %+v (redirect %+v, direct %+v), want %+v", r.Name, r.Action, r.Action.Redirect, r.Action.DirectResponse, want[i])
		}
	}
}

// TestCoverage: every section is listed, in dump order, as parsed, partial or
// ignored; an HCM with scoped_routes has an unknown route config use.
func TestCoverage(t *testing.T) {
	snap := parseFile(t, fixtures+"coverage/envoy/config_dump.json")
	s := func(typ string, st model.CoverageStatus) model.SectionCoverage {
		return model.SectionCoverage{Type: "type.googleapis.com/" + typ, Status: st}
	}
	want := []model.SectionCoverage{
		s("envoy.admin.v3.BootstrapConfigDump", model.CoverageIgnored),
		s("envoy.admin.v3.ClustersConfigDump", model.CoverageIgnored),
		s("envoy.admin.v3.EndpointsConfigDump", model.CoverageIgnored),
		s("envoy.admin.v3.ListenersConfigDump", model.CoverageParsed),
		s("envoy.admin.v3.ScopedRoutesConfigDump", model.CoverageIgnored),
		s("envoy.admin.v3.RoutesConfigDump", model.CoveragePartial),
		s("envoy.admin.v3.SecretsConfigDump", model.CoverageIgnored),
		s("envoy.admin.v3.EcdsConfigDump", model.CoverageParsed),
		s("example.UnknownConfigDump", model.CoverageIgnored),
	}
	if !reflect.DeepEqual(snap.Coverage.Sections, want) {
		t.Errorf("coverage =\n%+v\nwant\n%+v", snap.Coverage.Sections, want)
	}
	hcm := snap.Listeners[0].FilterChains[0].HCM
	if !hcm.ScopedRoutes || hcm.RouteConfigUse.State != model.RouteConfigUnknown {
		t.Errorf("scoped HCM = %+v, want ScopedRoutes and an unknown use", hcm)
	}

	// A section that fails to decode, and one whose @type can't be read.
	snap = parseString(t, `{"configs": [[], {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": 1}]}`)
	want = []model.SectionCoverage{{Status: model.CoverageIgnored}, s("envoy.admin.v3.ListenersConfigDump", model.CoveragePartial)}
	if !reflect.DeepEqual(snap.Coverage.Sections, want) {
		t.Errorf("coverage = %+v, want %+v", snap.Coverage.Sections, want)
	}
}

// TestDefaultFilterChain: default_filter_chain is parsed as the listener's
// last chain, with ID L<n>/Cd, and its routes get IDs under it.
func TestDefaultFilterChain(t *testing.T) {
	snap := parseFile(t, fixtures+"default-chain/envoy/config_dump.json")
	chains := snap.Listeners[0].FilterChains
	if len(chains) != 2 || chains[0].ID != "L0/C0" || chains[0].Default || chains[1].ID != "L0/Cd" || !chains[1].Default || chains[1].Name != "fallback" {
		t.Fatalf("chains = %+v, want L0/C0 then the default chain L0/Cd", chains)
	}
	hcm := chains[1].HCM
	if hcm == nil || hcm.ID != "L0/Cd/N0" || hcm.RouteConfig == nil || hcm.RouteConfig.VirtualHosts[0].Routes[0].ID != "L0/Cd/N0/RC/V0/R0" {
		t.Errorf("default chain HCM = %+v, want routes under L0/Cd/N0", hcm)
	}
}

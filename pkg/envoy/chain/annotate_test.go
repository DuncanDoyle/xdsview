package chain_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

func parseFile(t *testing.T, path string) *model.EnvoySnapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := parser.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return res.Snapshot
}

func parseString(t *testing.T, s string) *model.EnvoySnapshot {
	t.Helper()
	res, err := parser.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return res.Snapshot
}

// recordings returns a scenario's OSS dumps, keyed by version directory.
func recordings(t *testing.T, scenario string) map[string]string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join("../../../testdata/scenarios", scenario, "envoy/kgateway-*/config_dump.json"))
	if len(paths) != 2 {
		t.Fatalf("%s: got %d recordings, want 2", scenario, len(paths))
	}
	out := map[string]string{}
	for _, p := range paths {
		out[filepath.Base(filepath.Dir(p))] = p
	}
	return out
}

// routes returns every route in snap with its chain.
func routes(snap *model.EnvoySnapshot) []model.Route {
	var out []model.Route
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				out = append(out, vh.Routes...)
			}
		}
	}
	return out
}

const (
	setMetadataType = "type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config"
	corsType        = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"
	corsPolicyType  = "type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy"
	rateLimitType   = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"
	rlPerRouteType  = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"
)

func trafficPolicy(name, field string) model.Contribution {
	return model.Contribution{Kind: model.ContributionPolicy, Field: field, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "kgateway-system", Name: name}}
}

func gatewayExtension(name string) model.Contribution {
	return model.Contribution{Kind: model.ContributionExtension, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "kgateway-system", Name: name}}
}

// want03 is the chain of 03-multi-policy's one route, on v2.4.7 and v2.5.0 alike:
// kgateway disables the policy filters in the HCM and enables them per route.
func want03(attributed bool) model.EffectiveChain {
	yes, no := model.SideYes, model.SideNo
	c := model.EffectiveChain{Filters: []model.ChainEntry{
		{Name: "global_disable/ext_auth", Type: setMetadataType, Runs: false, State: model.FilterOff, Request: yes, Response: no},
		{Name: "envoy.filters.http.cors", Type: corsType, Runs: true, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: corsPolicyType, Request: yes, Response: yes},
		{Name: "ext_auth/kgateway-system/ext-authz", Type: extAuthzType, Runs: true, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: filterConfigType, Request: yes, Response: yes},
		{Name: "ratelimit/kgateway-system/ratelimit", Type: rateLimitType, Runs: true, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: rlPerRouteType, Request: yes, Response: yes},
		{Name: "envoy.filters.http.router", Type: routerType, Runs: true, State: model.FilterOn, Request: yes, Response: no},
	}}
	if attributed {
		c.Filters[1].Contributions = []model.Contribution{trafficPolicy("cors", "cors")}
		c.Filters[2].Contributions = []model.Contribution{gatewayExtension("ext-authz"), trafficPolicy("ext-auth", "extAuth")}
		c.Filters[3].Contributions = []model.Contribution{gatewayExtension("ratelimit"), trafficPolicy("rate-limit", "rateLimit.global")}
	}
	return c
}

func names(es []model.ChainEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

// TestAnnotate_Corpus03 checks the chain on the recorded 03-multi-policy dumps,
// without a mapper and with the kgateway mapper.
func TestAnnotate_Corpus03(t *testing.T) {
	for version, path := range recordings(t, "03-multi-policy") {
		snap := parseFile(t, path)
		for _, tc := range []struct {
			name string
			m    origin.Mapper
		}{{"no mapper", nil}, {"kgateway", origin.Kgateway(origin.KgatewayOptions{})}} {
			rs := routes(chain.Annotate(snap, tc.m))
			if len(rs) != 1 || rs[0].Chain == nil {
				t.Fatalf("%s %s: got %d routes, want 1 with a chain", version, tc.name, len(rs))
			}
			c := *rs[0].Chain
			if want := want03(tc.m != nil); !reflect.DeepEqual(c, want) {
				t.Errorf("%s %s: chain =\n%+v\nwant\n%+v", version, tc.name, c, want)
			}
			if got, want := names(c.RequestOrder()), []string{"envoy.filters.http.cors", "ext_auth/kgateway-system/ext-authz", "ratelimit/kgateway-system/ratelimit", "envoy.filters.http.router"}; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: request order = %v, want %v", version, got, want)
			}
			if got, want := names(c.ResponseOrder()), []string{"ratelimit/kgateway-system/ratelimit", "ext_auth/kgateway-system/ext-authz", "envoy.filters.http.cors"}; !reflect.DeepEqual(got, want) {
				t.Errorf("%s: response order = %v, want %v", version, got, want)
			}
		}
	}
}

// TestAnnotate_Corpus00 checks a route with only the router.
func TestAnnotate_Corpus00(t *testing.T) {
	for version, path := range recordings(t, "00-simple") {
		rs := routes(chain.Annotate(parseFile(t, path), origin.Kgateway(origin.KgatewayOptions{})))
		want := model.EffectiveChain{Filters: []model.ChainEntry{
			{Name: "envoy.filters.http.router", Type: routerType, Runs: true, State: model.FilterOn, Request: model.SideYes, Response: model.SideNo},
		}}
		if len(rs) != 1 || rs[0].Chain == nil || !reflect.DeepEqual(*rs[0].Chain, want) {
			t.Errorf("%s: routes = %+v, want one route with chain %+v", version, rs, want)
		}
	}
}

// TestAnnotate_CopiesInput checks that the input snapshot keeps nil chains and
// that every route in every HCM gets one, including two HCMs sharing a route config.
func TestAnnotate_CopiesInput(t *testing.T) {
	snap := parseString(t, pluginDump)
	out := chain.Annotate(snap, nil)
	for _, r := range routes(snap) {
		if r.Chain != nil {
			t.Errorf("input route %s got a chain", r.Name)
		}
	}
	rs := routes(out)
	if len(rs) != 2 {
		t.Fatalf("got %d routes, want 2 (one per HCM)", len(rs))
	}
	for _, r := range rs {
		if r.Chain == nil || len(r.Chain.Filters) != 3 {
			t.Errorf("route %s chain = %+v, want 3 entries", r.Name, r.Chain)
		}
	}
}

// pluginDump has a vendor filter enabled per route by a vendor policy field,
// and an OSS ext auth filter enabled by a TrafficPolicy. Two listeners share the route config.
const pluginDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [` + pluginHCM + `]}]}}},
  {"name": "listener~81", "active_state": {"listener": {"name": "listener~81", "filter_chains": [{"name": "listener~81", "filters": [` + pluginHCM + `]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "rc",
  "virtual_hosts": [{"name": "vh", "domains": ["*"], "routes": [{"name": "r", "match": {"prefix": "/"}, "route": {"cluster": "c"},
   "typed_per_filter_config": {
    "acme_waf/ns": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}},
    "ext_auth/ns/authz": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}},
   "metadata": {"filter_metadata": {
    "merge.AcmePolicy.acme.io": {"acmeWaf": ["acme.io/AcmePolicy/ns/waf"]},
    "merge.TrafficPolicy.gateway.kgateway.dev": {"extAuth": ["gateway.kgateway.dev/TrafficPolicy/ns/auth"]}}}}]}]}}]}]}`

const pluginHCM = `{"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "rc"}, "http_filters": [
 {"name": "acme_waf/ns", "disabled": true, "typed_config": {"@type": "type.googleapis.com/acme.waf.v1.Waf"}},
 {"name": "ext_auth/ns/authz", "disabled": true, "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"}},
 {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}`

// acmeMapper is a vendor plugin. It maps nothing but its own policy field.
type acmeMapper struct{ origin.Mapper }

func (acmeMapper) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "acmeWaf" && f.Type == "type.googleapis.com/acme.waf.v1.Waf"
}

// fieldless implements Mapper but not PolicyFilters.
type fieldless struct{ origin.Mapper }

func contributions(t *testing.T, snap *model.EnvoySnapshot, m origin.Mapper) map[string][]model.Contribution {
	t.Helper()
	out := map[string][]model.Contribution{}
	for _, e := range routes(chain.Annotate(snap, m))[0].Chain.Filters {
		out[e.Name] = e.Contributions
	}
	return out
}

// TestAnnotate_Contributions covers which mappers attribute which entries.
func TestAnnotate_Contributions(t *testing.T) {
	snap := parseString(t, pluginDump)
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	auth := model.Contribution{Kind: model.ContributionPolicy, Field: "extAuth", Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "auth"}}
	ext := model.Contribution{Kind: model.ContributionExtension, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "ns", Name: "authz"}}
	waf := model.Contribution{Kind: model.ContributionPolicy, Field: "acmeWaf", Certainty: model.Inferred,
		Ref: model.Ref{Group: "acme.io", Kind: "AcmePolicy", Namespace: "ns", Name: "waf"}}

	tests := []struct {
		name string
		m    origin.Mapper
		want map[string][]model.Contribution
	}{
		{"kgateway: unmapped vendor field attributes nothing", kgw,
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
		{"plugin first in a chain adds its field", origin.Chain(acmeMapper{kgw}, kgw),
			map[string][]model.Contribution{"acme_waf/ns": {waf}, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
		{"mapper without PolicyFilters: extensions only", fieldless{kgw},
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": {ext}, "envoy.filters.http.router": nil}},
		{"nil mapper", nil,
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": nil, "envoy.filters.http.router": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contributions(t, snap, tt.m); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("contributions = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// TestAnnotate_PolicyNeedsPerRouteLevel checks that a policy isn't attributed to
// an entry that runs on the HCM default, even when the field matches its kind.
func TestAnnotate_PolicyNeedsPerRouteLevel(t *testing.T) {
	dump := strings.Replace(pluginDump, `"ext_auth/ns/authz": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}`, `"unused": {}`, 1)
	dump = strings.ReplaceAll(dump, `{"name": "ext_auth/ns/authz", "disabled": true,`, `{"name": "ext_auth/ns/authz",`)
	got := contributions(t, parseString(t, dump), origin.Kgateway(origin.KgatewayOptions{}))["ext_auth/ns/authz"]
	want := []model.Contribution{{Kind: model.ContributionExtension, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "ns", Name: "authz"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("contributions = %+v, want the extension only %+v", got, want)
	}
}

package chain_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

func parseFile(t *testing.T, path string) *model.Snapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := parser.Parse(f, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func parseString(t *testing.T, s string) *model.Snapshot {
	t.Helper()
	res, err := parser.Parse(strings.NewReader(s), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return res
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

// routes returns every route in snap.
func routes(snap *model.Snapshot) []model.Route {
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

// want03 is the chain of 03-multi-policy's one route, on v2.4.7 and v2.5.0 alike:
// kgateway disables the policy filters in the HCM and enables them per route.
func want03(hcm string) model.EffectiveChain {
	yes, no, inline := model.TriYes, model.TriNo, model.ConfigInline
	slot := func(i string) model.ID { return model.ID(hcm + "/F" + i) }
	return model.EffectiveChain{Filters: []model.ChainEntry{
		{Slot: slot("0"), Name: "global_disable/ext_auth", Type: setMetadataType, Runs: no, State: model.FilterOff, Request: yes, Response: no, ConfigSource: inline},
		{Slot: slot("1"), Name: "envoy.filters.http.cors", Type: corsType, Runs: yes, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: corsPolicyType, Request: yes, Response: yes, ConfigSource: inline},
		{Slot: slot("2"), Name: "ext_auth/kgateway-system/ext-authz", Type: extAuthzType, Runs: yes, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: filterConfigType, Request: yes, Response: yes, ConfigSource: inline},
		{Slot: slot("3"), Name: "ratelimit/kgateway-system/ratelimit", Type: rateLimitType, Runs: yes, State: model.FilterEnabled, Level: model.LevelRoute, PerFilterType: rlPerRouteType, Request: yes, Response: yes, ConfigSource: inline},
		{Slot: slot("4"), Name: "envoy.filters.http.router", Type: routerType, Runs: yes, State: model.FilterOn, Request: yes, Response: no, ConfigSource: inline},
	}}
}

func names(es []model.ChainEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

// TestEffective_Corpus03 checks the chain on the recorded 03-multi-policy dumps.
func TestEffective_Corpus03(t *testing.T) {
	for version, path := range recordings(t, "03-multi-policy") {
		snap := parseFile(t, path)
		rs := routes(snap)
		if len(rs) != 1 {
			t.Fatalf("%s: got %d routes, want 1", version, len(rs))
		}
		c := chain.Effective(snap)[rs[0].ID]
		if want := want03(string(snap.Listeners[0].FilterChains[0].HCM.ID)); !reflect.DeepEqual(c, want) {
			t.Errorf("%s: chain =\n%+v\nwant\n%+v", version, c, want)
		}
		if got, want := names(c.RequestOrder()), []string{"envoy.filters.http.cors", "ext_auth/kgateway-system/ext-authz", "ratelimit/kgateway-system/ratelimit", "envoy.filters.http.router"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: request order = %v, want %v", version, got, want)
		}
		if got, want := names(c.ResponseOrder()), []string{"ratelimit/kgateway-system/ratelimit", "ext_auth/kgateway-system/ext-authz", "envoy.filters.http.cors"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: response order = %v, want %v", version, got, want)
		}
	}
}

// TestEffective_Corpus00 checks a route with only the router.
func TestEffective_Corpus00(t *testing.T) {
	for version, path := range recordings(t, "00-simple") {
		snap := parseFile(t, path)
		rs := routes(snap)
		if len(rs) != 1 {
			t.Fatalf("%s: got %d routes, want 1", version, len(rs))
		}
		slot := snap.Listeners[0].FilterChains[0].HCM.HTTPFilters[0].ID
		want := model.EffectiveChain{Filters: []model.ChainEntry{
			{Slot: slot, Name: "envoy.filters.http.router", Type: routerType, Runs: model.TriYes, State: model.FilterOn,
				Request: model.TriYes, Response: model.TriNo, ConfigSource: model.ConfigInline},
		}}
		if got := chain.Effective(snap)[rs[0].ID]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: chain = %+v, want %+v", version, got, want)
		}
	}
}

// TestEffective_SharedRouteConfig: two HCMs on one route config get one chain
// per route use, each with its own HCM's slot IDs.
func TestEffective_SharedRouteConfig(t *testing.T) {
	snap := parseString(t, sharedDump)
	chains := chain.Effective(snap)
	rs := routes(snap)
	if len(rs) != 2 || len(chains) != 2 || rs[0].ID == rs[1].ID {
		t.Fatalf("routes %d, chains %d; want two routes with distinct IDs, each with a chain", len(rs), len(chains))
	}
	for i, r := range rs {
		c := chains[r.ID]
		if len(c.Filters) != 3 || !c.Filters[0].Slot.Within(snap.Listeners[i].ID) {
			t.Errorf("route %s: chain %+v, want 3 entries with slots in listener %s", r.ID, c, snap.Listeners[i].ID)
		}
	}
}

// sharedDump has two listeners whose HCMs share one RDS route config.
const sharedDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [` + sharedHCM + `]}]}}},
  {"name": "listener~81", "active_state": {"listener": {"name": "listener~81", "filter_chains": [{"name": "listener~81", "filters": [` + sharedHCM + `]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "rc",
  "virtual_hosts": [{"name": "vh", "domains": ["*"], "routes": [{"name": "r", "match": {"prefix": "/"}, "route": {"cluster": "c"}}]}]}}]}]}`

const sharedHCM = `{"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "rc"}, "http_filters": [
 {"name": "acme_waf/ns", "disabled": true, "typed_config": {"@type": "type.googleapis.com/acme.waf.v1.Waf"}},
 {"name": "ext_auth/ns/authz", "disabled": true, "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"}},
 {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}`

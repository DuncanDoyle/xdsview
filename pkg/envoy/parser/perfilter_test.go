package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// perFilterDump has two listeners sharing route config "rc", which carries
// typed_per_filter_config at route config, virtual host and weighted cluster level.
const perFilterDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "a", "active_state": {"listener": {"name": "a", "filter_chains": [{"name": "a", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "rc"}}}]}]}}},
  {"name": "b", "active_state": {"listener": {"name": "b", "filter_chains": [{"name": "b", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "rc"}}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {
  "name": "rc",
  "typed_per_filter_config": {"cors": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "disabled": true}},
  "virtual_hosts": [{"name": "vh", "domains": ["*"],
   "typed_per_filter_config": {"ext_auth": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}},
   "routes": [{"name": "r", "match": {"prefix": "/"}, "route": {"weighted_clusters": {"clusters": [
    {"name": "v1", "weight": 80,
     "typed_per_filter_config": {"ext_auth": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "disabled": true}}},
    {"name": "v2", "weight": 20}]}}}]}]}}]}]}`

func parsePerFilter(t *testing.T) *model.EnvoySnapshot {
	t.Helper()
	result, err := parser.Parse(strings.NewReader(perFilterDump))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings: %v", result.Warnings)
	}
	return result.Snapshot
}

func filterConfig(fields map[string]any) map[string]any {
	fields["@type"] = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
	return fields
}

// TestParse_PerFilterConfigLevels checks the three levels on each HCM's joined route config.
func TestParse_PerFilterConfigLevels(t *testing.T) {
	snap := parsePerFilter(t)
	for _, l := range snap.Listeners {
		rc := l.FilterChains[0].HCM.RouteConfig
		if rc == nil {
			t.Fatalf("%s: no joined route config", l.Name)
		}
		if want := map[string]any{"cors": filterConfig(map[string]any{"disabled": true})}; !reflect.DeepEqual(rc.TypedPerFilterConfig, want) {
			t.Errorf("%s: route config TypedPerFilterConfig = %v, want %v", l.Name, rc.TypedPerFilterConfig, want)
		}
		vh := rc.VirtualHosts[0]
		if want := map[string]any{"ext_auth": filterConfig(map[string]any{"config": map[string]any{}})}; !reflect.DeepEqual(vh.TypedPerFilterConfig, want) {
			t.Errorf("%s: vhost TypedPerFilterConfig = %v, want %v", l.Name, vh.TypedPerFilterConfig, want)
		}
		wcs := vh.Routes[0].WeightedClusters
		if want := map[string]any{"ext_auth": filterConfig(map[string]any{"disabled": true})}; !reflect.DeepEqual(wcs[0].TypedPerFilterConfig, want) {
			t.Errorf("%s: backend v1 TypedPerFilterConfig = %v, want %v", l.Name, wcs[0].TypedPerFilterConfig, want)
		}
		if wcs[1].TypedPerFilterConfig != nil {
			t.Errorf("%s: backend v2 TypedPerFilterConfig = %v, want nil", l.Name, wcs[1].TypedPerFilterConfig)
		}
	}
}

// TestParse_PerFilterConfigIsolated checks that each HCM owns its copy of the
// new maps: changing listener a's leaves listener b's as parsed.
func TestParse_PerFilterConfigIsolated(t *testing.T) {
	snap := parsePerFilter(t)
	a := snap.Listeners[0].FilterChains[0].HCM.RouteConfig
	a.TypedPerFilterConfig["x"] = 1
	a.VirtualHosts[0].TypedPerFilterConfig["x"] = 1
	a.VirtualHosts[0].Routes[0].WeightedClusters[0].TypedPerFilterConfig["x"] = 1

	b := snap.Listeners[1].FilterChains[0].HCM.RouteConfig
	for level, m := range map[string]map[string]any{
		"route config":     b.TypedPerFilterConfig,
		"vhost":            b.VirtualHosts[0].TypedPerFilterConfig,
		"weighted cluster": b.VirtualHosts[0].Routes[0].WeightedClusters[0].TypedPerFilterConfig,
	} {
		if _, ok := m["x"]; ok {
			t.Errorf("%s: change to listener a's copy shows in listener b's", level)
		}
	}
}

package parser_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Metadata at listener, route config and vhost level; one source shared by two
// HCMs and one source with no use.
var metadataDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "a", "active_state": {"listener": {"name": "a", "metadata": {"filter_metadata": {"m": {"level": "listener"}}}, "filter_chains": [{"filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "shared"}, "http_filters": []}}]}]}}},
  ` + hcmListener("b", `"rds": {"route_config_name": "shared"}`) + `]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "shared", "metadata": {"filter_metadata": {"m": {"level": "rc"}}}, "typed_per_filter_config": {"f": {"x": 1}},
   "virtual_hosts": [{"name": "vh", "domains": ["*"], "metadata": {"filter_metadata": {"m": {"level": "vhost"}}}, "routes": [
    {"name": "r0", "match": {"prefix": "/"}, "route": {"cluster": "z"}}]}]}},
  {"route_config": {"name": "unused", "metadata": {"filter_metadata": {"m": {"level": "unused"}}}, "virtual_hosts": []}}]}]}`

// TestParse_LevelMetadata: listeners, sources and vhosts keep their
// filter_metadata, sources their per-filter config, even with no use
// (review 9.1-9.2 #3); vhosts and routes carry a SourcePath equal across uses.
func TestParse_LevelMetadata(t *testing.T) {
	snap := parseString(t, metadataDump)
	level := func(md map[string]any) any { m, _ := md["m"].(map[string]any); return m["level"] }
	if got := level(snap.Listeners[0].Metadata); got != "listener" {
		t.Errorf("listener metadata level = %v", got)
	}
	src := map[string]model.RouteConfigSource{}
	for _, s := range snap.RouteConfigs {
		src[s.Name] = s
	}
	if got := level(src["shared"].Metadata); got != "rc" {
		t.Errorf("shared source metadata level = %v", got)
	}
	if !reflect.DeepEqual(src["shared"].TypedPerFilterConfig, map[string]any{"f": map[string]any{"x": float64(1)}}) {
		t.Errorf("shared source per-filter config = %v", src["shared"].TypedPerFilterConfig)
	}
	if got := level(src["unused"].Metadata); got != "unused" {
		t.Errorf("unused source metadata level = %v", got)
	}
	var paths []model.ID
	for _, l := range snap.Listeners {
		vh := l.FilterChains[0].HCM.RouteConfig.VirtualHosts[0]
		if got := level(vh.Metadata); got != "vhost" {
			t.Errorf("%s metadata level = %v", vh.ID, got)
		}
		paths = append(paths, vh.SourcePath, vh.Routes[0].SourcePath)
	}
	id := src["shared"].ID
	want := []model.ID{id + "/V0", id + "/V0/R0", id + "/V0", id + "/V0/R0"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("source paths = %v, want %v", paths, want)
	}
	// Each use owns its copy of vhost metadata.
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Metadata["m"].(map[string]any)["level"] = "changed"
	if got := level(snap.Listeners[1].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Metadata); got != "vhost" {
		t.Errorf("uses share vhost metadata: %v", got)
	}
}

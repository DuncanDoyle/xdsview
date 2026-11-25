package parser_test

import (
	"encoding/json"
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
	want := []model.RouteConfigSource{
		{ID: "L0/C0/N0/RCI", Kind: model.SourceInline, Name: "local_route", Reported: "RS0"},
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

// moveState moves the dynamic listeners' active_state to field.
func moveState(s map[string]any, field string) {
	ls, _ := s["dynamic_listeners"].([]any)
	for _, l := range ls {
		m := l.(map[string]any)
		m[field] = m["active_state"]
		delete(m, "active_state")
	}
}

// TestDefaultFilterChain:default_filter_chain is parsed as the listener's
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

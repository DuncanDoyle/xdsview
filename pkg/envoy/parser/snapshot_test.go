package parser_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// occurrence is one ID-carrying element of a snapshot, with its kind.
type occurrence struct {
	kind string
	id   model.ID
}

// occurrences lists every occurrence in snap that must carry an ID.
func occurrences(snap *model.Snapshot) []occurrence {
	var out []occurrence
	add := func(kind string, id model.ID) { out = append(out, occurrence{kind, id}) }
	for _, s := range snap.RouteConfigs {
		add("source", s.ID)
	}
	for _, l := range snap.Listeners {
		add("listener", l.ID)
		for _, fc := range l.FilterChains {
			add("chain", fc.ID)
			hcm := fc.HCM
			// The HCM is its network filter's occurrence, so it shares that ID.
			for _, nf := range fc.NetworkFilters {
				if hcm != nil && nf.ID == hcm.ID {
					add("hcm", nf.ID)
				} else {
					add("network", nf.ID)
				}
			}
			if hcm == nil {
				continue
			}
			add("use", hcm.RouteConfigUse.ID)
			for _, f := range hcm.HTTPFilters {
				add("filter", f.ID)
			}
			for _, f := range hcm.UpstreamFilters {
				add("upstream", f.ID)
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for _, vh := range hcm.RouteConfig.VirtualHosts {
				add("vhost", vh.ID)
				for _, r := range vh.Routes {
					add("route", r.ID)
					for _, w := range r.WeightedClusters {
						add("backend", w.ID)
					}
				}
			}
		}
	}
	return out
}

func corpusDumps(t *testing.T) []string {
	t.Helper()
	var dumps []string
	err := filepath.WalkDir("../../../testdata", func(path string, d os.DirEntry, err error) error {
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
	if err != nil || len(dumps) == 0 {
		t.Fatalf("no corpus dumps: %v", err)
	}
	return dumps
}

func parseFile(t *testing.T, path string) *model.Snapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	snap, err := parser.Parse(f, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func parseString(t *testing.T, dump string) *model.Snapshot {
	t.Helper()
	snap, err := parser.Parse(strings.NewReader(dump), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// TestIDs_UniqueInEveryDump: every occurrence in every corpus dump has a
// non-empty ID that no other occurrence shares, and the HCM shapes appear.
func TestIDs_UniqueInEveryDump(t *testing.T) {
	kinds := map[string]bool{}
	for _, dump := range corpusDumps(t) {
		snap := parseFile(t, dump)
		seen := map[model.ID]string{}
		for _, o := range occurrences(snap) {
			kinds[o.kind] = true
			if o.id == "" {
				t.Errorf("%s: %s without an ID", dump, o.kind)
				continue
			}
			if prev, dup := seen[o.id]; dup {
				t.Errorf("%s: ID %s on a %s and a %s", dump, o.id, prev, o.kind)
			}
			seen[o.id] = o.kind
		}
	}
	for _, k := range []string{"source", "listener", "chain", "network", "hcm", "use", "filter", "upstream", "vhost", "route", "backend"} {
		if !kinds[k] {
			t.Errorf("no %s in the corpus; the check doesn't cover it", k)
		}
	}
}

// TestIDs_Within: a route is within its vhost, use, HCM, chain and listener,
// and not within a sibling.
func TestIDs_Within(t *testing.T) {
	snap := parseString(t, sharedDump)
	l := snap.Listeners[0]
	fc := l.FilterChains[0]
	vh := fc.HCM.RouteConfig.VirtualHosts[0]
	r := vh.Routes[0]
	for _, block := range []model.ID{r.ID, vh.ID, fc.HCM.RouteConfigUse.ID, fc.HCM.ID, fc.ID, l.ID} {
		if !r.ID.Within(block) {
			t.Errorf("%s not within %s", r.ID, block)
		}
	}
	for _, other := range []model.ID{vh.Routes[1].ID, snap.Listeners[1].ID, fc.HCM.HTTPFilters[0].ID, ""} {
		if r.ID.Within(other) {
			t.Errorf("%s within sibling %q", r.ID, other)
		}
	}
	if model.ID("L10/C0").Within("L1") {
		t.Error("L10/C0 within L1")
	}
}

// Two listeners on one RDS name, and an RDS name with two dynamic sources.
var sharedDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  ` + hcmListener("a", `"rds": {"route_config_name": "shared"}`) + `,
  ` + hcmListener("b", `"rds": {"route_config_name": "shared"}`) + `,
  ` + hcmListener("c", `"rds": {"route_config_name": "dup"}`) + `]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "shared", "virtual_hosts": [{"name": "vh", "domains": ["*"], "routes": [
   {"name": "r0", "match": {"prefix": "/"}, "route": {"weighted_clusters": {"clusters": [{"name": "x", "weight": 1}, {"name": "y", "weight": 1}]}}},
   {"name": "r1", "match": {"prefix": "/b"}, "route": {"cluster": "z"}}]}]}},
  {"route_config": {"name": "dup", "virtual_hosts": [{"name": "first", "domains": ["*"], "routes": []}]}},
  {"route_config": {"name": "dup", "virtual_hosts": [{"name": "second", "domains": ["*"], "routes": []}]}}]}]}`

// hcmListener is an active dynamic listener with one HCM; rc is its route
// config field (rds or route_config).
func hcmListener(name, rc string) string {
	return `{"name": "` + name + `", "active_state": {"listener": {"name": "` + name + `", "filter_chains": [{"filters": [
 {"name": "envoy.filters.network.http_connection_manager", "typed_config": {` + rc + `, "http_filters": [
  {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}`
}

// TestRouteConfigs_SharedSource: two HCMs on one RDS name join one source
// through two uses, each with its own vhost and route IDs.
func TestRouteConfigs_SharedSource(t *testing.T) {
	snap := parseString(t, sharedDump)
	var shared []model.RouteConfigSource
	for _, s := range snap.RouteConfigs {
		if s.Name == "shared" {
			shared = append(shared, s)
		}
	}
	if len(shared) != 1 || shared[0].Kind != model.SourceDynamic {
		t.Fatalf("shared sources = %+v, want one dynamic", shared)
	}
	a, b := snap.Listeners[0].FilterChains[0].HCM, snap.Listeners[1].FilterChains[0].HCM
	for _, h := range []*model.HCMConfig{a, b} {
		if u := h.RouteConfigUse; u.Source != shared[0].ID || u.State != model.RouteConfigFound || h.RouteConfig == nil {
			t.Errorf("use %+v, want found on %s", u, shared[0].ID)
		}
	}
	if a.RouteConfigUse.ID == b.RouteConfigUse.ID {
		t.Errorf("both uses have ID %s", a.RouteConfigUse.ID)
	}
	va, vb := a.RouteConfig.VirtualHosts[0], b.RouteConfig.VirtualHosts[0]
	if va.ID == vb.ID || va.Routes[0].ID == vb.Routes[0].ID || va.Routes[0].WeightedClusters[0].ID == vb.Routes[0].WeightedClusters[0].ID {
		t.Errorf("uses share vhost, route or backend IDs: %s %s", va.Routes[0].ID, vb.Routes[0].ID)
	}
}

// TestRouteConfigs_DuplicateName: two dynamic sources with one name are both
// listed, and the HCM naming it is ambiguous, not joined, with a warning.
func TestRouteConfigs_DuplicateName(t *testing.T) {
	snap := parseString(t, sharedDump)
	var dups []model.ID
	for _, s := range snap.RouteConfigs {
		if s.Name == "dup" {
			dups = append(dups, s.ID)
		}
	}
	if len(dups) != 2 || dups[0] == dups[1] {
		t.Errorf("dup sources = %v, want two", dups)
	}
	hcm := snap.Listeners[2].FilterChains[0].HCM
	use := hcm.RouteConfigUse
	if use.State != model.RouteConfigAmbiguous || use.Source != "" || hcm.RouteConfig != nil {
		t.Errorf("use = %+v, routeConfig = %v; want ambiguous and not joined", use, hcm.RouteConfig)
	}
	want := model.Warning{Subject: use.ID, Code: model.WarnRDSAmbiguous,
		Message: `listener "c" filter chain #0: route config "dup" is ambiguous: 2 dynamic route configs have that name`}
	if !reflect.DeepEqual(snap.Warnings, []model.Warning{want}) {
		t.Errorf("warnings = %+v, want %+v", snap.Warnings, want)
	}
}

// A static entry that is a dynamic listener's inline config, an unused one,
// and an HCM whose RDS name matches only the unused static entry.
var sourcesDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  ` + hcmListener("inline", `"route_config": {"name": "local", "virtual_hosts": [{"name": "v", "domains": ["*"], "routes": [{"name": "r", "match": {"path": "/ready"}, "route": {"cluster": "admin"}}]}]}`) + `,
  ` + hcmListener("rds", `"rds": {"route_config_name": "orphan"}`) + `]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "static_route_configs": [
  {"route_config": {"@type": "type.googleapis.com/envoy.config.route.v3.RouteConfiguration", "name": "local", "virtual_hosts": [{"name": "v", "domains": ["*"], "routes": [{"name": "r", "match": {"path": "/ready"}, "route": {"cluster": "admin"}}]}]}},
  {"route_config": {"name": "orphan", "virtual_hosts": []}}]}]}`

// TestRouteConfigs_SourcesAndUses: an inline config records the static entry
// Envoy reports for it, which stays a source; another static entry is a source
// with no use; an RDS name never resolves to a static source.
func TestRouteConfigs_SourcesAndUses(t *testing.T) {
	snap := parseString(t, sourcesDump)
	inline := snap.Listeners[0].FilterChains[0].HCM
	want := []model.RouteConfigSource{
		{ID: inline.ID + "/RCI", Kind: model.SourceInline, Name: "local", Reported: "RS0"},
		{ID: "RS0", Kind: model.SourceStatic, Name: "local"},
		{ID: "RS1", Kind: model.SourceStatic, Name: "orphan"},
	}
	if !reflect.DeepEqual(snap.RouteConfigs, want) {
		t.Errorf("sources = %+v\nwant %+v", snap.RouteConfigs, want)
	}
	if u := inline.RouteConfigUse; u.Source != inline.ID+"/RCI" || u.State != model.RouteConfigFound ||
		inline.RouteConfig == nil || inline.RouteConfig.VirtualHosts[0].Routes[0].ID == "" {
		t.Errorf("inline use = %+v, want found on its inline source with route IDs", u)
	}
	rds := snap.Listeners[1].FilterChains[0].HCM
	if u := rds.RouteConfigUse; u.Source != "" || u.State != model.RouteConfigUnknown || rds.RouteConfig != nil {
		t.Errorf("rds use = %+v, want unresolved (static entries never answer RDS)", u)
	}
}

// TestRouteConfigs_Scenario00: main_route and prometheus_route are inline
// configs of static listeners, also listed in static_route_configs. Each is
// one inline source that records the static ID as Reported, and that static
// source is in the snapshot; no HCM joins a static source.
func TestRouteConfigs_Scenario00(t *testing.T) {
	snap := parseFile(t, "../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
	inline := map[string]model.RouteConfigSource{}
	static := map[model.ID]string{}
	for _, s := range snap.RouteConfigs {
		switch s.Kind {
		case model.SourceInline:
			inline[s.Name] = s
		case model.SourceStatic:
			static[s.ID] = s.Name
		}
	}
	for _, name := range []string{"main_route", "prometheus_route"} {
		if s := inline[name]; s.ID == "" || static[s.Reported] != name {
			t.Errorf("%s: inline source = %+v, want one whose Reported static source has its name", name, s)
		}
	}
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM != nil && strings.HasPrefix(string(fc.HCM.RouteConfigUse.Source), "RS") {
				t.Errorf("%s joined static source %s", fc.HCM.RouteConfigUse.ID, fc.HCM.RouteConfigUse.Source)
			}
		}
	}
}

// TestRouteConfigs_MalformedStaticField: a bad static_route_configs field
// costs only the static entries; dynamic configs still join their HCMs.
func TestRouteConfigs_MalformedStaticField(t *testing.T) {
	snap := parseString(t, `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [`+hcmListener("a", `"rds": {"route_config_name": "rc"}`)+`]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "static_route_configs": 1, "dynamic_route_configs": [
  {"route_config": {"name": "rc", "virtual_hosts": [{"name": "vh", "domains": ["*"], "routes": []}]}}]}]}`)

	use := snap.Listeners[0].FilterChains[0].HCM.RouteConfigUse
	if use.State != model.RouteConfigFound || use.Source != "RD0" {
		t.Errorf("use = %+v, want found on RD0", use)
	}
	want := "skipped RoutesConfigDump static_route_configs: json: "
	if len(snap.Warnings) != 1 || snap.Warnings[0].Code != model.WarnSectionSkipped || !strings.HasPrefix(snap.Warnings[0].Message, want) {
		t.Errorf("warnings = %+v, want one %s %q…", snap.Warnings, model.WarnSectionSkipped, want)
	}
}

// TestWarnings_Typed: each warning keeps its text and gains a code and, where
// it has one, a subject.
func TestWarnings_Typed(t *testing.T) {
	cases := []struct {
		name, dump string
		want       model.Warning
	}{
		{"unreadable section", `{"configs": [[]]}`,
			model.Warning{Code: model.WarnSectionSkipped, Message: "skipped config section: cannot read @type: json: "}},
		{"listeners section", `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": 1}]}`,
			model.Warning{Code: model.WarnSectionSkipped, Message: "skipped ListenersConfigDump: json: "}},
		{"routes section", `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": 1}]}`,
			model.Warning{Code: model.WarnSectionSkipped, Message: "skipped RoutesConfigDump dynamic_route_configs: json: "}},
		{"route config", `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "bad", "virtual_hosts": 1}}]}]}`,
			model.Warning{Code: model.WarnRouteConfigSkipped, Message: `skipped route config "bad": json: `}},
		{"nameless route config", `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {}}]}]}`,
			model.Warning{Code: model.WarnRouteConfigSkipped, Message: "skipped a route config with no name"}},
		{"HCM", `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [` +
			hcmListener("x", `"http_filters": 1`) + `]}]}`,
			model.Warning{Subject: "L0/C0", Code: model.WarnHCMSkipped, Message: `listener "x" filter chain #0: skipped HTTP connection manager: json: `}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap := parseString(t, c.dump)
			// The want message is a prefix: the decoder's error text varies by Go version.
			if len(snap.Warnings) != 1 || snap.Warnings[0].Subject != c.want.Subject || snap.Warnings[0].Code != c.want.Code ||
				!strings.HasPrefix(snap.Warnings[0].Message, c.want.Message) {
				t.Errorf("warnings = %+v\nwant [%+v]", snap.Warnings, c.want)
			}
		})
	}
}

// TestParse_Deterministic: two parses of one dump are deep-equal, IDs included.
func TestParse_Deterministic(t *testing.T) {
	for _, dump := range corpusDumps(t) {
		if a, b := parseFile(t, dump), parseFile(t, dump); !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two parses differ", dump)
		}
	}
}

// TestParse_Capture: the capture defaults to unverified and is carried as given.
func TestParse_Capture(t *testing.T) {
	if got := parseString(t, `{"configs": []}`).Capture; got != model.CaptureUnverified {
		t.Errorf("default capture = %q, want unverified", got)
	}
	snap, err := parser.Parse(strings.NewReader(`{"configs": []}`), parser.Options{Capture: model.CaptureComplete})
	if err != nil || snap.Capture != model.CaptureComplete {
		t.Errorf("capture = %q, err %v; want complete", snap.Capture, err)
	}
}

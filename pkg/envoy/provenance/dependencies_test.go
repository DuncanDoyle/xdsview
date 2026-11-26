package provenance_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// merge is kgateway merge metadata naming TrafficPolicy ns/<name> for extAuth.
func merge(name string) string {
	return `"metadata": {"filter_metadata": {"merge.TrafficPolicy.gateway.kgateway.dev": {"extAuth": ["gateway.kgateway.dev/TrafficPolicy/ns/` + name + `"]}}}`
}

func hcmListener(name, rds string, metadata string) string {
	if metadata != "" {
		metadata += ", "
	}
	return `{"name": "` + name + `", "active_state": {"listener": {"name": "` + name + `", ` + metadata + `"filter_chains": [{"filters": [
 {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "` + rds + `"}, "http_filters": [
  {"name": "ext_auth/ns/authz", "disabled": true, "typed_config": {"@type": "` + authzType + `"}},
  {"name": "custom", "disabled": true, "typed_config": {"@type": "type.googleapis.com/example.Custom"}},
  {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}`
}

// Listeners L0 and L1 share route config "shared" (policy "src" on it); L2
// has its own; "unused" has a policy and no use. In "shared", route R0 has
// route_source metadata, a backend, a route policy and ext_authz enabled;
// R1 has only a policy and enables the custom filter, which no module claims.
var depsDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  ` + hcmListener("listener~80", "shared", merge("listener")) + `,
  ` + hcmListener("listener~81", "shared", "") + `,
  ` + hcmListener("listener~82", "own", "") + `]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [
  {"route_config": {"name": "shared", ` + merge("src") + `, "virtual_hosts": [{"name": "listener~80~example_com", "domains": ["example.com"], ` + merge("vhost") + `, "routes": [
   {"name": "r0", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_web_80"}, "typed_per_filter_config": {"ext_auth/ns/authz": {}},
    "metadata": {"filter_metadata": {"dev.kgateway.route_source": {"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "ns", "name": "web", "rule": "_rule-0", "match": "_match-0"},
     "merge.TrafficPolicy.gateway.kgateway.dev": {"extAuth": ["gateway.kgateway.dev/TrafficPolicy/ns/route"]}}}},
   {"name": "r1", "match": {"prefix": "/b"}, "typed_per_filter_config": {"custom": {}}, ` + merge("only") + `}]}]}},
  {"route_config": {"name": "own", "virtual_hosts": []}},
  {"route_config": {"name": "unused", ` + merge("unused") + `, "virtual_hosts": []}}]}]}`

func policy(name string) model.Ref {
	return model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: name}
}

func hasObject(d provenance.Dependencies, ref model.Ref, kind string) bool {
	return slices.Contains(d.Objects, provenance.Dependent{Object: ref, Kind: kind})
}

func hasGap(d provenance.Dependencies, subject model.ID, kind string) bool {
	return slices.ContainsFunc(d.Gaps, func(g provenance.Gap) bool { return g.Subject == subject && g.Kind == kind })
}

// TestDependencies (R6) covers the scope rules and the expected gaps.
func TestDependencies(t *testing.T) {
	snap, err := parser.Parse(strings.NewReader(depsDump), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ix := compose(t, snap, kgateway.New())
	rec := provenance.KindPolicyRecord

	// A listener block includes the claims of a source it shares with another listener.
	l1 := ix.Dependencies("L1")
	if !hasObject(l1, policy("src"), rec) || !hasObject(l1, policy("vhost"), rec) {
		t.Errorf("L1 objects %v lack the shared source's and vhost's records", l1.Objects)
	}
	// The unused source's record is in no listener's scope; the own source has none.
	for _, l := range []model.ID{"L0", "L1", "L2"} {
		if d := ix.Dependencies(l); hasObject(d, policy("unused"), rec) {
			t.Errorf("%s depends on the unused source's record", l)
		}
	}
	if d := ix.Dependencies("L2"); hasObject(d, policy("src"), rec) {
		t.Error("L2 depends on a source it doesn't use")
	}

	// A route block: its own claims, its vhost's and source's records, its
	// listener's claims, and its HCM's slot claims.
	r0 := ix.Dependencies("L0/C0/N0/RC/V0/R0")
	route := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}
	want := []provenance.Dependent{
		{Object: policy("listener"), Kind: rec},
		{Object: policy("route"), Kind: rec},
		{Object: policy("src"), Kind: rec},
		{Object: policy("vhost"), Kind: rec},
		{Object: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "ns", Name: "authz"}, Kind: provenance.KindExtensionSource},
		{Object: route, Kind: provenance.KindRouteSource},
		{Object: model.Ref{Kind: "Service", Namespace: "ns", Name: "web"}, Kind: provenance.KindBackendSource},
	}
	for _, d := range want {
		if !hasObject(r0, d.Object, d.Kind) {
			t.Errorf("R0 objects lack %+v", d)
		}
	}
	// The link from the route's record names that policy too.
	if !hasObject(r0, policy("route"), provenance.KindFilterLink) {
		t.Errorf("R0 objects lack the filter link's policy: %v", r0.Objects)
	}
	if hasObject(r0, policy("only"), rec) {
		t.Error("R0 depends on R1's record")
	}

	// A route with only a policy claim has a routeSource gap; a slot with
	// per-filter config and no extension claim has an extensionSource gap.
	r1 := ix.Dependencies("L0/C0/N0/RC/V0/R1")
	if !hasGap(r1, "L0/C0/N0/RC/V0/R1", provenance.KindRouteSource) {
		t.Errorf("R1 gaps %v lack routeSource", r1.Gaps)
	}
	if !hasGap(r1, "L0/C0/N0/F1", provenance.KindExtensionSource) {
		t.Errorf("R1 gaps %v lack the custom slot's extensionSource", r1.Gaps)
	}
	if hasGap(r0, "L0/C0/N0/RC/V0/R0", provenance.KindRouteSource) || hasGap(r0, "L0/C0/N0/RC/V0/R0", provenance.KindBackendSource) {
		t.Errorf("R0 has route or backend gaps: %v", r0.Gaps)
	}
	// No GatewaySource from kgateway metadata: every listener shows the gap.
	if !hasGap(r0, "L0", provenance.KindGatewaySource) {
		t.Errorf("R0 gaps %v lack the listener's gatewaySource", r0.Gaps)
	}
}

// TestDependencies_PolicyGaps: per-filter config at a level with no record
// there, and a filter that runs by default with no listener-level record, are
// policyRecord gaps. A dropped claim leaves a gap of its kind.
func TestDependencies_PolicyGaps(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}}
	snap.Listeners[0].FilterChains[0].HCM.HTTPFilters[0].Disabled = false
	// No input expects a listenerSource, so only the drop can make this gap (Codex #9).
	dropped := provenance.Claim{Subject: vhost0, Assertion: provenance.ListenerSource{Port: 80}, Certainty: model.Inferred}
	ix := compose(t, snap, fixed{"m", []provenance.Claim{dropped}})
	d := ix.Dependencies(route0)
	for _, g := range []provenance.Gap{
		{Subject: route0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelRoute},
		{Subject: vhost0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelVirtualHost},
		{Subject: slot0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelListener},
	} {
		if !slices.Contains(d.Gaps, g) {
			t.Errorf("gaps %v lack %+v", d.Gaps, g)
		}
	}
	if hasGap(d, "L0/C0/N0/F1", provenance.KindPolicyRecord) {
		t.Error("the router has a policyRecord gap")
	}
	if !hasGap(ix.Dependencies(route1), vhost0, provenance.KindListenerSource) {
		t.Error("the dropped claim's subject has no listenerSource gap")
	}
}

// TestDependencies_UpstreamSlot (Codex r4 P1): a route block relies on its
// HCM's router upstream filters. An upstream slot's ExtensionSource object
// joins Objects, and an upstream slot without one is a gap.
func TestDependencies_UpstreamSlot(t *testing.T) {
	snap := twoListeners()
	up := model.ID("L0/C0/N0/U0")
	snap.Listeners[0].FilterChains[0].HCM.UpstreamFilters = []model.HTTPFilter{
		{ID: up, Name: "ext_proc/ns/up", Type: "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"}}
	ext := model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "ns", Name: "up"}
	claim := provenance.Claim{Subject: up, Assertion: provenance.ExtensionSource{Extension: &ext, FilterKind: "ext_proc"},
		Certainty: model.Inferred, Evidence: ev(up, &ext)}
	gap := provenance.Gap{Subject: up, Kind: provenance.KindExtensionSource}

	d := compose(t, snap, fixed{"m", []provenance.Claim{claim}}).Dependencies(route0)
	if !hasObject(d, ext, provenance.KindExtensionSource) {
		t.Errorf("objects %v lack the upstream slot's extension", d.Objects)
	}
	if slices.Contains(d.Gaps, gap) {
		t.Errorf("claimed upstream slot has a gap: %v", d.Gaps)
	}
	if d := compose(t, snap, fixed{"m", nil}).Dependencies(route0); !slices.Contains(d.Gaps, gap) {
		t.Errorf("gaps %v lack %+v", d.Gaps, gap)
	}
}

// TestDependencies_BackendOverride (Codex r4 P1): a weighted backend's
// per-filter config is a gap. No record level holds it, so a linked route
// record doesn't cover it.
func TestDependencies_BackendOverride(t *testing.T) {
	snap := twoListeners()
	r1 := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[1]
	w := route1 + "/W0"
	r1.Cluster = ""
	r1.WeightedClusters = []model.WeightedCluster{{ID: w, Name: "c", Weight: 1, TypedPerFilterConfig: map[string]any{"authz": map[string]any{}}}}
	ix := compose(t, snap, fixed{"m", []provenance.Claim{record("r", route1, provenance.LevelRoute, policyA), link("r", route1, slot0)}})
	if len(provenance.Resolve[provenance.FilterLink](ix, route1).Claims) != 1 {
		t.Fatalf("the route's link was dropped: %v", ix.Warnings())
	}
	gap := provenance.Gap{Subject: w, Kind: provenance.KindPolicyRecord, Slot: slot0}
	for _, block := range []model.ID{route1, w, vhost0} {
		if d := ix.Dependencies(block); !slices.Contains(d.Gaps, gap) {
			t.Errorf("%s gaps %v lack %+v", block, d.Gaps, gap)
		}
	}
}

// TestDependencies_EntryWithoutSlot (Codex r4 P1): per-filter config for a
// filter the HCM doesn't install is a gap with no slot, at every level.
func TestDependencies_EntryWithoutSlot(t *testing.T) {
	snap := twoListeners()
	ghost := map[string]any{"ghost": map[string]any{}}
	hcm := snap.Listeners[0].FilterChains[0].HCM
	snap.RouteConfigs[0].TypedPerFilterConfig = ghost
	hcm.RouteConfig.TypedPerFilterConfig = ghost
	hcm.RouteConfig.VirtualHosts[0].Routes[1].TypedPerFilterConfig = ghost
	ix := compose(t, snap, fixed{"m", []provenance.Claim{record("src", "RD0", provenance.LevelRouteConfig, policyA)}})
	rec := provenance.KindPolicyRecord
	for block, want := range map[model.ID][]provenance.Gap{
		route1: {{Subject: "RD0", Kind: rec, Level: provenance.LevelRouteConfig}, {Subject: route1, Kind: rec, Level: provenance.LevelRoute}},
		route0: {{Subject: "RD0", Kind: rec, Level: provenance.LevelRouteConfig}},
		"RD0":  {{Subject: "RD0", Kind: rec, Level: provenance.LevelRouteConfig}},
	} {
		d := ix.Dependencies(block)
		for _, g := range want {
			if !slices.Contains(d.Gaps, g) {
				t.Errorf("%s gaps %v lack %+v", block, d.Gaps, g)
			}
		}
	}
}

// TestDependencies_ReportedSource (Codex r4 P2): the static entry Envoy
// reports for an inline config is that config, not an unused source. Its
// block is covered by the inline source's placement evidence, and keeps the
// inline source's gap without it.
func TestDependencies_ReportedSource(t *testing.T) {
	snap := twoListeners()
	inline := model.ID("L0/C0/N0/RCI")
	authz := map[string]any{"authz": map[string]any{}}
	hcm := snap.Listeners[0].FilterChains[0].HCM
	hcm.RouteConfigUse.Source = inline
	hcm.RouteConfig.TypedPerFilterConfig = authz
	snap.RouteConfigs[0] = model.RouteConfigSource{ID: inline, Kind: model.SourceInline, Name: "l0", Reported: "RS0", TypedPerFilterConfig: authz}
	snap.RouteConfigs = append(snap.RouteConfigs, model.RouteConfigSource{ID: "RS0", Kind: model.SourceStatic, Name: "l0", TypedPerFilterConfig: authz})
	rec := record("src", inline, provenance.LevelRouteConfig, policyA)

	ix := compose(t, snap, fixed{"m", []provenance.Claim{rec, link("src", route0, slot0), link("src", route1, slot0)}})
	for _, block := range []model.ID{"RS0", inline} {
		d := ix.Dependencies(block)
		if slices.ContainsFunc(d.Gaps, func(g provenance.Gap) bool {
			return g.Kind == provenance.KindPolicyRecord && g.Level == provenance.LevelRouteConfig
		}) {
			t.Errorf("%s has route config policyRecord gaps: %v", block, d.Gaps)
		}
		if !hasObject(d, policyA, provenance.KindPolicyRecord) {
			t.Errorf("%s objects %v lack the inline source's record", block, d.Objects)
		}
	}
	gap := provenance.Gap{Subject: inline, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelRouteConfig}
	if d := compose(t, snap, fixed{"m", []provenance.Claim{rec}}).Dependencies("RS0"); !slices.Contains(d.Gaps, gap) {
		t.Errorf("RS0 gaps %v lack %+v", d.Gaps, gap)
	}
}

// TestDependencies_ListenerGap (Codex r2 listener-level gap, r3): a default-on
// slot keeps its listener-level gap for a route unless a link on that route
// places a listener-level record on the slot. A sibling's link never closes
// it. A closing record's objects are in Dependencies, even for a block that
// doesn't hold the listener.
func TestDependencies_ListenerGap(t *testing.T) {
	snap := twoListeners()
	hcm := snap.Listeners[0].FilterChains[0].HCM
	hcm.HTTPFilters[0].Disabled = false
	cors := model.ID("L0/C0/N0/F2")
	hcm.HTTPFilters = append(hcm.HTTPFilters, model.HTTPFilter{ID: cors, Name: "cors", Type: "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors", Disabled: true})
	for i := range hcm.RouteConfig.VirtualHosts[0].Routes {
		hcm.RouteConfig.VirtualHosts[0].Routes[i].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}, "cors": map[string]any{}}
	}
	gap := provenance.Gap{Subject: slot0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelListener}
	rec := record("l", l0, provenance.LevelListener, policyA)
	blocks := []model.ID{route0, route1, "RD0"}
	for _, tc := range []struct {
		name   string
		claims []provenance.Claim
		gap    []bool // per block
	}{
		{"unlinked listener record", []provenance.Claim{rec}, []bool{true, true, true}},
		{"listener record for another filter", []provenance.Claim{rec, link("l", route0, cors)}, []bool{true, true, true}},
		{"linked on R0 only", []provenance.Claim{rec, link("l", route0, slot0)}, []bool{false, true, true}},
		{"linked on every route", []provenance.Claim{rec, link("l", route0, slot0), link("l", route1, slot0)}, []bool{false, false, false}},
	} {
		ix := compose(t, snap, fixed{"m", tc.claims})
		for i, block := range blocks {
			d := ix.Dependencies(block)
			if got := slices.Contains(d.Gaps, gap); got != tc.gap[i] {
				t.Errorf("%s: %s slot gap = %v, want %v", tc.name, block, got, tc.gap[i])
			}
			if !tc.gap[i] && !hasObject(d, policyA, provenance.KindPolicyRecord) {
				t.Errorf("%s: %s objects %v lack the listener record's policy", tc.name, block, d.Objects)
			}
		}
	}
}

// TestDependencies_PolicyGapsPerSlot (Codex #1): per-filter config for slot S
// at level X is covered only by a link placing a level-X record on S. A linked
// record for one filter leaves another filter's config a gap.
func TestDependencies_PolicyGapsPerSlot(t *testing.T) {
	snap := twoListeners()
	hcm := snap.Listeners[0].FilterChains[0].HCM
	cors := model.ID("L0/C0/N0/F2")
	hcm.HTTPFilters = append(hcm.HTTPFilters, model.HTTPFilter{ID: cors, Name: "cors", Type: "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors", Disabled: true})
	both := map[string]any{"authz": map[string]any{}, "cors": map[string]any{}}
	hcm.RouteConfig.VirtualHosts[0].Routes[0].TypedPerFilterConfig = both
	hcm.RouteConfig.VirtualHosts[0].TypedPerFilterConfig = both
	ix := compose(t, snap, fixed{"m", []provenance.Claim{
		record("route", route0, provenance.LevelRoute, policyA),
		record("vhost", vhost0, provenance.LevelVirtualHost, policyB),
		link("route", route0, cors),
		link("vhost", route0, cors),
	}})
	d := ix.Dependencies(route0)
	rec := provenance.KindPolicyRecord
	for _, g := range []provenance.Gap{
		{Subject: route0, Kind: rec, Slot: slot0, Level: provenance.LevelRoute},
		{Subject: vhost0, Kind: rec, Slot: slot0, Level: provenance.LevelVirtualHost},
	} {
		if !slices.Contains(d.Gaps, g) {
			t.Errorf("gaps %v lack %+v", d.Gaps, g)
		}
	}
	for _, g := range d.Gaps {
		if g.Kind == rec && g.Slot == cors {
			t.Errorf("linked cors config has a gap: %+v", g)
		}
	}
}

// TestDependencies_PlacementPerRoute (Codex r2 #1, N2): a vhost-level gap on
// a slot closes for a route only when a link on that route places a vhost
// record on the slot, with the record and link both at inferred or exact.
func TestDependencies_PlacementPerRoute(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}}
	gap := provenance.Gap{Subject: vhost0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelVirtualHost}
	vhRecord := func(c model.Certainty) provenance.Claim {
		r := record("vhost", vhost0, provenance.LevelVirtualHost, policyA)
		r.Certainty = c
		return r
	}
	onRoute := func(route model.ID, c model.Certainty) provenance.Claim {
		l := link("vhost", route, slot0)
		l.Certainty = c
		return l
	}
	for _, tc := range []struct {
		name   string
		claims []provenance.Claim
		gap    bool
	}{
		{"linked on this route", []provenance.Claim{vhRecord(model.Exact), onRoute(route0, model.Inferred)}, false},
		{"linked on a sibling route", []provenance.Claim{vhRecord(model.Exact), onRoute(route1, model.Inferred)}, true},
		{"two links from the record on this route", []provenance.Claim{vhRecord(model.Exact), onRoute(route0, model.Inferred), onRoute(route0, model.Inferred)}, true},
		{"ambiguous link", []provenance.Claim{vhRecord(model.Exact), onRoute(route0, model.Ambiguous)}, true},
		{"unknown link", []provenance.Claim{vhRecord(model.Exact), onRoute(route0, model.Unknown)}, true},
		{"unknown record", []provenance.Claim{vhRecord(model.Unknown), onRoute(route0, model.Inferred)}, true},
	} {
		ix := compose(t, snap, fixed{"m", tc.claims})
		if got := slices.Contains(ix.Dependencies(route0).Gaps, gap); got != tc.gap {
			t.Errorf("%s: vhost gap on R0 = %v, want %v", tc.name, got, tc.gap)
		}
	}
	// The sibling's own view is closed by its own link.
	ix := compose(t, snap, fixed{"m", []provenance.Claim{vhRecord(model.Exact), onRoute(route1, model.Inferred)}})
	if slices.Contains(ix.Dependencies(route1).Gaps, gap) {
		t.Error("R1's own link leaves its vhost gap")
	}
	if !slices.Contains(ix.Dependencies(vhost0).Gaps, gap) {
		t.Error("the vhost block has no gap though R0 has no link")
	}
}

// TestDependencies_SourceBlock (Codex r2 N1): a route config source block
// keeps a policyRecord gap for its per-filter config until links on every
// route it serves place a source-level record on the slot.
func TestDependencies_SourceBlock(t *testing.T) {
	snap := twoListeners()
	// The parser sets per-filter config on both the source and its use's copy.
	snap.RouteConfigs[0].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}}
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.TypedPerFilterConfig = snap.RouteConfigs[0].TypedPerFilterConfig
	gap := provenance.Gap{Subject: "RD0", Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelRouteConfig}
	rec := record("src", "RD0", provenance.LevelRouteConfig, policyA)
	for _, tc := range []struct {
		name   string
		claims []provenance.Claim
		gap    bool
	}{
		{"no record", nil, true},
		{"one route of two linked", []provenance.Claim{rec, link("src", route0, slot0)}, true},
		{"every route linked", []provenance.Claim{rec, link("src", route0, slot0), link("src", route1, slot0)}, false},
	} {
		ix := compose(t, snap, fixed{"m", tc.claims})
		d := ix.Dependencies("RD0")
		if got := slices.Contains(d.Gaps, gap); got != tc.gap {
			t.Errorf("%s: RD0 gap = %v, want %v (gaps %v)", tc.name, got, tc.gap, d.Gaps)
		}
	}
}

// TestDependencies_UnusedSource (Codex r3 N1): a source no HCM uses keeps a
// policyRecord gap while it has per-filter config, a record on it or not,
// since no link can place that config on a slot.
func TestDependencies_UnusedSource(t *testing.T) {
	snap := twoListeners()
	snap.RouteConfigs = append(snap.RouteConfigs,
		model.RouteConfigSource{ID: "RD2", Kind: model.SourceDynamic, Name: "unused", TypedPerFilterConfig: map[string]any{"authz": map[string]any{}}},
		model.RouteConfigSource{ID: "RD3", Kind: model.SourceDynamic, Name: "bare"})
	gap := provenance.Gap{Subject: "RD2", Kind: provenance.KindPolicyRecord, Level: provenance.LevelRouteConfig}
	for _, claims := range [][]provenance.Claim{nil, {record("src", "RD2", provenance.LevelRouteConfig, policyA)}} {
		ix := compose(t, snap, fixed{"m", claims})
		if d := ix.Dependencies("RD2"); !slices.Contains(d.Gaps, gap) {
			t.Errorf("%d records: RD2 gaps %v lack %+v", len(claims), d.Gaps, gap)
		}
		if d := ix.Dependencies("RD3"); hasGap(d, "RD3", provenance.KindPolicyRecord) {
			t.Errorf("RD3 without per-filter config has a gap: %v", d.Gaps)
		}
	}
}

// TestDependencies_ClosersObjects (Codex r3 P1): every claim that closes a gap
// adds its evidence objects, even when it sits outside the block's scope.
func TestDependencies_ClosersObjects(t *testing.T) {
	snap := twoListeners()
	snap.RouteConfigs[0].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}}
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.TypedPerFilterConfig = snap.RouteConfigs[0].TypedPerFilterConfig
	cite := func(l provenance.Claim) provenance.Claim {
		p := policyB
		l.Evidence = append(l.Evidence, ev(route0, &p)...)
		return l
	}
	ix := compose(t, snap, fixed{"m", []provenance.Claim{
		record("src", "RD0", provenance.LevelRouteConfig, policyA),
		cite(link("src", route0, slot0)), link("src", route1, slot0),
	}})
	d := ix.Dependencies("RD0")
	if hasGap(d, "RD0", provenance.KindPolicyRecord) {
		t.Fatalf("RD0 gaps %v: the links should close it", d.Gaps)
	}
	if !hasObject(d, policyB, provenance.KindFilterLink) {
		t.Errorf("RD0 objects %v lack the closing link's object", d.Objects)
	}
}

// TestDependencies_Bootstrap: a static listener, everything under it, its
// inline source and that source's reported static copy come from the proxy's
// startup config. They add no gaps and no objects; a dynamic listener keeps both.
func TestDependencies_Bootstrap(t *testing.T) {
	snap := twoListeners()
	l := &snap.Listeners[0]
	l.Static = true
	hcm := l.FilterChains[0].HCM
	inline := model.ID("L0/C0/N0/RCI")
	authz := map[string]any{"authz": map[string]any{}}
	hcm.HTTPFilters[0].Disabled = false
	hcm.RouteConfigUse.Source = inline
	hcm.RouteConfig.TypedPerFilterConfig = authz
	hcm.RouteConfig.VirtualHosts[0].Routes[1].MirrorClusters = []string{"m"}
	hcm.RouteConfig.VirtualHosts[0].Routes[1].WeightedClusters = []model.WeightedCluster{{ID: route1 + "/W0", Name: "c", Weight: 1, TypedPerFilterConfig: authz}}
	l.FilterChains[0].NetworkFilters = []model.NetworkFilter{{ID: "L0/C0/N0", Name: "hcm"}, {ID: "L0/C0/N1", Name: "rbac"}}
	snap.RouteConfigs[0] = model.RouteConfigSource{ID: inline, Kind: model.SourceInline, Name: "l0", Reported: "RS0", TypedPerFilterConfig: authz}
	snap.RouteConfigs = append(snap.RouteConfigs, model.RouteConfigSource{ID: "RS0", Kind: model.SourceStatic, Name: "l0", TypedPerFilterConfig: authz})
	// A route claim, and a dropped claim, on bootstrap subjects.
	claims := []provenance.Claim{source(route0, routeA, model.Exact),
		{Subject: vhost0, Assertion: provenance.ListenerSource{Port: 80}, Certainty: model.Inferred}}
	ix := compose(t, snap, fixed{"m", claims})

	for _, block := range []model.ID{l0, vhost0, route0, route1, route1 + "/W0", inline, "RS0"} {
		d := ix.Dependencies(block)
		if len(d.Gaps) != 0 || len(d.Objects) != 0 {
			t.Errorf("%s: gaps %v, objects %v, want none", block, d.Gaps, d.Objects)
		}
	}
	if d := ix.Dependencies(otherR); !hasGap(d, l1, provenance.KindGatewaySource) || !hasGap(d, otherR, provenance.KindRouteSource) {
		t.Errorf("dynamic route gaps %v lack its listener and route gaps", d.Gaps)
	}
}

// TestDependencies_StaticListenerRDS (Codex r6 P1): a route config that a
// static listener fetches over RDS is dynamic. Its virtual hosts, routes,
// backends and per-filter config keep their gaps; the listener and its slots stay bootstrap.
func TestDependencies_StaticListenerRDS(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].Static = true
	ix := compose(t, snap, fixed{"m", nil})
	d := ix.Dependencies(route0)
	for _, g := range []provenance.Gap{
		{Subject: vhost0, Kind: provenance.KindHostSource},
		{Subject: route0, Kind: provenance.KindRouteSource},
		{Subject: route0, Kind: provenance.KindBackendSource},
		{Subject: route0, Kind: provenance.KindPolicyRecord, Slot: slot0, Level: provenance.LevelRoute},
	} {
		if !slices.Contains(d.Gaps, g) {
			t.Errorf("gaps %v lack %+v", d.Gaps, g)
		}
	}
	for _, g := range d.Gaps {
		if g.Subject == l0 || g.Subject == slot0 {
			t.Errorf("bootstrap subject is a gap: %+v", g)
		}
	}
}

// TestRecords lists a route's records at every level, most specific first.
func TestRecords(t *testing.T) {
	snap, _ := parser.Parse(strings.NewReader(depsDump), parser.Options{})
	ix := compose(t, snap, kgateway.New())
	var got []string
	for _, c := range provenance.Records(ix, snap, "L0/C0/N0/RC/V0/R0") {
		p := c.Assertion.(provenance.PolicyRecord)
		got = append(got, p.Policy.Name+"@"+string(p.Level))
	}
	want := []string{"route@route", "vhost@virtualHost", "src@routeConfig", "listener@listener"}
	if !slices.Equal(got, want) {
		t.Errorf("records %v, want %v", got, want)
	}
}

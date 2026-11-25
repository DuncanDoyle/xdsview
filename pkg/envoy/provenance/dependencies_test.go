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
	return slices.Contains(d.Gaps, provenance.Gap{Subject: subject, Kind: kind})
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
	dropped := source(route1, routeA, model.Inferred)
	dropped.Evidence = nil
	ix := compose(t, snap, fixed{"m", []provenance.Claim{dropped}})
	d := ix.Dependencies(route0)
	for _, g := range []provenance.Gap{
		{Subject: route0, Kind: provenance.KindPolicyRecord},
		{Subject: vhost0, Kind: provenance.KindPolicyRecord},
		{Subject: slot0, Kind: provenance.KindPolicyRecord},
	} {
		if !hasGap(d, g.Subject, g.Kind) {
			t.Errorf("gaps %v lack %+v", d.Gaps, g)
		}
	}
	if hasGap(d, "L0/C0/N0/F1", provenance.KindPolicyRecord) {
		t.Error("the router has a policyRecord gap")
	}
	if !hasGap(ix.Dependencies(route1), route1, provenance.KindRouteSource) {
		t.Error("the dropped claim's subject has no routeSource gap")
	}

	// A listener-level record closes the default-on slot's gap.
	p := policyA
	ix = compose(t, snap, fixed{"m", []provenance.Claim{{Subject: l0, Assertion: provenance.PolicyRecord{Policy: p, Field: "f", Level: provenance.LevelListener},
		Certainty: model.Exact, Evidence: ev(l0, &p)}}})
	if hasGap(ix.Dependencies(route0), slot0, provenance.KindPolicyRecord) {
		t.Error("slot gap despite a listener-level record")
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

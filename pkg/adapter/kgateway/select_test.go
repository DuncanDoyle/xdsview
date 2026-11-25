package kgateway_test

import (
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

func httpRoute(ns, name string) model.Ref {
	return model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: ns, Name: name}
}

// selectSnap has one vhost "v" with routes:
//
//	R0 metadata: exact reading web/ns rule 0
//	R1 reads as a/b-c or a-b/c
//	R2 names a-b/c rule 0, but its rule name is invalid: no reading
//	R3 reads as d/e-f or d-e/f
//	R4 reads as other/ns only
func selectSnap() *model.Snapshot {
	routes := []model.Route{
		{Name: "v-route-0-httproute-web-ns-0-0-matcher-0", Metadata: map[string]any{"dev.kgateway.route_source": map[string]any{
			"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "ns", "name": "web", "rule": "_rule-0", "match": "_match-0"}}},
		{Name: "v-route-1-httproute-a-b-c-0-0-matcher-0"},
		{Name: "v-route-2-httproute-a-b-c-0-0-Bad_Rule-matcher-0"},
		{Name: "v-route-3-httproute-d-e-f-0-0-matcher-0"},
		{Name: "v-route-4-httproute-other-ns-0-0-matcher-0"},
	}
	hcm := &model.HCMConfig{
		HTTPFilters:    []model.HTTPFilter{{Name: "envoy.filters.http.router", Type: snaptest.RouterType}},
		RouteConfigUse: model.RouteConfigUse{Source: "RD0", State: model.RouteConfigFound},
		RouteConfig:    &model.RouteConfig{Name: "rc", VirtualHosts: []model.VirtualHost{{Name: "v", Routes: routes}}},
	}
	return snaptest.IDs(&model.Snapshot{
		Listeners:    []model.Listener{{Name: "listener~80", State: model.ListenerState{Status: model.ListenerActive}, FilterChains: []model.NetworkFilterChain{{HCM: hcm}}}},
		RouteConfigs: []model.RouteConfigSource{{ID: "RD0", Kind: model.SourceDynamic, Name: "rc"}},
	})
}

func route(n int) model.ID { return model.ID("L0/C0/N0/RC/V0/R" + string(rune('0'+n))) }

// TestSelect covers selection rules 1–6 (R8).
func TestSelect(t *testing.T) {
	snap := selectSnap()
	ix := compose(t, snap)
	zero, one := 0, 1
	abc := httpRoute("c", "a-b")

	cases := []struct {
		name                         string
		q                            kgateway.Query
		routes, ambiguous, unresolve []model.ID
		narrowed                     []model.ID
	}{
		{"exact reading", kgateway.Query{Route: httpRoute("ns", "web")}, []model.ID{route(0)}, nil, nil, nil},
		{"rule index matches", kgateway.Query{Route: httpRoute("ns", "web"), RuleIndex: &zero}, []model.ID{route(0)}, nil, nil, nil},
		{"rule index differs", kgateway.Query{Route: httpRoute("ns", "web"), RuleIndex: &one}, nil, nil, nil, nil},
		{"rule index and name, both must match", kgateway.Query{Route: httpRoute("ns", "web"), RuleIndex: &zero, RuleName: "x"}, nil, nil, nil, nil},
		{"several readings", kgateway.Query{Route: abc}, nil, []model.ID{route(1)}, []model.ID{route(2)}, nil},
		{"narrowed by Known stays ambiguous", kgateway.Query{Route: abc, Known: []model.Ref{abc}}, nil, []model.ID{route(1)}, []model.ID{route(2)}, []model.ID{route(1)}},
		{"every reading outside Known: Known ignored", kgateway.Query{Route: httpRoute("f", "d-e"), Known: []model.Ref{abc}}, nil, []model.ID{route(3)}, nil, nil},
		{"Known never removes an exact reading", kgateway.Query{Route: httpRoute("ns", "web"), Known: []model.Ref{abc}}, []model.ID{route(0)}, nil, nil, nil},
		{"other kinds are not selected", kgateway.Query{Route: model.Ref{Group: "gateway.networking.k8s.io", Kind: "GRPCRoute", Namespace: "ns", Name: "web"}}, nil, nil, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sel := kgateway.Select(ix, snap, c.q)
			var narrowed []model.ID
			for _, n := range sel.Narrowed {
				narrowed = append(narrowed, n.Route)
			}
			if !slices.Equal(sel.Routes, c.routes) || !slices.Equal(sel.Ambiguous, c.ambiguous) || !slices.Equal(sel.Unresolved, c.unresolve) || !slices.Equal(narrowed, c.narrowed) {
				t.Errorf("selection = %+v\nwant routes %v ambiguous %v unresolved %v narrowed %v", sel, c.routes, c.ambiguous, c.unresolve, c.narrowed)
			}
		})
	}

	// The narrowing names what Known kept and removed; certainty isn't raised,
	// and the index keeps every candidate.
	sel := kgateway.Select(ix, snap, kgateway.Query{Route: abc, Known: []model.Ref{abc}})
	n := sel.Narrowed[0]
	if len(n.Kept) != 1 || n.Kept[0].Assertion.(provenance.RouteSource).Route != abc || len(n.Dropped) != 1 {
		t.Errorf("narrowing = %+v", n)
	}
	if res := provenance.Resolve[provenance.RouteSource](ix, route(1)); res.Certainty != model.Ambiguous || len(res.Candidates) != 2 {
		t.Errorf("index after Select = %+v, want both readings", res)
	}
}

// TestNarrow: live mode without a query narrows every route Known decides.
func TestNarrow(t *testing.T) {
	ix := compose(t, selectSnap())
	got := kgateway.Narrow(ix, []model.Ref{httpRoute("c", "a-b"), httpRoute("ns", "other")})
	if len(got) != 1 || got[0].Route != route(1) {
		t.Errorf("narrowings = %+v, want R1 only", got)
	}
	if got := kgateway.Narrow(ix, nil); got != nil {
		t.Errorf("no Known narrowed %+v", got)
	}
}

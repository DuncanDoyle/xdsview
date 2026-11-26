package provenance_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// input returns the input of block whose gap is g.
func input(t *testing.T, ix *provenance.Index, block model.ID, g provenance.Gap) provenance.Input {
	t.Helper()
	for _, in := range ix.Inputs(block) {
		if in.Gap == g {
			return in
		}
	}
	t.Fatalf("%s: no input %+v", block, g)
	return provenance.Input{}
}

func refs(cs []provenance.Candidate) []model.Ref {
	var out []model.Ref
	for _, c := range cs {
		out = append(out, c.Ref)
	}
	return out
}

// TestInputs_Status: each input carries its status, a specific gap reason,
// and for a route every RouteSource candidate with its claim's evidence.
func TestInputs_Status(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[1].MirrorClusters = []string{"m"}
	snap.Listeners[1].Static = true
	snap.RouteConfigs[1].Kind = model.SourceInline // L1's routes are bootstrap
	// R0 reads two ways; R1 has only an unknown reading; L0's listener has a dropped claim.
	ix := compose(t, snap, fixed{"m", []provenance.Claim{
		source(route0, routeA, model.Inferred), source(route0, routeB, model.Inferred),
		source(route1, routeB, model.Unknown),
		{Subject: l0, Assertion: provenance.GatewaySource{Gateway: routeA}, Certainty: model.Exact},
	}})
	rs := provenance.KindRouteSource
	for _, tc := range []struct {
		block  model.ID
		gap    provenance.Gap
		status provenance.InputStatus
		reason provenance.GapReason
		cands  []model.Ref
	}{
		{route0, provenance.Gap{Subject: route0, Kind: rs}, provenance.InputGap, provenance.ReasonAmbiguous, []model.Ref{routeA, routeB}},
		{route1, provenance.Gap{Subject: route1, Kind: rs}, provenance.InputGap, provenance.ReasonNoEvidence, []model.Ref{routeB}},
		{route1, provenance.Gap{Subject: route1, Kind: provenance.KindBackendSource, Cluster: "m"}, provenance.InputGap, provenance.ReasonNoEvidence, nil},
		{route1, provenance.Gap{Subject: l0, Kind: provenance.KindGatewaySource}, provenance.InputGap, provenance.ReasonDropped, nil},
		{otherR, provenance.Gap{Subject: otherR, Kind: rs}, provenance.InputBootstrap, "", nil},
	} {
		in := input(t, ix, tc.block, tc.gap)
		if in.Status != tc.status || in.Reason != tc.reason || !slices.Equal(refs(in.Candidates), tc.cands) {
			t.Errorf("%s %+v: got %s/%s %v, want %s/%s %v", tc.block, tc.gap, in.Status, in.Reason, refs(in.Candidates), tc.status, tc.reason, tc.cands)
		}
		for _, c := range in.Candidates {
			if len(c.Claim.Evidence) == 0 || c.Claim.Assertion.(provenance.RouteSource).Route != c.Ref {
				t.Errorf("%s: candidate %+v lacks its claim's evidence", tc.block, c)
			}
		}
	}

	// One exact reading covers the route; the candidate is listed though no cluster is consulted.
	ix = compose(t, snap, fixed{"m", []provenance.Claim{source(route0, routeA, model.Exact)}})
	in := input(t, ix, route0, provenance.Gap{Subject: route0, Kind: rs})
	if in.Status != provenance.InputCovered || in.Reason != "" || !slices.Equal(refs(in.Candidates), []model.Ref{routeA}) {
		t.Errorf("covered route: got %s/%s %v", in.Status, in.Reason, refs(in.Candidates))
	}
}

// TestInputs_Complete: a route's candidate list is complete only when every
// reading at the highest certainty is Exhaustive and no listed reading, at
// any certainty, is Incomplete.
// An incomplete, unaffirmed or empty list is a gap, never covered.
func TestInputs_Complete(t *testing.T) {
	partial := func(r model.Ref) provenance.Claim {
		c := source(route0, r, model.Inferred)
		c.Assertion = provenance.RouteSource{Route: r, Incomplete: true}
		return c
	}
	with := func(c model.Certainty, a provenance.RouteSource) provenance.Claim {
		cl := source(route0, a.Route, c)
		cl.Assertion = a
		return cl
	}
	rs := provenance.Gap{Subject: route0, Kind: provenance.KindRouteSource}
	for _, tc := range []struct {
		name     string
		claims   []provenance.Claim
		complete bool
		status   provenance.InputStatus
		reason   provenance.GapReason
	}{
		{"exact", []provenance.Claim{source(route0, routeA, model.Exact)}, true, provenance.InputCovered, ""},
		{"inferred", []provenance.Claim{source(route0, routeA, model.Inferred)}, true, provenance.InputCovered, ""},
		{"inferred incomplete", []provenance.Claim{partial(routeA)}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"ambiguous incomplete", []provenance.Claim{partial(routeA), partial(routeB)}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"lower incomplete reading", []provenance.Claim{partial(routeB), source(route0, routeA, model.Exact)}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"unknown incomplete reading", []provenance.Claim{with(model.Unknown, provenance.RouteSource{Route: routeB, Incomplete: true}), source(route0, routeA, model.Exact)}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"exact incomplete", []provenance.Claim{with(model.Exact, provenance.RouteSource{Route: routeA, Exhaustive: true, Incomplete: true})}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"inferred unaffirmed", []provenance.Claim{with(model.Inferred, provenance.RouteSource{Route: routeA})}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"exact unaffirmed", []provenance.Claim{with(model.Exact, provenance.RouteSource{Route: routeA})}, false, provenance.InputGap, provenance.ReasonIncomplete},
		{"none", nil, false, provenance.InputGap, provenance.ReasonNoEvidence},
	} {
		in := input(t, compose(t, twoListeners(), fixed{"m", tc.claims}), route0, rs)
		if in.Complete != tc.complete || in.Status != tc.status || in.Reason != tc.reason {
			t.Errorf("%s: got complete=%t %s/%s, want complete=%t %s/%s", tc.name, in.Complete, in.Status, in.Reason, tc.complete, tc.status, tc.reason)
		}
	}
}

// TestInputs_UnresolvedUse: a route config use that joined no source hides
// routes the index can't see, so it is a gap in every block that holds it.
func TestInputs_UnresolvedUse(t *testing.T) {
	use := model.ID("L0/C0/N0/RC")
	for _, tc := range []struct {
		name   string
		state  model.RouteConfigState
		scoped bool
		static bool
	}{
		{"missing", model.RouteConfigMissing, false, false},
		{"ambiguous", model.RouteConfigAmbiguous, false, false},
		{"unknown", model.RouteConfigUnknown, false, false},
		{"scoped", model.RouteConfigUnknown, true, false},
		{"static listener over RDS", model.RouteConfigUnknown, false, true},
	} {
		snap := twoListeners()
		hcm := snap.Listeners[0].FilterChains[0].HCM
		hcm.RouteConfigUse = model.RouteConfigUse{ID: use, State: tc.state}
		hcm.RouteConfig, hcm.ScopedRoutes = nil, tc.scoped
		snap.Listeners[0].Static = tc.static
		ix := compose(t, snap, fixed{"m", nil})
		for _, block := range []model.ID{l0, use} {
			in := input(t, ix, block, provenance.Gap{Subject: use})
			if in.Status != provenance.InputGap || in.Reason != provenance.ReasonUnresolved {
				t.Errorf("%s: %s: use is %s/%s, want gap/unresolved", tc.name, block, in.Status, in.Reason)
			}
		}
	}
	// A joined use is no input of its own.
	ix := compose(t, twoListeners(), fixed{"m", nil})
	for _, in := range ix.Inputs(l0) {
		if in.Subject == use {
			t.Errorf("found use has input %+v", in)
		}
	}
}

// TestInputs_DirectResponse: a direct response is what serves its route, and
// may come from an object other than the HTTPRoute, so it is always a
// backendSource gap. A redirect comes from the HTTPRoute and the listener,
// which routeSource and gatewaySource cover: it adds no input.
func TestInputs_DirectResponse(t *testing.T) {
	snap := twoListeners()
	routes := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes
	routes[0].Cluster, routes[0].Action = "", model.RouteAction{Kind: model.ActionDirectResponse, DirectResponse: &model.DirectResponse{Status: 503}}
	routes[1].Cluster, routes[1].Action = "", model.RouteAction{Kind: model.ActionRedirect, Redirect: &model.Redirect{ResponseCode: 301}}
	// Even an exact backend claim doesn't cover the direct response.
	ix := compose(t, snap, fixed{"m", []provenance.Claim{
		{Subject: route0, Assertion: provenance.BackendSource{Backend: policyA}, Certainty: model.Exact, Evidence: ev(route0, &policyA)},
	}})
	g := provenance.Gap{Subject: route0, Kind: provenance.KindBackendSource}
	for _, block := range []model.ID{route0, vhost0, l0, "RD0"} {
		in := input(t, ix, block, g)
		if in.Status != provenance.InputGap || in.Reason != provenance.ReasonNotCoverable {
			t.Errorf("%s: direct response is %s/%s, want gap/notCoverable", block, in.Status, in.Reason)
		}
		if !slices.Contains(ix.Dependencies(block).Gaps, g) {
			t.Errorf("%s: Dependencies lacks %+v", block, g)
		}
	}
	for _, in := range ix.Inputs(route1) {
		if in.Subject == route1 && in.Kind == provenance.KindBackendSource {
			t.Errorf("redirect has input %+v", in)
		}
	}
}

// TestInputs_SourceBlock: a route config source block holds every use of
// it, so its inputs include each use's listener, virtual hosts, routes and backends.
func TestInputs_SourceBlock(t *testing.T) {
	ix := compose(t, twoListeners(), fixed{"m", nil})
	for _, g := range []provenance.Gap{
		{Subject: l0, Kind: provenance.KindGatewaySource},
		{Subject: vhost0, Kind: provenance.KindHostSource},
		{Subject: route0, Kind: provenance.KindRouteSource},
		{Subject: route1, Kind: provenance.KindBackendSource},
	} {
		if in := input(t, ix, "RD0", g); in.Status != provenance.InputGap {
			t.Errorf("RD0 %+v is %s, want gap", g, in.Status)
		}
	}
	for _, in := range ix.Inputs("RD0") {
		if in.Subject.Within(l1) {
			t.Errorf("RD0 has an input of L1: %+v", in)
		}
	}
}

// TestInputs_DroppedMirror (Codex r6 P2): a dropped mirror BackendSource
// claim is a gap on the mirror input, not on the route's primary backend.
func TestInputs_DroppedMirror(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[1].MirrorClusters = []string{"m"}
	backend := model.Ref{Kind: "Service", Namespace: "ns", Name: "svc"}
	primary := provenance.Claim{Subject: route1, Assertion: provenance.BackendSource{Backend: backend}, Certainty: model.Inferred, Evidence: ev(route1, &backend)}
	mirror := provenance.Claim{Subject: route1, Assertion: provenance.BackendSource{Backend: backend, Mirror: "m"}, Certainty: model.Inferred} // no evidence: dropped
	ix := compose(t, snap, fixed{"m", []provenance.Claim{primary, mirror}})
	bs := provenance.KindBackendSource
	if in := input(t, ix, route1, provenance.Gap{Subject: route1, Kind: bs}); in.Status != provenance.InputCovered {
		t.Errorf("primary backend is %s/%s, want covered", in.Status, in.Reason)
	}
	if in := input(t, ix, route1, provenance.Gap{Subject: route1, Kind: bs, Cluster: "m"}); in.Reason != provenance.ReasonDropped {
		t.Errorf("mirror backend is %s/%s, want gap/dropped", in.Status, in.Reason)
	}
}

// TestInputs_Mirror (scenario 02_4): a mirror cluster is an ordinary backend.
// The kgateway module claims it, its input is covered, and the route's own
// backend still resolves to its primary cluster alone.
func TestInputs_Mirror(t *testing.T) {
	paths, _ := filepath.Glob("../../../testdata/scenarios/02_4-single-policy/envoy/*/config_dump.json")
	if len(paths) == 0 {
		t.Fatal("no 02_4 dumps")
	}
	for _, path := range paths {
		snap := parseFile(t, path)
		ix := compose(t, snap, kgateway.New())
		mirrors := 0
		for _, l := range snap.Listeners {
			for _, fc := range l.FilterChains {
				if fc.HCM == nil || fc.HCM.RouteConfig == nil {
					continue
				}
				for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
					for _, r := range vh.Routes {
						for _, m := range r.MirrorClusters {
							mirrors++
							in := input(t, ix, r.ID, provenance.Gap{Subject: r.ID, Kind: provenance.KindBackendSource, Cluster: m})
							if in.Status != provenance.InputCovered {
								t.Errorf("%s: %s mirror %s is %s/%s, want covered", path, r.ID, m, in.Status, in.Reason)
							}
							if res := provenance.ResolveMirror(ix, r.ID, m); res.Certainty != model.Inferred || len(res.Candidates) != 1 || res.Candidates[0].Mirror != m {
								t.Errorf("%s: %s mirror %s resolves to %+v", path, r.ID, m, res)
							}
							if res := provenance.Resolve[provenance.BackendSource](ix, r.ID); len(res.Candidates) != 1 || res.Candidates[0].Mirror != "" {
								t.Errorf("%s: %s primary backend resolves to %+v", path, r.ID, res.Candidates)
							}
						}
					}
				}
			}
		}
		if mirrors == 0 {
			t.Errorf("%s: no mirror clusters", path)
		}
	}
}

// TestInputs_DerivesDependencies checks, for every block of every corpus dump
// and fixture, that Dependencies' gaps are exactly the inputs with gap status,
// and that every route candidate's object is in Objects.
func TestInputs_DerivesDependencies(t *testing.T) {
	for _, path := range dumps(t) {
		name, _ := filepath.Rel("../../../testdata", filepath.Dir(path))
		snap := parseFile(t, path)
		ix := compose(t, snap, kgateway.New())
		for _, b := range blockIDs(snap) {
			d := ix.Dependencies(b)
			gaps := map[provenance.Gap]bool{}
			for _, in := range ix.Inputs(b) {
				if in.Status == provenance.InputGap {
					gaps[in.Gap] = true
				}
				if in.Status == provenance.InputBootstrap {
					continue
				}
				for _, c := range in.Candidates {
					if !hasObject(d, c.Ref, provenance.KindRouteSource) {
						t.Errorf("%s: %s: candidate %v is no object", name, b, c.Ref)
					}
				}
			}
			if len(d.Gaps) != len(gaps) || slices.ContainsFunc(d.Gaps, func(g provenance.Gap) bool { return !gaps[g] }) {
				t.Errorf("%s: %s: gaps %v, inputs give %v", name, b, d.Gaps, slices.Collect(maps.Keys(gaps)))
			}
		}
	}
}

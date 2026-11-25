package main

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/filter"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// The parity tests compare the kgateway provenance module with the code it
// replaces, origin.Annotate plus krp's attribute, on every corpus dump and
// fixture. They go with that code (9.3 part B). The intended differences are
// the Phase 8 P1 fixes, none of which the corpus shows; TestParity_P1Fixes
// lists each one on a hand-built route.

// TestParity_Text renders every dump both ways: today's annotated snapshot
// with krp's contribution labels, and the unannotated snapshot with the
// provenance text hooks. The text is identical.
func TestParity_Text(t *testing.T) {
	for _, dump := range allDumps(t) {
		t.Run(dump, func(t *testing.T) {
			m := origin.Kgateway(origin.KgatewayOptions{Known: corpusKnown})
			old := origin.Annotate(parseFile(t, dump), m)
			oldChains := chain.Effective(old)
			var want bytes.Buffer
			if err := render.Text(&want, old, oldChains, render.Options{Labels: attribute(old, oldChains, m).labels}); err != nil {
				t.Fatal(err)
			}

			snap := parseFile(t, dump)
			ix, err := provenance.Compose(snap, kgateway.New())
			if err != nil {
				t.Fatal(err)
			}
			if w := ix.Warnings(); len(w) > 0 {
				t.Errorf("compose warnings: %v", w)
			}
			opts := render.Options{Labels: provenance.Labels(ix, kgateway.Narrow(ix, corpusKnown)), Trailer: provenance.Notes(ix)}
			var got bytes.Buffer
			if err := render.Text(&got, snap, chain.Effective(snap), opts); err != nil {
				t.Fatal(err)
			}
			if got.String() != want.String() {
				t.Errorf("text differs\n--- new\n%s\n--- today\n%s", got.String(), want.String())
			}
		})
	}
}

// oldKrpText is krp's text before part B: filter.Filter under --route, then
// origin.Annotate with Known, then attribute's labels.
func oldKrpText(t *testing.T, dump string, q *kgateway.Query, known []model.Ref) string {
	snap := parseFile(t, dump)
	if q != nil {
		opts := filter.Options{HTTPRouteName: q.Route.Name, HTTPRouteNamespace: q.Route.Namespace, RuleIndex: filter.AllRules, RuleName: q.RuleName}
		if q.RuleIndex != nil {
			opts.RuleIndex = *q.RuleIndex
		}
		snap = filter.Filter(snap, opts)
		known = append(known, q.Route)
	}
	m := origin.Kgateway(origin.KgatewayOptions{Known: known})
	snap = origin.Annotate(snap, m)
	chains := chain.Effective(snap)
	var b bytes.Buffer
	if err := render.Text(&b, snap, chains, render.Options{Labels: attribute(snap, chains, m).labels}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestParity_Krp runs krp's old and new paths on every dump and fixture: in
// file mode, in live mode with the corpus HTTPRoutes, and under a set of
// --route and --rule queries. The text is identical.
func TestParity_Krp(t *testing.T) {
	ruleIndex := func(i int) *int { return &i }
	queries := []kgateway.Query{
		{Route: httpRoute("kgateway-system", "api-example-com")},
		{Route: httpRoute("kgateway-system", "api-example-com"), RuleIndex: ruleIndex(0)},
		{Route: httpRoute("kgateway-system", "api-example-com"), RuleIndex: ruleIndex(1)},
		{Route: httpRoute("kgateway-system", "api-example-com"), RuleName: "a-b"},
		{Route: httpRoute("kgateway-system", "api-example-com"), RuleName: "a"},
		{Route: httpRoute("kgateway-system", "api-example-com-admin")},
		{Route: httpRoute("kgwctl-proxy", "orders")},
		{Route: httpRoute("kgwctl-proxy", "public")},
		{Route: httpRoute("kgateway-system", "does-not-exist")},
	}
	compared := 0
	for _, dump := range allDumps(t) {
		reqs := []request{{}, {live: true, known: corpusKnown}}
		for _, q := range queries {
			q.Known = []model.Ref{q.Route}
			reqs = append(reqs, request{query: &q})
		}
		for _, req := range reqs {
			res, err := build(parseFile(t, dump), req)
			if err != nil {
				t.Fatal(err)
			}
			want := oldKrpText(t, dump, req.query, req.known)
			if got := plainText(t, res); got != want {
				t.Errorf("%s %+v: text differs\n--- new\n%s\n--- old\n%s", dump, req.query, got, want)
			}
			compared++
		}
	}
	t.Logf("%d outputs compared", compared)
}

func httpRoute(ns, name string) model.Ref {
	return model.Ref{Group: gatewayGroup, Kind: "HTTPRoute", Namespace: ns, Name: name}
}

// TestParity_Structured compares, per occurrence, the resolved origins,
// policies and contributions with today's annotations.
func TestParity_Structured(t *testing.T) {
	for _, dump := range allDumps(t) {
		t.Run(dump, func(t *testing.T) {
			m := origin.Kgateway(origin.KgatewayOptions{Known: corpusKnown})
			old := origin.Annotate(parseFile(t, dump), m)
			contribs := attribute(old, chain.Effective(old), m)

			snap := parseFile(t, dump)
			ix, _ := provenance.Compose(snap, kgateway.New())
			narrowed := map[model.ID]provenance.Narrowing{}
			for _, n := range kgateway.Narrow(ix, corpusKnown) {
				narrowed[n.Route] = n
			}
			links := 0
			for _, l := range old.Listeners {
				if got := listenerOrigin(ix, l.ID); !reflect.DeepEqual(got, l.Origin) {
					t.Errorf("%s origin = %+v, today %+v", l.ID, got, l.Origin)
				}
				for _, fc := range l.FilterChains {
					if fc.HCM == nil || fc.HCM.RouteConfig == nil {
						continue
					}
					for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
						if got := vhostOrigin(ix, vh.ID); !reflect.DeepEqual(got, vh.Origin) {
							t.Errorf("%s origin = %+v, today %+v", vh.ID, got, vh.Origin)
						}
						for _, r := range vh.Routes {
							n, ok := narrowed[r.ID]
							if got := routeAnnotation(ix, r.ID, n, ok); !reflect.DeepEqual(got, r.Origin) {
								t.Errorf("%s origin = %+v, today %+v", r.ID, got, r.Origin)
							}
							if got := routePolicies(ix, snap, r.ID); !reflect.DeepEqual(got, r.Policies) {
								t.Errorf("%s policies = %+v, today %+v", r.ID, got, r.Policies)
							}
							for _, f := range fc.HCM.HTTPFilters {
								got := slotContributions(ix, r.ID, f.ID)
								if want := contribs[r.ID][f.ID]; !reflect.DeepEqual(got, want) {
									t.Errorf("%s %s contributions = %+v, today %+v", r.ID, f.ID, got, want)
								}
								links += len(got)
							}
						}
					}
				}
			}
			t.Logf("%d contributions compared", links)
		})
	}
}

func listenerOrigin(ix *provenance.Index, id model.ID) *model.ListenerOrigin {
	ls := provenance.Resolve[provenance.ListenerSource](ix, id)
	if len(ls.Candidates) != 1 {
		return nil
	}
	return &model.ListenerOrigin{Certainty: ls.Certainty, Port: ls.Candidates[0].Port, Section: ls.Candidates[0].Section}
}

func vhostOrigin(ix *provenance.Index, id model.ID) *model.VirtualHostOrigin {
	l := listenerOrigin(ix, id)
	h := provenance.Resolve[provenance.HostSource](ix, id)
	if l == nil || len(h.Candidates) != 1 {
		return nil
	}
	return &model.VirtualHostOrigin{Certainty: h.Certainty, Listener: *l, Hostname: h.Candidates[0].Hostname}
}

// routeAnnotation is the new route origin in today's shape.
func routeAnnotation(ix *provenance.Index, id model.ID, n provenance.Narrowing, narrowed bool) *model.RouteAnnotation {
	res := provenance.Resolve[provenance.RouteSource](ix, id)
	cands, certainty := res.Candidates, res.Certainty
	if narrowed {
		cands = nil
		for _, c := range n.Kept {
			cands = append(cands, c.Assertion.(provenance.RouteSource))
		}
		certainty = model.Inferred
		if len(cands) > 1 {
			certainty = model.Ambiguous
		}
	}
	if len(cands) == 0 {
		return nil
	}
	a := &model.RouteAnnotation{Certainty: certainty, NarrowedByKnown: narrowed && len(cands) == 1}
	for _, c := range cands {
		a.Candidates = append(a.Candidates, model.RouteOrigin{Route: c.Route, RuleIndex: c.RuleIndex, RuleName: c.RuleName, MatchIndex: c.MatchIndex, IndexCertainty: c.IndexCertainty})
	}
	return a
}

func routePolicies(ix *provenance.Index, snap *model.Snapshot, id model.ID) []model.PolicyRef {
	var out []model.PolicyRef
	for _, c := range provenance.Records(ix, snap, id) {
		if p := c.Assertion.(provenance.PolicyRecord); p.Level == provenance.LevelRoute {
			out = append(out, model.PolicyRef{Policy: p.Policy, Field: p.Field, Certainty: c.Certainty})
		}
	}
	return out
}

func slotContributions(ix *provenance.Index, route, slot model.ID) []model.Contribution {
	var out []model.Contribution
	if e := provenance.Resolve[provenance.ExtensionSource](ix, slot); len(e.Candidates) == 1 && e.Candidates[0].Extension != nil {
		out = append(out, model.Contribution{Kind: model.ContributionExtension, Ref: *e.Candidates[0].Extension, Certainty: e.Certainty})
	}
	for _, c := range ix.About(route) {
		if l, ok := c.Assertion.(provenance.FilterLink); ok && l.Slot == slot {
			rec, _ := ix.Record(c)
			p := rec.Assertion.(provenance.PolicyRecord)
			out = append(out, model.Contribution{Kind: model.ContributionPolicy, Ref: p.Policy, Field: p.Field, Certainty: min(rec.Certainty, c.Certainty)})
		}
	}
	return out
}

// krpRoutes is how krp keeps today's --route selection on the new API: the
// routes Select returns, plus ambiguous routes Known narrowed to one
// matching reading (krp passes --route as Known, as it does today).
func krpRoutes(sel kgateway.Selection, q kgateway.Query) []model.ID {
	out := slices.Clone(sel.Routes)
	for _, n := range sel.Narrowed {
		if len(n.Kept) == 1 && queryMatches(q, n.Kept[0].Assertion.(provenance.RouteSource)) {
			out = append(out, n.Route)
		}
	}
	slices.SortFunc(out, model.CompareID)
	return out
}

func queryMatches(q kgateway.Query, o provenance.RouteSource) bool {
	return o.Route == q.Route && (q.RuleIndex == nil || o.RuleIndex == *q.RuleIndex) && (q.RuleName == "" || o.RuleName == q.RuleName)
}

// TestParity_Selection: for every dump and a set of --route/--rule queries,
// Select gives today's filter.Filter routes (with krp's narrowing rule), and
// its ambiguous routes that krp doesn't keep are today's filter.Ambiguous.
func TestParity_Selection(t *testing.T) {
	ruleIndex := func(i int) *int { return &i }
	type query struct {
		name, ns, rule string
		index          *int
	}
	queries := []query{
		{"api-example-com", "kgateway-system", "", nil},
		{"api-example-com", "kgateway-system", "", ruleIndex(0)},
		{"api-example-com", "kgateway-system", "", ruleIndex(1)},
		{"api-example-com", "kgateway-system", "a-b", nil},
		{"api-example-com", "kgateway-system", "a", nil},
		{"api-example-com-admin", "kgateway-system", "", nil},
		{"orders", "kgwctl-proxy", "", nil},
		{"public", "kgwctl-proxy", "", nil},
		{"does-not-exist", "kgateway-system", "", nil},
	}
	selected, narrowed := 0, 0
	defer func() {
		// Most corpus route names read two ways, so most selections go through Known.
		if selected == 0 || narrowed == 0 {
			t.Errorf("selected %d routes, %d narrowed: the test compares nothing", selected, narrowed)
		}
	}()
	for _, dump := range allDumps(t) {
		for _, qq := range queries {
			ref := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: qq.ns, Name: qq.name}
			// Today's krp: --route is the mapper's Known, then filter.
			m := origin.Kgateway(origin.KgatewayOptions{Known: []model.Ref{ref}})
			old := origin.Annotate(parseFile(t, dump), m)
			opts := filter.Options{HTTPRouteName: qq.name, HTTPRouteNamespace: qq.ns, RuleIndex: filter.AllRules, RuleName: qq.rule}
			if qq.index != nil {
				opts.RuleIndex = *qq.index
			}
			var want []model.ID
			walk(filter.Filter(old, opts), func(r model.Route) { want = append(want, r.ID) })
			wantAmbiguous := filter.Ambiguous(old, opts)

			snap := parseFile(t, dump)
			ix, _ := provenance.Compose(snap, kgateway.New())
			q := kgateway.Query{Route: ref, RuleIndex: qq.index, RuleName: qq.rule, Known: []model.Ref{ref}}
			sel := kgateway.Select(ix, snap, q)
			got := krpRoutes(sel, q)
			if !slices.Equal(got, want) {
				t.Errorf("%s %+v: routes = %v, today %v", dump, qq, got, want)
			}
			selected, narrowed = selected+len(got), narrowed+len(sel.Narrowed)
			var gotAmbiguous []string
			for _, id := range sel.Ambiguous {
				if !slices.Contains(got, id) {
					r, _ := snap.Route(id)
					gotAmbiguous = append(gotAmbiguous, r.Name)
				}
			}
			if !slices.Equal(gotAmbiguous, wantAmbiguous) {
				t.Errorf("%s %+v: ambiguous = %v, today %v", dump, qq, gotAmbiguous, wantAmbiguous)
			}
			if len(sel.Unresolved) > 0 {
				t.Errorf("%s %+v: unresolved %v; the corpus has none", dump, qq, sel.Unresolved)
			}
		}
	}
}

// TestParity_P1Fixes lists every intended difference from today: the Phase 8
// deferred P1 fixes. Each case shows today's answer and the module's.
func TestParity_P1Fixes(t *testing.T) {
	m := origin.Kgateway(origin.KgatewayOptions{})
	old := origin.Annotate(snaptest.P1(), m)
	contribs := attribute(old, chain.Effective(old), m)
	snap := snaptest.P1()
	ix, err := provenance.Compose(snap, kgateway.New())
	if err != nil {
		t.Fatal(err)
	}
	const (
		r0, r1, r2       = "L0/C0/N0/RC/V0/R0", "L0/C0/N0/RC/V0/R1", "L0/C0/N0/RC/V0/R2"
		composite, xform = "L0/C0/N0/F0", "L0/C0/N0/F1"
	)
	web := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"}

	// #2: --rule 0 misses a route whose rule name is api.v1 today; Select finds it.
	var today []model.ID
	walk(filter.Filter(old, filter.Options{HTTPRouteName: "web", HTTPRouteNamespace: "shop", RuleIndex: 0}), func(r model.Route) { today = append(today, r.ID) })
	zero := 0
	sel := kgateway.Select(ix, snap, kgateway.Query{Route: web, RuleIndex: &zero})
	if slices.Contains(today, r0) || !slices.Contains(sel.Routes, r0) {
		t.Errorf("#2: today %v, new %v; want R0 only in new", today, sel.Routes)
	}

	// #5: a Composite running rustformation gets no link today; the module links it.
	if len(contribs[r1][composite]) != 0 || len(slotContributions(ix, r1, composite)) != 1 {
		t.Errorf("#5: today %v, new %v", contribs[r1][composite], slotContributions(ix, r1, composite))
	}

	// #4: a module source that overrides the library name still links today; the module doesn't.
	if len(contribs[r2][xform]) != 1 || len(slotContributions(ix, r2, xform)) != 0 {
		t.Errorf("#4: today %v, new %v", contribs[r2][xform], slotContributions(ix, r2, xform))
	}
}

// walk calls fn for every route in snap.
func walk(snap *model.Snapshot, fn func(model.Route)) {
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					fn(r)
				}
			}
		}
	}
}

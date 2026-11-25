package provenance_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// TestText_RowsPerRoute: one slot under two routes with different per-filter
// config and links gets its own labels on each row; the trailer lists each
// route's records; a narrowing prints the kept reading inline.
func TestText_RowsPerRoute(t *testing.T) {
	snap := twoListeners()
	snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[1].TypedPerFilterConfig = map[string]any{"authz": map[string]any{}}
	ext := model.Ref{Group: "example.dev", Kind: "Extension", Namespace: "ns", Name: "authz"}
	claims := []provenance.Claim{
		{Subject: slot0, Assertion: provenance.ExtensionSource{Extension: &ext}, Certainty: model.Inferred, Evidence: ev(slot0, &ext)},
		record("a", route0, provenance.LevelRoute, policyA),
		record("b", route1, provenance.LevelRoute, policyB),
		record("v", vhost0, provenance.LevelVirtualHost, policyB),
		link("a", route0, slot0),
		link("b", route1, slot0),
		source(route1, routeA, model.Inferred),
		source(route1, routeB, model.Inferred),
	}
	ix := compose(t, snap, fixed{"m", claims})
	narrowed := []provenance.Narrowing{{Route: route1, Kept: provenance.Resolve[provenance.RouteSource](ix, route1).Claims[:1]}}
	var b bytes.Buffer
	opts := render.Options{Labels: provenance.Labels(ix, narrowed), Trailer: provenance.Notes(ix)}
	if err := render.Text(&b, snap, chain.Effective(snap), opts); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"route / · HTTPRoute ns/a · rule 0 · match 0 (chosen from known HTTPRoutes)",
		"1 authz   enabled by route · Extension ns/authz (inferred) · Policy ns/a (f, inferred)",
		"1 authz   enabled by route · Extension ns/authz (inferred) · Policy ns/b (f, inferred)",
		"policies: Policy ns/a (f)",
		"policies: Policy ns/b (f)",
		"inherited policies: Policy ns/b (f, vhost)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
}

// TestText_Formats: an exact reading prints bare, an inferred one says so,
// indices read from a name say where they came from, several readings print
// as ambiguous, and a record's certainty shows unless exact.
func TestText_Formats(t *testing.T) {
	read := func(o provenance.RouteSource, c model.Certainty) provenance.Claim {
		return provenance.Claim{Subject: route1, Assertion: o, Certainty: c, Evidence: ev(route1, &o.Route)}
	}
	cases := []struct {
		claims []provenance.Claim
		want   string
	}{
		{[]provenance.Claim{read(provenance.RouteSource{Route: routeA, RuleIndex: 1, MatchIndex: 0}, model.Exact)},
			"route / · HTTPRoute ns/a · rule 1 · match 0\n"},
		{[]provenance.Claim{read(provenance.RouteSource{Route: routeA, RuleIndex: 1, MatchIndex: 0}, model.Inferred)},
			"route / · HTTPRoute ns/a · rule 1 · match 0 (inferred)\n"},
		{[]provenance.Claim{read(provenance.RouteSource{Route: routeA, RuleIndex: 1, RuleName: "api", MatchIndex: 0, IndexCertainty: model.Inferred}, model.Exact)},
			"route / · HTTPRoute ns/a · rule 1 (api) · match 0 (partly from name)\n"},
		{[]provenance.Claim{read(provenance.RouteSource{Route: routeA, RuleIndex: 0, MatchIndex: 0}, model.Inferred), read(provenance.RouteSource{Route: routeB, RuleIndex: 1, MatchIndex: 0}, model.Inferred)},
			"route / · ambiguous: HTTPRoute ns/a, HTTPRoute ns/b\n"},
	}
	for _, c := range cases {
		snap := twoListeners()
		ix := compose(t, snap, fixed{"m", c.claims})
		var b bytes.Buffer
		if err := render.Text(&b, snap, chain.Effective(snap), render.Options{Labels: provenance.Labels(ix, nil)}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), c.want) {
			t.Errorf("no %q in\n%s", c.want, b.String())
		}
	}
	for _, c := range []model.Certainty{model.Unknown, model.Ambiguous, model.Inferred} {
		rec := record("a", route1, provenance.LevelRoute, policyA)
		rec.Certainty = c
		snap := twoListeners()
		ix := compose(t, snap, fixed{"m", []provenance.Claim{rec}})
		var b bytes.Buffer
		if err := render.Text(&b, snap, chain.Effective(snap), render.Options{Trailer: provenance.Notes(ix)}); err != nil {
			t.Fatal(err)
		}
		if want := "policies: Policy ns/a (f, " + c.String() + ")\n"; !strings.Contains(b.String(), want) {
			t.Errorf("no %q in\n%s", want, b.String())
		}
	}
}

// names is a NameMapper that answers from a table.
type names map[provenance.NameKind]map[string]provenance.NameAnswer

func (n names) Name(k provenance.NameKind, name string) provenance.NameAnswer { return n[k][name] }

// TestGuess: one kind's answer is returned as is; a name that reads as two
// kinds is ambiguous and lists every candidate of each.
func TestGuess(t *testing.T) {
	svc := model.Ref{Kind: "Service", Namespace: "ns", Name: "web"}
	gw := model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "ns", Name: "web"}
	m := names{
		provenance.NameCluster:  {"web": {Certainty: model.Inferred, Candidates: []model.Ref{svc}}, "only": {Certainty: model.Exact, Candidates: []model.Ref{svc}}},
		provenance.NameListener: {"web": {Certainty: model.Inferred, Candidates: []model.Ref{gw}}},
	}
	if got := provenance.Guess(m, "only"); got.Certainty != model.Exact || !slices.Equal(got.Candidates, []model.Ref{svc}) {
		t.Errorf("Guess(only) = %+v", got)
	}
	if got := provenance.Guess(m, "web"); got.Certainty != model.Ambiguous || !slices.Equal(got.Candidates, []model.Ref{svc, gw}) {
		t.Errorf("Guess(web) = %+v, want ambiguous with both", got)
	}
	if got := provenance.Guess(m, "none"); got.Certainty != model.Unknown || got.Candidates != nil {
		t.Errorf("Guess(none) = %+v", got)
	}
}

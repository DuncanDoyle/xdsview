package provenance_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// fixed is a module that returns the claims it holds. It uses exported API only (R15).
type fixed struct {
	name   string
	claims []provenance.Claim
}

func (m fixed) Name() string                              { return m.name }
func (m fixed) Claims(*model.Snapshot) []provenance.Claim { return m.claims }

const (
	authzType = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"
	l0, l1    = model.ID("L0"), model.ID("L1")
	route0    = model.ID("L0/C0/N0/RC/V0/R0") // ext_authz enabled per route
	route1    = model.ID("L0/C0/N0/RC/V0/R1") // no per-filter config
	vhost0    = model.ID("L0/C0/N0/RC/V0")
	slot0     = model.ID("L0/C0/N0/F0") // L0's ext_authz
	other     = model.ID("L1/C0/N0/F0") // L1's ext_authz
	otherR    = model.ID("L1/C0/N0/RC/V0/R0")
)

var (
	policyA = model.Ref{Group: "example.dev", Kind: "Policy", Namespace: "ns", Name: "a"}
	policyB = model.Ref{Group: "example.dev", Kind: "Policy", Namespace: "ns", Name: "b"}
	routeA  = model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "a"}
	routeB  = model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "b"}
)

// twoListeners has listeners L0 and L1, each with ext_authz (off) and a
// router. L0's route R0 enables ext_authz; R1 and L1's route don't.
func twoListeners() *model.Snapshot {
	listener := func(name string, src model.ID, routes ...model.Route) model.Listener {
		hcm := &model.HCMConfig{
			HTTPFilters: []model.HTTPFilter{
				{Name: "authz", Type: authzType, Disabled: true},
				{Name: "router", Type: snaptest.RouterType},
			},
			RouteConfigUse: model.RouteConfigUse{Source: src, State: model.RouteConfigFound},
			RouteConfig:    &model.RouteConfig{Name: name, VirtualHosts: []model.VirtualHost{{Name: "vh", Domains: []string{"*"}, Routes: routes}}},
		}
		return model.Listener{Name: name, State: model.ListenerState{Status: model.ListenerActive},
			FilterChains: []model.NetworkFilterChain{{HCM: hcm}}}
	}
	enabled := map[string]any{"authz": map[string]any{}}
	return snaptest.IDs(&model.Snapshot{
		Listeners: []model.Listener{
			listener("l0", "RD0", model.Route{Name: "r0", Cluster: "c", TypedPerFilterConfig: enabled}, model.Route{Name: "r1", Cluster: "c"}),
			listener("l1", "RD1", model.Route{Name: "r0", Cluster: "c"}),
		},
		RouteConfigs: []model.RouteConfigSource{{ID: "RD0", Kind: model.SourceDynamic, Name: "l0"}, {ID: "RD1", Kind: model.SourceDynamic, Name: "l1"}},
	})
}

func ev(subject model.ID, object *model.Ref) []provenance.Evidence {
	return []provenance.Evidence{{Kind: provenance.EvidenceMetadata, Subject: subject, Field: "test", Object: object}}
}

func record(ref string, subject model.ID, level provenance.Level, p model.Ref) provenance.Claim {
	return provenance.Claim{Ref: ref, Subject: subject, Assertion: provenance.PolicyRecord{Policy: p, Field: "f", Level: level}, Certainty: model.Exact, Evidence: ev(subject, &p)}
}

func link(record string, route, slot model.ID) provenance.Claim {
	return provenance.Claim{Subject: route, Assertion: provenance.FilterLink{Record: record, Slot: slot}, Certainty: model.Inferred,
		Evidence: []provenance.Evidence{{Kind: provenance.EvidenceTable, Subject: slot, Field: "test"}}}
}

func source(subject model.ID, r model.Ref, c model.Certainty) provenance.Claim {
	return provenance.Claim{Subject: subject, Assertion: provenance.RouteSource{Route: r, RuleIndex: 0, MatchIndex: 0}, Certainty: c, Evidence: ev(subject, &r)}
}

func compose(t *testing.T, snap *model.Snapshot, modules ...provenance.Module) *provenance.Index {
	t.Helper()
	ix, err := provenance.Compose(snap, modules...)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func ids(claims []provenance.Claim) []string {
	var out []string
	for _, c := range claims {
		out = append(out, c.ID)
	}
	return out
}

// TestCompose_Rule3 drops a claim with an unknown subject, the wrong subject
// kind, no evidence, an object no evidence names, or a certainty over its
// kind's cap, each with a warning; the rest are kept.
func TestCompose_Rule3(t *testing.T) {
	good := source(route0, routeA, model.Inferred)
	unknown := source("L9/C0/N0/RC/V0/R0", routeA, model.Inferred)
	wrongKind := source(vhost0, routeA, model.Inferred)
	noEvidence := source(route0, routeA, model.Inferred)
	noEvidence.Evidence = nil
	unnamed := source(route0, routeB, model.Inferred)
	unnamed.Evidence = ev(route0, &routeA)
	rec := record("r", route0, provenance.LevelRoute, policyA)
	overCap := link("r", route0, slot0)
	overCap.Certainty = model.Exact
	wrongLevel := record("w", route0, provenance.LevelVirtualHost, policyA)
	ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{good, unknown, wrongKind, noEvidence, unnamed, rec, overCap, wrongLevel}})

	if got, want := ids(ix.Claims()), []string{"m/0", "m/5"}; !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
	var codes []string
	for _, w := range ix.Warnings() {
		codes = append(codes, w.Code+" "+w.Message[strings.Index(w.Message, "): ")+3:])
	}
	want := []string{
		"claim-dropped subject not in the snapshot",
		"claim-dropped subject is the wrong kind of occurrence",
		"claim-dropped no evidence",
		"claim-dropped no evidence names HTTPRoute ns/b",
		"claim-dropped certainty exact exceeds the cap inferred",
		`claim-dropped level "virtualHost" does not match the subject`,
	}
	if !slices.Equal(codes, want) {
		t.Errorf("warnings:\n%s\nwant\n%s", strings.Join(codes, "\n"), strings.Join(want, "\n"))
	}
}

// TestCompose_DuplicateModules fails on a programming error only.
func TestCompose_DuplicateModules(t *testing.T) {
	if _, err := provenance.Compose(twoListeners(), fixed{name: "m"}, fixed{name: "m"}); err == nil {
		t.Error("duplicate module names accepted")
	}
}

// TestCompose_Alternatives: distinct RouteSource claims at equal certainty
// are ambiguous with both candidates; at unequal certainty the higher wins and
// both claims stay listed. Two policy records on one route are both kept.
func TestCompose_Alternatives(t *testing.T) {
	ix := compose(t, twoListeners(),
		fixed{"a", []provenance.Claim{source(route0, routeA, model.Inferred), source(route1, routeA, model.Exact), record("p", route0, provenance.LevelRoute, policyA)}},
		fixed{"b", []provenance.Claim{source(route0, routeB, model.Inferred), source(route1, routeB, model.Inferred), record("p", route0, provenance.LevelRoute, policyB)}})

	r0 := provenance.Resolve[provenance.RouteSource](ix, route0)
	if r0.Certainty != model.Ambiguous || len(r0.Candidates) != 2 || r0.Candidates[0].Route != routeA || r0.Candidates[1].Route != routeB {
		t.Errorf("R0 = %+v, want ambiguous between a and b", r0)
	}
	r1 := provenance.Resolve[provenance.RouteSource](ix, route1)
	if r1.Certainty != model.Exact || len(r1.Candidates) != 1 || r1.Candidates[0].Route != routeA || len(r1.Claims) != 2 {
		t.Errorf("R1 = %+v, want exact a with both claims listed", r1)
	}
	if p := provenance.Resolve[provenance.PolicyRecord](ix, route0); len(p.Candidates) != 2 {
		t.Errorf("policy records = %+v, want both", p.Candidates)
	}
}

// TestCompose_Rule4 resolves filter links: a dangling or rejected record, a
// slot on another HCM, a route with no per-filter config for the slot, a
// record on an unrelated listener, and two links from one record on one route
// are dropped; a link to a vhost-level record is kept.
func TestCompose_Rule4(t *testing.T) {
	rejected := record("rejected", route0, provenance.LevelRoute, policyA)
	rejected.Evidence = nil
	claims := []provenance.Claim{
		record("route", route0, provenance.LevelRoute, policyA),
		record("vhost", vhost0, provenance.LevelVirtualHost, policyA),
		record("twice", route0, provenance.LevelRoute, policyB),
		record("far", otherR, provenance.LevelRoute, policyB),
		rejected,
		link("route", route0, slot0),   // kept
		link("vhost", route0, slot0),   // kept: the vhost is an ancestor
		link("missing", route0, slot0), // dangling
		link("rejected", route0, slot0),
		link("route", route0, other), // another HCM's slot
		link("vhost", route1, slot0), // R1 has no per-filter config
		link("far", route0, slot0),   // record on an unrelated listener
		link("twice", route0, slot0), // two links from one record on one route
		link("twice", route0, slot0),
	}
	ix := compose(t, twoListeners(), fixed{"m", claims})
	var kept []string
	for _, c := range ix.About(route0) {
		if l, ok := c.Assertion.(provenance.FilterLink); ok {
			kept = append(kept, l.Record)
			if rec, ok := ix.Record(c); !ok || rec.Ref != l.Record {
				t.Errorf("Record(%s) = %+v", c.ID, rec)
			}
		}
	}
	if !slices.Equal(kept, []string{"route", "vhost"}) {
		t.Errorf("kept links %v, want route and vhost", kept)
	}
	var dropped int
	for _, w := range ix.Warnings() {
		if w.Code == model.WarnLinkDropped {
			dropped++
		}
	}
	if dropped != 7 {
		t.Errorf("%d links dropped, want 7: %v", dropped, ix.Warnings())
	}
}

// TestCompose_PointerAssertions (Codex #2): a pointer assertion goes through
// the same rules as its value: a dangling *FilterLink is dropped, a valid one
// is kept as a FilterLink value, and a nil pointer is dropped.
func TestCompose_PointerAssertions(t *testing.T) {
	p := policyA
	ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{
		{Ref: "r", Subject: route0, Assertion: &provenance.PolicyRecord{Policy: p, Field: "f", Level: provenance.LevelRoute}, Certainty: model.Exact, Evidence: ev(route0, &p)},
		{Subject: route0, Assertion: &provenance.FilterLink{Record: "missing", Slot: slot0}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
		{Subject: route0, Assertion: &provenance.FilterLink{Record: "r", Slot: other}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
		{Subject: route0, Assertion: &provenance.FilterLink{Record: "r", Slot: slot0}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
		{Subject: route0, Assertion: (*provenance.FilterLink)(nil), Certainty: model.Inferred, Evidence: ev(slot0, nil)},
	}})
	links := provenance.Resolve[provenance.FilterLink](ix, route0)
	if len(links.Candidates) != 1 || links.Candidates[0] != (provenance.FilterLink{Record: "r", Slot: slot0}) {
		t.Errorf("links %+v, want only r on slot0", links.Candidates)
	}
	for _, c := range ix.Claims() {
		if _, ok := c.Assertion.(*provenance.FilterLink); ok {
			t.Errorf("%s kept as a pointer", c.ID)
		}
		if _, ok := c.Assertion.(*provenance.PolicyRecord); ok {
			t.Errorf("%s kept as a pointer", c.ID)
		}
	}
	if len(ix.Warnings()) != 3 {
		t.Errorf("warnings %v, want 3 drops", ix.Warnings())
	}
}

// shadowed embeds a FilterLink and overrides Kind on its pointer, so only
// *shadowed implements Assertion; its value doesn't.
type shadowed struct{ provenance.FilterLink }

func (*shadowed) Kind() string { return "shadowed" }

// lookalike embeds a FilterLink and implements Assertion as a value.
type lookalike struct{ provenance.FilterLink }

// TestCompose_ForeignAssertions (Codex N3): an assertion whose type isn't one
// of the package's kinds is dropped with a warning, as a pointer or a value,
// and Compose never panics.
func TestCompose_ForeignAssertions(t *testing.T) {
	p := policyA
	fl := provenance.FilterLink{Record: "missing", Slot: slot0}
	ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{
		{Ref: "r", Subject: route0, Assertion: provenance.PolicyRecord{Policy: p, Field: "f", Level: provenance.LevelRoute}, Certainty: model.Exact, Evidence: ev(route0, &p)},
		{Subject: route0, Assertion: &shadowed{fl}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
		{Subject: route0, Assertion: &lookalike{fl}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
		{Subject: route0, Assertion: lookalike{fl}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
	}})
	if got := ids(ix.Claims()); !slices.Equal(got, []string{"m/0"}) {
		t.Errorf("kept %v, want only the record", got)
	}
	var reasons []string
	for _, w := range ix.Warnings() {
		reasons = append(reasons, w.Message[strings.Index(w.Message, "): ")+3:])
	}
	if want := []string{"unknown assertion type", "unknown assertion type", "unknown assertion type"}; !slices.Equal(reasons, want) {
		t.Errorf("warnings %v, want %v", reasons, want)
	}
}

// nilLink embeds a nil *FilterLink, so its promoted methods panic.
type nilLink struct{ *provenance.FilterLink }

// TestCompose_ForeignNilEmbed (Codex r3 N3): dropping a foreign assertion
// names its Go type and never calls its methods, in Compose or Dependencies.
func TestCompose_ForeignNilEmbed(t *testing.T) {
	ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{
		{Subject: route0, Assertion: nilLink{}, Certainty: model.Inferred, Evidence: ev(slot0, nil)},
	}})
	if w := ix.Warnings(); len(w) != 1 || !strings.Contains(w[0].Message, "(provenance_test.nilLink on") {
		t.Errorf("warnings %v, want one naming provenance_test.nilLink", w)
	}
	if d := ix.Dependencies(route0); !hasGap(d, route0, "provenance_test.nilLink") {
		t.Errorf("gaps %v lack the dropped claim's type", d.Gaps)
	}
}

// TestCompose_Order: claims are ordered by subject (numerically), then module
// order, then emission order, the same on every run.
func TestCompose_Order(t *testing.T) {
	mods := []provenance.Module{
		fixed{"z", []provenance.Claim{source(otherR, routeA, model.Inferred), source(route1, routeA, model.Inferred), source(route0, routeA, model.Inferred)}},
		fixed{"a", []provenance.Claim{source(route0, routeB, model.Inferred)}},
	}
	first := compose(t, twoListeners(), mods...)
	if got, want := ids(first.Claims()), []string{"z/2", "a/0", "z/1", "z/0"}; !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
	for range 5 {
		if again := compose(t, twoListeners(), mods...); !reflect.DeepEqual(again.Claims(), first.Claims()) {
			t.Fatal("order differs between runs")
		}
	}
}

// TestClaim_UnknownKind (Codex #6): a claim of a kind this version doesn't
// know decodes to an Opaque assertion that writes its fields back unchanged,
// and Compose drops it.
func TestClaim_UnknownKind(t *testing.T) {
	in := `{"id":"m/0","subject":"L0","assertion":{"kind":"futureSource","x":[1,2]},"certainty":"exact","evidence":[],"module":"m"}`
	var c provenance.Claim
	if err := json.Unmarshal([]byte(in), &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	o, ok := c.Assertion.(provenance.Opaque)
	if !ok || o.Kind() != "futureSource" {
		t.Fatalf("assertion = %#v, want Opaque futureSource", c.Assertion)
	}
	out, err := json.Marshal(c)
	if err != nil || !strings.Contains(string(out), `"assertion":{"kind":"futureSource","x":[1,2]}`) {
		t.Errorf("re-encoded %s (%v)", out, err)
	}
	c.Evidence = ev(l0, nil)
	if ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{c}}); len(ix.Claims()) != 0 {
		t.Errorf("Compose kept an opaque claim: %+v", ix.Claims())
	}
}

// TestClaim_JSON: an assertion encodes as an object with its kind, and a
// claim decodes back to the same value.
func TestClaim_JSON(t *testing.T) {
	ix := compose(t, twoListeners(), fixed{"m", []provenance.Claim{source(route0, routeA, model.Inferred), record("r", route0, provenance.LevelRoute, policyA), link("r", route0, slot0)}})
	for _, c := range ix.Claims() {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		var head struct {
			Assertion struct {
				Kind string `json:"kind"`
			} `json:"assertion"`
		}
		if json.Unmarshal(b, &head) != nil || head.Assertion.Kind != c.Assertion.Kind() {
			t.Errorf("%s: no kind in %s", c.ID, b)
		}
		var back provenance.Claim
		if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, c) {
			t.Errorf("%s: round trip = %+v (%v), want %+v", c.ID, back, err, c)
		}
	}
}

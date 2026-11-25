package provenance

import (
	"reflect"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Index holds the claims [Compose] kept, ordered by subject ([model.CompareID]),
// then module order, then emission order. It is read-only and safe for
// concurrent use.
type Index struct {
	snap      *model.Snapshot
	st        *structure
	claims    []Claim
	bySubject map[model.ID][]int
	byRef     map[[2]string]int
	dropped   []Claim
	warnings  []model.Warning
}

// Claims returns every claim in the index, in index order.
func (ix *Index) Claims() []Claim { return slices.Clone(ix.claims) }

// About returns the claims whose subject is id, in index order.
func (ix *Index) About(id model.ID) []Claim {
	var out []Claim
	for _, i := range ix.bySubject[id] {
		out = append(out, ix.claims[i])
	}
	return out
}

// Warnings returns why claims were dropped, in the order Compose dropped them.
func (ix *Index) Warnings() []model.Warning { return slices.Clone(ix.warnings) }

// Record returns the policy record claim a [FilterLink] claim names.
func (ix *Index) Record(link Claim) (Claim, bool) {
	l, ok := link.Assertion.(FilterLink)
	if !ok {
		return Claim{}, false
	}
	i, ok := ix.byRef[[2]string{link.Module, l.Record}]
	if !ok {
		return Claim{}, false
	}
	return ix.claims[i], true
}

// Resolution is what the index says about one kind of assertion on one subject.
//
// For an alternatives kind, Candidates are the distinct assertions at the
// highest certainty, in claim order; Certainty is that certainty, or
// [model.Ambiguous] when they disagree. For a cumulative kind ([PolicyRecord],
// [FilterLink]) every assertion is a candidate and Certainty is the highest.
// Claims lists every claim of the kind, the outranked ones too.
type Resolution[T Assertion] struct {
	Certainty  model.Certainty
	Candidates []T
	Claims     []Claim
}

// Resolve resolves the assertions of kind T about id.
func Resolve[T Assertion](ix *Index, id model.ID) Resolution[T] {
	var res Resolution[T]
	for _, c := range ix.About(id) {
		if _, ok := c.Assertion.(T); ok {
			res.Claims = append(res.Claims, c)
			res.Certainty = max(res.Certainty, c.Certainty)
		}
	}
	if len(res.Claims) == 0 {
		return res
	}
	var zero T
	cumulative := any(zero) != nil && zero.spec().cumulative // T may be Assertion itself
	for _, c := range res.Claims {
		a := c.Assertion.(T)
		if cumulative {
			res.Candidates = append(res.Candidates, a)
			continue
		}
		if c.Certainty == res.Certainty && !slices.ContainsFunc(res.Candidates, func(b T) bool { return reflect.DeepEqual(a, b) }) {
			res.Candidates = append(res.Candidates, a)
		}
	}
	if !cumulative && len(res.Candidates) > 1 {
		res.Certainty = model.Ambiguous
	}
	return res
}

// Dependencies is the evidence scope of a block: what its view rests on.
type Dependencies struct {
	Objects []Dependent // every object the block's claims rest on, with the claim kind, sorted
	Gaps    []Gap       // expected attribution that is missing or ambiguous, sorted
}

// Dependent is one object a block's claims rest on.
type Dependent struct {
	Object model.Ref
	Kind   string // the claim's assertion kind, e.g. "backendSource"
}

// Gap is an expected attribution that is missing or ambiguous. A policyRecord
// gap also names the filter slot and the level its record is missing for.
type Gap struct {
	Subject model.ID
	Kind    string   // the assertion kind expected, e.g. "routeSource"
	Slot    model.ID // policyRecord only: the HTTP filter slot
	Level   Level    // policyRecord only: the level the record is missing at
}

// Dependencies returns the evidence scope of block (any occurrence ID). Its
// claims are those on:
//  1. the block and everything in it;
//  2. its ancestors up to the listener, and the route config source of every
//     use in or above it, a source shared with other HCMs included;
//  3. the HTTP filter slots of the HCM that serves it, or for a route config
//     source, of every HCM whose use resolves to it.
//
// Gaps, per expected kind:
//   - routeSource: a route that doesn't resolve to one reading at inferred or exact;
//   - backendSource: a route or weighted backend with a cluster name and no
//     such BackendSource;
//   - extensionSource: an HTTP filter slot with per-filter config on any route
//     of its HCM and no such ExtensionSource;
//   - policyRecord: per-filter config for a slot at route, virtual host or
//     route config level, unless for each route checked a kept [FilterLink]
//     on that route places a record of that level on the slot, link and
//     record both at inferred or exact; and an HTTP filter slot that runs by
//     default and is not the router or a helper, unless for each route checked
//     such a link on that route places a listener-level record on the slot.
//     Every link and record that closes such a gap joins Objects;
//   - gatewaySource: a listener with no GatewaySource above unknown;
//   - a dropped claim's kind on its subject (Compose rule 3).
func (ix *Index) Dependencies(block model.ID) Dependencies {
	scope := ix.scope(block)
	var deps Dependencies
	gaps, claims := ix.gaps(scope)
	for _, c := range ix.claims {
		if scope[c.Subject] {
			claims = append(claims, c)
		}
	}
	for _, c := range claims {
		for _, e := range c.Evidence {
			if e.Object == nil {
				continue
			}
			d := Dependent{Object: *e.Object, Kind: c.Assertion.Kind()}
			if !slices.Contains(deps.Objects, d) {
				deps.Objects = append(deps.Objects, d)
			}
		}
	}
	slices.SortFunc(deps.Objects, func(a, b Dependent) int {
		return strings.Compare(refKey(a.Object)+"\x00"+a.Kind, refKey(b.Object)+"\x00"+b.Kind)
	})
	deps.Gaps = gaps
	return deps
}

func refKey(r model.Ref) string {
	return r.Group + "\x00" + r.Kind + "\x00" + r.Namespace + "\x00" + r.Name
}

// scope returns the subjects whose claims count for block (rules 1–3 above).
func (ix *Index) scope(block model.ID) map[model.ID]bool {
	st := ix.st
	out := map[model.ID]bool{}
	for id := range st.kinds {
		if id.Within(block) || block.Within(id) {
			out[id] = true
		}
	}
	for use, src := range st.useSource {
		if use.Within(block) || block.Within(use) {
			out[src] = true
		}
		if src.Within(block) || block.Within(src) {
			for _, s := range st.slots[st.hcmOf(use)] {
				out[s] = true
			}
		}
	}
	if hcm := st.hcmOf(block); hcm != "" {
		for _, s := range st.slots[hcm] {
			out[s] = true
		}
	}
	return out
}

// gaps returns the expected attributions missing in scope, and the links and
// records that close a policyRecord gap, whose objects the block depends on.
func (ix *Index) gaps(scope map[model.ID]bool) ([]Gap, []Claim) {
	var out []Gap
	addGap := func(g Gap) {
		if !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	add := func(id model.ID, kind string) { addGap(Gap{Subject: id, Kind: kind}) }
	has := func(id model.ID, kind string, min model.Certainty) bool {
		for _, c := range ix.About(id) {
			if c.Assertion.Kind() == kind && c.Certainty >= min {
				return true
			}
		}
		return false
	}
	resolved := func(id model.ID, kind string) bool {
		switch kind {
		case KindRouteSource:
			return Resolve[RouteSource](ix, id).Certainty >= model.Inferred
		case KindBackendSource:
			return Resolve[BackendSource](ix, id).Certainty >= model.Inferred
		case KindExtensionSource:
			return Resolve[ExtensionSource](ix, id).Certainty >= model.Inferred
		}
		return has(id, kind, model.Inferred)
	}
	configured := map[model.ID]bool{} // slots with per-filter config on any route
	for route, c := range ix.st.chains {
		if ix.st.kinds[route] != subjectRoute {
			continue
		}
		for _, e := range c.Filters {
			if ix.st.configured(route, e.Slot) {
				configured[e.Slot] = true
			}
		}
	}
	// placed holds, per (route, record subject, slot), the kept link and its
	// record, both at inferred or exact.
	placed := map[[3]model.ID][]Claim{}
	for _, c := range ix.claims {
		if l, ok := c.Assertion.(FilterLink); ok && c.Certainty >= model.Inferred {
			if rec, ok := ix.Record(c); ok && rec.Certainty >= model.Inferred {
				k := [3]model.ID{c.Subject, rec.Subject, l.Slot}
				placed[k] = append(placed[k], c, rec)
			}
		}
	}
	var closers []Claim
	// placing returns the links and records placing a record on `at` onto slot
	// for every in-scope route of routes (all of them when none is in scope),
	// or false when a checked route has none.
	placing := func(at, slot model.ID, routes []model.ID) ([]Claim, bool) {
		checked := slices.DeleteFunc(slices.Clone(routes), func(r model.ID) bool { return !scope[r] })
		if len(checked) == 0 {
			checked = routes
		}
		var out []Claim
		for _, r := range checked {
			c := placed[[3]model.ID{r, at, slot}]
			if len(c) == 0 {
				return nil, false
			}
			out = append(out, c...)
		}
		return out, len(checked) > 0
	}
	// unplaced checks config at `at` against the routes beneath it.
	unplaced := func(at model.ID, level Level, pfc map[string]any, hcm *model.HCMConfig, routes []model.ID) {
		for _, f := range hcm.HTTPFilters {
			if _, ok := pfc[f.Name]; !ok || !scope[at] || !scope[f.ID] {
				continue
			}
			if c, ok := placing(at, f.ID, routes); ok {
				closers = append(closers, c...)
			} else {
				addGap(Gap{Subject: at, Kind: KindPolicyRecord, Slot: f.ID, Level: level})
			}
		}
	}
	sources := map[model.ID]model.RouteConfigSource{}
	for _, src := range ix.snap.RouteConfigs {
		sources[src.ID] = src
	}
	for _, l := range ix.snap.Listeners {
		if scope[l.ID] && !has(l.ID, KindGatewaySource, model.Unknown+1) {
			add(l.ID, KindGatewaySource)
		}
		for _, fc := range l.FilterChains {
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			for _, f := range hcm.HTTPFilters {
				if !scope[f.ID] {
					continue
				}
				if configured[f.ID] && !resolved(f.ID, KindExtensionSource) {
					add(f.ID, KindExtensionSource)
				}
				if !f.Disabled && !isRouter(f) && !ix.helper(f.ID) {
					if c, ok := placing(l.ID, f.ID, routeIDs(hcm.RouteConfig, nil)); ok {
						closers = append(closers, c...)
					} else {
						addGap(Gap{Subject: f.ID, Kind: KindPolicyRecord, Slot: f.ID, Level: LevelListener})
					}
				}
			}
			if src, ok := sources[hcm.RouteConfigUse.Source]; ok {
				unplaced(src.ID, LevelRouteConfig, src.TypedPerFilterConfig, hcm, routeIDs(hcm.RouteConfig, nil))
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for _, vh := range hcm.RouteConfig.VirtualHosts {
				unplaced(vh.ID, LevelVirtualHost, vh.TypedPerFilterConfig, hcm, routeIDs(nil, &vh))
				for _, r := range vh.Routes {
					unplaced(r.ID, LevelRoute, r.TypedPerFilterConfig, hcm, []model.ID{r.ID})
					if scope[r.ID] {
						if !resolved(r.ID, KindRouteSource) {
							add(r.ID, KindRouteSource)
						}
						if r.Cluster != "" && !resolved(r.ID, KindBackendSource) {
							add(r.ID, KindBackendSource)
						}
					}
					for _, w := range r.WeightedClusters {
						if scope[w.ID] && w.Name != "" && !resolved(w.ID, KindBackendSource) {
							add(w.ID, KindBackendSource)
						}
					}
				}
			}
		}
	}
	for _, c := range ix.dropped {
		if scope[c.Subject] && c.Assertion != nil {
			add(c.Subject, kindOf(c.Assertion))
		}
	}
	slices.SortFunc(out, func(a, b Gap) int {
		if c := model.CompareID(a.Subject, b.Subject); c != 0 {
			return c
		}
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if c := model.CompareID(a.Slot, b.Slot); c != 0 {
			return c
		}
		return strings.Compare(string(a.Level), string(b.Level))
	})
	return out, closers
}

// routeIDs returns the IDs of vh's routes, or of every route in rc when vh is nil.
func routeIDs(rc *model.RouteConfig, vh *model.VirtualHost) []model.ID {
	var vhs []model.VirtualHost
	switch {
	case vh != nil:
		vhs = []model.VirtualHost{*vh}
	case rc != nil:
		vhs = rc.VirtualHosts
	}
	var out []model.ID
	for _, v := range vhs {
		for _, r := range v.Routes {
			out = append(out, r.ID)
		}
	}
	return out
}

// helper reports whether an ExtensionSource marks slot as a helper.
func (ix *Index) helper(slot model.ID) bool {
	for _, e := range Resolve[ExtensionSource](ix, slot).Candidates {
		if e.Helper {
			return true
		}
	}
	return false
}

// isRouter reports whether f is Envoy's router filter.
func isRouter(f model.HTTPFilter) bool {
	return strings.HasSuffix(f.Type, ".filters.http.router.v3.Router")
}

// Records returns the [PolicyRecord] claims that apply to a route: on the
// route, its virtual host, the route config source its use resolves to, and
// its listener, most specific first. Each record's Level says where it sits.
func Records(ix *Index, snap *model.Snapshot, route model.ID) []Claim {
	var subjects []model.ID
	if r, ok := snap.Route(route); ok {
		subjects = append(subjects, r.ID)
	}
	if hcm, ok := snap.HCM(route); ok && hcm.RouteConfig != nil {
		for _, vh := range hcm.RouteConfig.VirtualHosts {
			if route.Within(vh.ID) {
				subjects = append(subjects, vh.ID)
			}
		}
		if src := hcm.RouteConfigUse.Source; src != "" {
			subjects = append(subjects, src)
		}
	}
	for _, l := range snap.Listeners {
		if route.Within(l.ID) {
			subjects = append(subjects, l.ID)
		}
	}
	var out []Claim
	for _, s := range subjects {
		for _, c := range ix.About(s) {
			if _, ok := c.Assertion.(PolicyRecord); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

// Narrowing records how a consumer's list of known objects narrowed a
// route's readings: the [RouteSource] claims it kept and those it removed.
type Narrowing struct {
	Route   model.ID
	Kept    []Claim
	Dropped []Claim
}

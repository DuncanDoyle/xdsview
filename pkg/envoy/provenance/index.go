package provenance

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

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

	depOnce sync.Once
	dep     *depIndex // built on the first Dependencies call
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

// Gap is an input of a block that no claim covers. A policyRecord gap also
// names the filter slot and the level its record is missing at.
type Gap struct {
	Subject model.ID
	Kind    string   // the assertion kind expected, e.g. "routeSource"
	Slot    model.ID // policyRecord only: the HTTP filter slot, empty when none is installed
	Level   Level    // policyRecord only: the level the record is missing at, empty for a weighted backend
}

// Dependencies returns the evidence scope of block (any occurrence ID). It is
// fail-closed (design §5): it enumerates every input the block shows or
// relies on, and each input no claim covers is a [Gap].
//
// The inputs are:
//   - each listener, virtual host, route and weighted backend in or above
//     the block;
//   - each HTTP filter slot, downstream and router upstream, of every HCM
//     serving the block: the HCM in or above it, or for a route config
//     source, each HCM whose use resolves to it;
//   - each per-filter config entry on the route config source of a use in or
//     above the block (or the source block itself), and on each virtual
//     host, route and weighted backend in or above it, including entries no
//     installed slot takes. A source no HCM uses is a block of its entries.
//     A static source that an inline source reports counts as that source.
//
// An input is covered only by claims at inferred or exact:
//   - listener: GatewaySource; virtual host: HostSource; route: RouteSource;
//     route or weighted backend with a cluster: BackendSource;
//   - a slot other than the router: ExtensionSource. A downstream slot that
//     runs by default and is no helper also needs, for each route checked, a
//     kept FilterLink on that route placing a listener-level PolicyRecord on it;
//   - a per-filter config entry at level X for slot S: for each route
//     checked, a kept FilterLink on that route placing a level-X PolicyRecord
//     on S. An entry on a weighted backend, or for no installed downstream
//     slot, has no cover.
//
// The routes checked are the block's routes beneath the input, or all of
// them when the block holds none. A dropped claim's kind on a subject in
// scope is also a gap (Compose rule 3).
//
// Objects holds the evidence objects of the claims on every subject above,
// and of every claim that covers an input, wherever its subject lives.
func (ix *Index) Dependencies(block model.ID) Dependencies {
	scope, inputs := ix.inputs(block)
	objects := map[Dependent]bool{}
	add := func(c Claim) {
		for _, e := range c.Evidence {
			if e.Object != nil {
				objects[Dependent{Object: *e.Object, Kind: c.Assertion.Kind()}] = true
			}
		}
	}
	for id := range scope {
		for _, i := range ix.bySubject[id] {
			add(ix.claims[i])
		}
	}
	gaps := map[Gap]bool{}
	for _, in := range inputs {
		if !in.covered {
			gaps[in.gap] = true
			continue
		}
		for _, i := range in.covers {
			add(ix.claims[i])
		}
	}
	for _, c := range ix.dropped {
		if scope[c.Subject] && c.Assertion != nil {
			gaps[Gap{Subject: c.Subject, Kind: kindOf(c.Assertion)}] = true
		}
	}
	var deps Dependencies
	deps.Objects = slices.SortedFunc(maps.Keys(objects), func(a, b Dependent) int {
		return strings.Compare(refKey(a.Object)+"\x00"+a.Kind, refKey(b.Object)+"\x00"+b.Kind)
	})
	deps.Gaps = slices.SortedFunc(maps.Keys(gaps), func(a, b Gap) int {
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
	return deps
}

func refKey(r model.Ref) string {
	return r.Group + "\x00" + r.Kind + "\x00" + r.Namespace + "\x00" + r.Name
}

// input is one thing a block shows or relies on, and the claims covering it.
type input struct {
	gap     Gap   // reported when not covered
	covered bool  // claims at inferred or exact cover it
	covers  []int // indices of the claims that can cover it
}

// depIndex is the snapshot's structure as Dependencies reads it.
type depIndex struct {
	parent   map[model.ID]model.ID
	children map[model.ID][]model.ID
	hcms     map[model.ID]*hcmInfo
	owner    map[model.ID]*hcmInfo   // virtual host, route or weighted backend → its HCM
	users    map[model.ID][]*hcmInfo // route config source → the HCMs whose use resolves to it
	reports  map[model.ID][]model.ID // static source → the inline sources that report it
	sources  map[model.ID]*model.RouteConfigSource
	vhosts   map[model.ID]*model.VirtualHost
	routes   map[model.ID]*model.Route
	backends map[model.ID]*model.WeightedCluster
	placed   map[[3]model.ID][]int // (route, record subject, slot) → kept link and record, both at inferred or exact
}

// hcmInfo is one HCM, its listener, its routes and its slots by filter name.
type hcmInfo struct {
	hcm        *model.HCMConfig
	listener   model.ID
	routes     []model.ID
	slots      map[string][]model.ID
	upstreamBy map[string]model.ID
}

// deps returns the index's depIndex, building it once.
func (ix *Index) deps() *depIndex {
	ix.depOnce.Do(func() { ix.dep = newDepIndex(ix) })
	return ix.dep
}

func newDepIndex(ix *Index) *depIndex {
	d := &depIndex{parent: map[model.ID]model.ID{}, children: map[model.ID][]model.ID{}, hcms: map[model.ID]*hcmInfo{},
		owner: map[model.ID]*hcmInfo{}, users: map[model.ID][]*hcmInfo{}, reports: map[model.ID][]model.ID{},
		sources: map[model.ID]*model.RouteConfigSource{}, vhosts: map[model.ID]*model.VirtualHost{}, routes: map[model.ID]*model.Route{},
		backends: map[model.ID]*model.WeightedCluster{}, placed: map[[3]model.ID][]int{}}
	tie := func(parent, child model.ID) {
		d.parent[child] = parent
		d.children[parent] = append(d.children[parent], child)
	}
	for i := range ix.snap.RouteConfigs {
		s := &ix.snap.RouteConfigs[i]
		d.sources[s.ID] = s
		if s.Reported != "" {
			d.reports[s.Reported] = append(d.reports[s.Reported], s.ID)
		}
	}
	for li := range ix.snap.Listeners {
		l := &ix.snap.Listeners[li]
		for fi := range l.FilterChains {
			fc := &l.FilterChains[fi]
			tie(l.ID, fc.ID)
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			h := &hcmInfo{hcm: hcm, listener: l.ID, slots: map[string][]model.ID{}, upstreamBy: map[string]model.ID{}}
			d.hcms[hcm.ID] = h
			tie(fc.ID, hcm.ID)
			for _, f := range hcm.HTTPFilters {
				tie(hcm.ID, f.ID)
				h.slots[f.Name] = append(h.slots[f.Name], f.ID)
			}
			for _, f := range hcm.UpstreamFilters {
				tie(hcm.ID, f.ID)
				if _, ok := h.upstreamBy[f.Name]; !ok {
					h.upstreamBy[f.Name] = f.ID
				}
			}
			use := hcm.RouteConfigUse.ID
			tie(hcm.ID, use)
			if src := hcm.RouteConfigUse.Source; src != "" {
				d.users[src] = append(d.users[src], h)
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for vi := range hcm.RouteConfig.VirtualHosts {
				vh := &hcm.RouteConfig.VirtualHosts[vi]
				d.vhosts[vh.ID], d.owner[vh.ID] = vh, h
				tie(use, vh.ID)
				for ri := range vh.Routes {
					r := &vh.Routes[ri]
					d.routes[r.ID], d.owner[r.ID] = r, h
					h.routes = append(h.routes, r.ID)
					tie(vh.ID, r.ID)
					for wi := range r.WeightedClusters {
						w := &r.WeightedClusters[wi]
						d.backends[w.ID], d.owner[w.ID] = w, h
						tie(r.ID, w.ID)
					}
				}
			}
		}
	}
	for i, c := range ix.claims {
		if l, ok := c.Assertion.(FilterLink); ok && c.Certainty >= model.Inferred {
			if j, ok := ix.byRef[[2]string{c.Module, l.Record}]; ok && ix.claims[j].Certainty >= model.Inferred {
				k := [3]model.ID{c.Subject, ix.claims[j].Subject, l.Slot}
				d.placed[k] = append(d.placed[k], i, j)
			}
		}
	}
	return d
}

// lineage returns block, its ancestors and its descendants in the listener
// tree. A block outside the tree (a source, say) is matched by ID.
func (ix *Index) lineage(block model.ID) map[model.ID]bool {
	d := ix.deps()
	out := map[model.ID]bool{}
	if k := ix.st.kinds[block]; k != 0 && k != subjectSource {
		for id, ok := block, true; ok; id, ok = d.parent[id] {
			out[id] = true
		}
		var walk func(model.ID)
		walk = func(id model.ID) {
			for _, c := range d.children[id] {
				out[c] = true
				walk(c)
			}
		}
		walk(block)
		return out
	}
	for id, k := range ix.st.kinds {
		if k != subjectSource && (id.Within(block) || block.Within(id)) {
			out[id] = true
		}
	}
	return out
}

// inputs enumerates block's inputs (see Dependencies), and returns the
// subjects whose claims the block rests on.
func (ix *Index) inputs(block model.ID) (map[model.ID]bool, []input) {
	d := ix.deps()
	in := ix.lineage(block)
	scope := maps.Clone(in)
	var out []input
	srcIn := map[*hcmInfo]bool{} // the HCMs serving the block; true when its source is in scope
	inRoutes := map[model.ID][]model.ID{}
	for id := range in {
		switch ix.st.kinds[id] {
		case subjectListener:
			out = append(out, ix.alternative(id, KindGatewaySource))
		case subjectHCM:
			h := d.hcms[id]
			srcIn[h] = srcIn[h] || in[h.hcm.RouteConfigUse.ID]
		case subjectUse:
			if src := ix.st.useSource[id]; src != "" {
				scope[src] = true
			}
		case subjectVirtualHost:
			out = append(out, ix.alternative(id, KindHostSource))
		case subjectRoute:
			out = append(out, ix.alternative(id, KindRouteSource))
			if d.routes[id].Cluster != "" {
				out = append(out, ix.alternative(id, KindBackendSource))
			}
			inRoutes[d.owner[id].hcm.ID] = append(inRoutes[d.owner[id].hcm.ID], id)
			inRoutes[d.parent[id]] = append(inRoutes[d.parent[id]], id)
		case subjectBackend:
			if d.backends[id].Name != "" {
				out = append(out, ix.alternative(id, KindBackendSource))
			}
		}
	}
	// checked returns the block's routes under parent, or all of them when it holds none.
	checked := func(parent model.ID, all []model.ID) []model.ID {
		if r := inRoutes[parent]; len(r) > 0 {
			return r
		}
		return all
	}
	if ix.st.kinds[block] == subjectSource {
		used := false
		for _, t := range append([]model.ID{block}, d.reports[block]...) {
			scope[t] = true
			for _, h := range d.users[t] {
				srcIn[h], used = true, true
			}
		}
		if !used {
			for range d.sources[block].TypedPerFilterConfig {
				out = append(out, input{gap: Gap{Subject: block, Kind: KindPolicyRecord, Level: LevelRouteConfig}})
			}
		}
	}
	// entries enumerates the per-filter config at `at` for h's slots.
	entries := func(h *hcmInfo, at model.ID, level Level, pfc map[string]any, routes []model.ID) {
		for name := range pfc {
			slots := h.slots[name]
			if len(slots) == 0 {
				out = append(out, input{gap: Gap{Subject: at, Kind: KindPolicyRecord, Slot: h.upstreamBy[name], Level: level}})
			}
			for _, s := range slots {
				out = append(out, ix.placement(Gap{Subject: at, Kind: KindPolicyRecord, Slot: s, Level: level}, at, s, routes))
			}
		}
	}
	for h, src := range srcIn {
		hcm := h.hcm
		all := checked(hcm.ID, h.routes)
		for _, f := range hcm.HTTPFilters {
			scope[f.ID] = true
			if isRouter(f) {
				continue
			}
			out = append(out, ix.alternative(f.ID, KindExtensionSource))
			if !f.Disabled && !ix.helper(f.ID) {
				out = append(out, ix.placement(Gap{Subject: f.ID, Kind: KindPolicyRecord, Slot: f.ID, Level: LevelListener}, h.listener, f.ID, all))
			}
		}
		for _, f := range hcm.UpstreamFilters {
			scope[f.ID] = true
			if !isRouter(f) {
				out = append(out, ix.alternative(f.ID, KindExtensionSource))
			}
		}
		if s := d.sources[hcm.RouteConfigUse.Source]; src && s != nil {
			entries(h, s.ID, LevelRouteConfig, s.TypedPerFilterConfig, all)
		}
	}
	for id := range in {
		switch ix.st.kinds[id] {
		case subjectVirtualHost:
			entries(d.owner[id], id, LevelVirtualHost, d.vhosts[id].TypedPerFilterConfig, checked(id, d.children[id]))
		case subjectRoute:
			entries(d.owner[id], id, LevelRoute, d.routes[id].TypedPerFilterConfig, []model.ID{id})
		case subjectBackend:
			h := d.owner[id]
			for name := range d.backends[id].TypedPerFilterConfig {
				slot := h.upstreamBy[name]
				if s := h.slots[name]; len(s) > 0 {
					slot = s[0]
				}
				out = append(out, input{gap: Gap{Subject: id, Kind: KindPolicyRecord, Slot: slot}})
			}
		}
	}
	return scope, out
}

// alternative is the input on id that an alternatives kind covers.
func (ix *Index) alternative(id model.ID, kind string) input {
	var c model.Certainty
	switch kind {
	case KindGatewaySource:
		c = Resolve[GatewaySource](ix, id).Certainty
	case KindHostSource:
		c = Resolve[HostSource](ix, id).Certainty
	case KindRouteSource:
		c = Resolve[RouteSource](ix, id).Certainty
	case KindExtensionSource:
		c = Resolve[ExtensionSource](ix, id).Certainty
	case KindBackendSource:
		c = Resolve[BackendSource](ix, id).Certainty
	}
	in := input{gap: Gap{Subject: id, Kind: kind}, covered: c >= model.Inferred}
	for _, i := range ix.bySubject[id] {
		if ix.claims[i].Assertion.Kind() == kind && ix.claims[i].Certainty >= model.Inferred {
			in.covers = append(in.covers, i)
		}
	}
	return in
}

// placement is the input that links on each of routes cover by placing a
// record on `at` onto slot. With no routes, nothing covers it.
func (ix *Index) placement(gap Gap, at, slot model.ID, routes []model.ID) input {
	d := ix.deps()
	in := input{gap: gap, covered: len(routes) > 0}
	for _, r := range routes {
		c := d.placed[[3]model.ID{r, at, slot}]
		if len(c) == 0 {
			in.covered = false
		}
		in.covers = append(in.covers, c...)
	}
	return in
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

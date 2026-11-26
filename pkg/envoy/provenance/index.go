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

// Resolve resolves the assertions of kind T about id. For [BackendSource] it
// resolves the subject's own cluster; see [ResolveMirror] for mirrors.
func Resolve[T Assertion](ix *Index, id model.ID) Resolution[T] {
	return resolve[T](ix, id, "")
}

// ResolveMirror resolves the [BackendSource] claims about a route's mirror
// cluster. Each mirror cluster's claims compete only with each other.
func ResolveMirror(ix *Index, route model.ID, cluster string) Resolution[BackendSource] {
	return resolve[BackendSource](ix, route, cluster)
}

// resolve resolves the assertions of kind T about id, keeping only
// BackendSource claims for the given mirror cluster.
func resolve[T Assertion](ix *Index, id model.ID, mirror string) Resolution[T] {
	var res Resolution[T]
	for _, c := range ix.About(id) {
		if b, ok := c.Assertion.(BackendSource); ok && b.Mirror != mirror {
			continue
		}
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
	Cluster string   // backendSource only: the mirror cluster, empty for the route's own backend
}

// Input is one thing a block shows or relies on (see [Index.Dependencies]),
// and whether claims cover it. Gap names it, and is what Dependencies reports
// when Status is [InputGap].
type Input struct {
	Gap
	Status InputStatus
	Reason GapReason // gap only
	// Candidates is set for a routeSource input: every RouteSource claim on
	// the route, at any certainty. Cluster state is never consulted.
	Candidates []Candidate
	// Complete is set for a routeSource input whose readings at the highest
	// certainty name every possible source: one exact reading, or inferred
	// readings none marks Incomplete. An input that isn't complete is a gap.
	Complete bool
}

// InputStatus says whether an input is covered.
type InputStatus string

// Input statuses.
const (
	InputCovered   InputStatus = "covered"   // claims at inferred or exact cover it
	InputGap       InputStatus = "gap"       // no claim covers it
	InputBootstrap InputStatus = "bootstrap" // from the proxy's startup config: nothing to attribute
)

// GapReason says why an input is a gap.
type GapReason string

// Gap reasons.
const (
	ReasonAmbiguous    GapReason = "ambiguous"    // several candidates at the highest certainty
	ReasonNoEvidence   GapReason = "noEvidence"   // no claim at inferred or better
	ReasonDropped      GapReason = "dropped"      // Compose dropped a claim of this kind on the subject
	ReasonIncomplete   GapReason = "incomplete"   // the route's readings may miss its source
	ReasonNotCoverable GapReason = "notCoverable" // no claim kind can cover it, e.g. a network filter
)

// Candidate is one reading of a route: the HTTPRoute it names and the claim,
// with its evidence.
type Candidate struct {
	Ref   model.Ref
	Claim Claim
}

// Inputs returns every input of block (any occurrence ID), sorted by subject,
// kind, slot, level and cluster. [Index.Dependencies] derives from it: its
// gaps are the inputs with status [InputGap].
func (ix *Index) Inputs(block model.ID) []Input {
	_, inputs := ix.inputs(block)
	out := make([]Input, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, in.Input)
	}
	slices.SortFunc(out, func(a, b Input) int { return compareGap(a.Gap, b.Gap) })
	return out
}

// Dependencies returns the evidence scope of block (any occurrence ID). It is
// fail-closed (design §5): it enumerates every input the block shows or
// relies on, and each input no claim covers is a [Gap].
//
// The inputs are:
//   - each listener, virtual host, route and weighted backend in or above
//     the block, and each mirror cluster of such a route;
//   - each network filter other than the HCM of every filter chain in or
//     above the block;
//   - each HTTP filter slot, downstream and router upstream, of every HCM
//     serving the block: the HCM in or above it, or for a route config
//     source, each HCM whose use resolves to it;
//   - each per-filter config entry on the route config source of a use in or
//     above the block (or the source block itself), and on each virtual
//     host, route and weighted backend in or above it, including entries no
//     installed slot takes. A source no HCM uses is a block of its entries.
//     A static source that an inline source reports counts as that source,
//     the inline source's listener lineage included.
//
// An input is covered only by claims at inferred or exact:
//   - listener: GatewaySource; virtual host: HostSource; route: RouteSource;
//     route or weighted backend with a cluster: BackendSource; mirror
//     cluster: BackendSource for that mirror ([ResolveMirror]);
//   - a route whose cluster is picked per request (header or specifier
//     plugin), or a network filter other than the HCM: nothing;
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
//
// A bootstrap subject comes from the proxy's startup config: a static listener,
// anything in it, or a static source only its inline sources report. Route
// config content a static listener fetches, e.g. over RDS, is not bootstrap.
// A bootstrap subject is never a gap and adds no objects.
func (ix *Index) Dependencies(block model.ID) Dependencies {
	scope, inputs := ix.inputs(block)
	d := ix.deps()
	objects := map[Dependent]bool{}
	add := func(c Claim) {
		for _, e := range c.Evidence {
			if e.Object != nil {
				objects[Dependent{Object: *e.Object, Kind: c.Assertion.Kind()}] = true
			}
		}
	}
	for id := range scope {
		if d.bootstrap(id) {
			continue
		}
		for _, i := range ix.bySubject[id] {
			add(ix.claims[i])
		}
	}
	var deps Dependencies
	for _, in := range inputs {
		if in.Status == InputGap {
			deps.Gaps = append(deps.Gaps, in.Gap)
		}
		for _, i := range in.covers {
			add(ix.claims[i])
		}
	}
	deps.Objects = slices.SortedFunc(maps.Keys(objects), func(a, b Dependent) int {
		return strings.Compare(refKey(a.Object)+"\x00"+a.Kind, refKey(b.Object)+"\x00"+b.Kind)
	})
	slices.SortFunc(deps.Gaps, compareGap)
	return deps
}

func compareGap(a, b Gap) int {
	if c := model.CompareID(a.Subject, b.Subject); c != 0 {
		return c
	}
	if c := strings.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := model.CompareID(a.Slot, b.Slot); c != 0 {
		return c
	}
	if c := strings.Compare(string(a.Level), string(b.Level)); c != 0 {
		return c
	}
	return strings.Compare(a.Cluster, b.Cluster)
}

func refKey(r model.Ref) string {
	return r.Group + "\x00" + r.Kind + "\x00" + r.Namespace + "\x00" + r.Name
}

// input is an [Input] and the claims covering it.
type input struct {
	Input
	covers []int // indices of the covering claims; none for a gap or bootstrap input
}

// notCoverable is an input no claim kind can cover.
func notCoverable(g Gap) input {
	return input{Input: Input{Gap: g, Status: InputGap, Reason: ReasonNotCoverable}}
}

// depIndex is the snapshot's structure as Dependencies reads it.
type depIndex struct {
	parent   map[model.ID]model.ID
	children map[model.ID][]model.ID
	hcms     map[model.ID]*hcmInfo
	owner    map[model.ID]*hcmInfo   // virtual host, route or weighted backend → its HCM
	users    map[model.ID][]*hcmInfo // route config source → the HCMs whose use resolves to it
	reports  map[model.ID][]model.ID // static source → the inline sources that report it
	chains   map[model.ID]*model.NetworkFilterChain
	sources  map[model.ID]*model.RouteConfigSource
	vhosts   map[model.ID]*model.VirtualHost
	routes   map[model.ID]*model.Route
	backends map[model.ID]*model.WeightedCluster
	placed   map[[3]model.ID][]int // (route, record subject, slot) → kept link and record, both at inferred or exact
	static   map[model.ID]bool     // static (bootstrap) listeners
	fetched  map[model.ID]bool     // uses in static listeners whose content is not inline, e.g. RDS
	bootSrc  map[model.ID]bool     // static sources only static listeners' inline sources report
}

// bootstrap reports whether id comes from the proxy's startup config: a static
// listener and anything in it except fetched route config content, or a
// static source only its inline sources report.
func (d *depIndex) bootstrap(id model.ID) bool {
	if d.bootSrc[id] {
		return true
	}
	l, _, _ := strings.Cut(string(id), "/")
	if !d.static[model.ID(l)] {
		return false
	}
	for use := range d.fetched {
		if id.Within(use) {
			return false
		}
	}
	return true
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
		owner: map[model.ID]*hcmInfo{}, users: map[model.ID][]*hcmInfo{}, reports: map[model.ID][]model.ID{}, chains: map[model.ID]*model.NetworkFilterChain{},
		sources: map[model.ID]*model.RouteConfigSource{}, vhosts: map[model.ID]*model.VirtualHost{}, routes: map[model.ID]*model.Route{},
		backends: map[model.ID]*model.WeightedCluster{}, placed: map[[3]model.ID][]int{}, static: map[model.ID]bool{}, fetched: map[model.ID]bool{}, bootSrc: map[model.ID]bool{}}
	for _, l := range ix.snap.Listeners {
		if l.Static {
			d.static[l.ID] = true
		}
	}
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
			d.chains[fc.ID] = fc
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
			if s := d.sources[hcm.RouteConfigUse.Source]; l.Static && (s == nil || s.Kind != model.SourceInline) {
				d.fetched[use] = true
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
	for src, inl := range d.reports {
		d.bootSrc[src] = !slices.ContainsFunc(inl, func(id model.ID) bool { return !d.bootstrap(id) })
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
	if ix.st.kinds[block] == subjectSource {
		// A reported static source is its inline source, listener lineage included.
		for _, t := range d.reports[block] {
			maps.Copy(in, ix.lineage(t))
		}
	}
	scope := maps.Clone(in)
	var out []input
	srcIn := map[*hcmInfo]bool{} // the HCMs serving the block; true when its source is in scope
	inRoutes := map[model.ID][]model.ID{}
	for id := range in {
		switch ix.st.kinds[id] {
		case subjectListener:
			out = append(out, ix.alternative(id, KindGatewaySource))
		case subjectFilterChain:
			fc := d.chains[id]
			for _, nf := range fc.NetworkFilters {
				if fc.HCM == nil || nf.ID != fc.HCM.ID {
					out = append(out, notCoverable(Gap{Subject: nf.ID, Kind: KindExtensionSource}))
				}
			}
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
			r := d.routes[id]
			if a := r.Action; r.Cluster != "" {
				out = append(out, ix.alternative(id, KindBackendSource))
			} else if a.ClusterHeader != "" || a.ClusterSpecifierPlugin != "" || a.InlineClusterSpecifierPlugin != nil {
				// The cluster is picked per request: no backend claim covers it.
				out = append(out, notCoverable(Gap{Subject: id, Kind: KindBackendSource}))
			}
			for _, m := range r.MirrorClusters {
				out = append(out, ix.cover(id, KindBackendSource, m))
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
				out = append(out, notCoverable(Gap{Subject: block, Kind: KindPolicyRecord, Level: LevelRouteConfig}))
			}
		}
	}
	// entries enumerates the per-filter config at `at` for h's slots.
	entries := func(h *hcmInfo, at model.ID, level Level, pfc map[string]any, routes []model.ID) {
		for name := range pfc {
			slots := h.slots[name]
			if len(slots) == 0 {
				out = append(out, notCoverable(Gap{Subject: at, Kind: KindPolicyRecord, Slot: h.upstreamBy[name], Level: level}))
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
				out = append(out, notCoverable(Gap{Subject: id, Kind: KindPolicyRecord, Slot: slot}))
			}
		}
	}
	// A dropped claim's kind on a subject in scope is a gap (Compose rule 3).
	for _, c := range ix.dropped {
		if scope[c.Subject] && c.Assertion != nil {
			out = append(out, input{Input: Input{Gap: Gap{Subject: c.Subject, Kind: kindOf(c.Assertion)}, Status: InputGap, Reason: ReasonDropped}})
		}
	}
	return scope, merge(d, out)
}

// merge joins inputs with the same gap: a gap wins over a cover, and a drop
// over any other reason. Bootstrap inputs lose their status and covers.
func merge(d *depIndex, ins []input) []input {
	var out []input
	at := map[Gap]int{}
	for _, in := range ins {
		if d.bootstrap(in.Subject) {
			in.Status, in.Reason, in.covers = InputBootstrap, "", nil
		}
		i, ok := at[in.Gap]
		if !ok {
			at[in.Gap] = len(out)
			out = append(out, in)
			continue
		}
		m := &out[i]
		m.covers = append(m.covers, in.covers...)
		if in.Status == InputGap && (m.Status == InputCovered || in.Reason == ReasonDropped) {
			m.Status, m.Reason = in.Status, in.Reason
		}
		if m.Candidates == nil {
			m.Candidates = in.Candidates
		}
	}
	return out
}

// alternative is the input on id that an alternatives kind covers.
func (ix *Index) alternative(id model.ID, kind string) input {
	return ix.cover(id, kind, "")
}

// cover is the input on id that an alternatives kind covers; for a
// BackendSource, the one behind the mirror cluster (empty: id's own cluster).
func (ix *Index) cover(id model.ID, kind, mirror string) input {
	var c model.Certainty
	full := false
	switch kind {
	case KindGatewaySource:
		c = Resolve[GatewaySource](ix, id).Certainty
	case KindHostSource:
		c = Resolve[HostSource](ix, id).Certainty
	case KindRouteSource:
		res := Resolve[RouteSource](ix, id)
		c, full = res.Certainty, complete(res)
	case KindExtensionSource:
		c = Resolve[ExtensionSource](ix, id).Certainty
	case KindBackendSource:
		c = resolve[BackendSource](ix, id, mirror).Certainty
	}
	in := input{Input: Input{Gap: Gap{Subject: id, Kind: kind, Cluster: mirror}, Status: InputCovered, Complete: full}}
	switch {
	case kind == KindRouteSource && !full && c >= model.Ambiguous:
		in.Status, in.Reason = InputGap, ReasonIncomplete
	case c == model.Ambiguous:
		in.Status, in.Reason = InputGap, ReasonAmbiguous
	case c < model.Inferred:
		in.Status, in.Reason = InputGap, ReasonNoEvidence
	}
	for _, i := range ix.bySubject[id] {
		cl := ix.claims[i]
		if r, ok := cl.Assertion.(RouteSource); ok && kind == KindRouteSource {
			in.Candidates = append(in.Candidates, Candidate{Ref: r.Route, Claim: cl})
		}
		if b, ok := cl.Assertion.(BackendSource); ok && b.Mirror != mirror {
			continue
		}
		if in.Status == InputCovered && cl.Assertion.Kind() == kind && cl.Certainty >= model.Inferred {
			in.covers = append(in.covers, i)
		}
	}
	return in
}

// complete reports whether a route's readings at the highest certainty name
// every possible source: an exact reading, or readings none marks Incomplete.
func complete(res Resolution[RouteSource]) bool {
	top := model.Unknown
	for _, c := range res.Claims {
		top = max(top, c.Certainty)
	}
	if top < model.Ambiguous {
		return false
	}
	for _, c := range res.Claims {
		if c.Certainty == top && top < model.Exact && c.Assertion.(RouteSource).Incomplete {
			return false
		}
	}
	return true
}

// placement is the input that links on each of routes cover by placing a
// record on `at` onto slot. With no routes, nothing can cover it.
func (ix *Index) placement(gap Gap, at, slot model.ID, routes []model.ID) input {
	if len(routes) == 0 {
		return notCoverable(gap)
	}
	d := ix.deps()
	in := input{Input: Input{Gap: gap, Status: InputCovered}}
	for _, r := range routes {
		c := d.placed[[3]model.ID{r, at, slot}]
		if len(c) == 0 {
			return input{Input: Input{Gap: gap, Status: InputGap, Reason: ReasonNoEvidence}}
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

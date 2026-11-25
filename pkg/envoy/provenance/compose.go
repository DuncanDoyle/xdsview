package provenance

import (
	"fmt"
	"slices"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Module makes claims about a snapshot. Claims gets the module's own deep copy
// of the snapshot, so it may change it freely; changes reach no one else.
type Module interface {
	Name() string
	Claims(snap *model.Snapshot) []Claim
}

// Compose runs every module on its own copy of snap and joins their claims
// into an [Index]. It fails only on a programming error: duplicate module
// names. Claims that break a rule are dropped with a warning on the index:
//
//  1. Each module gets its own deep copy of snap; snap is not changed.
//  2. Claims keep module order, then emission order, within a subject.
//  3. A claim is dropped when its subject is not in snap or is the wrong kind
//     of occurrence for its assertion, it has no evidence, an object its
//     assertion names is not the Object of any of its evidence, its certainty
//     exceeds its kind's cap, or its Ref repeats one of its module's.
//  4. A [FilterLink] must name a [PolicyRecord] of the same module that
//     survived rule 3, on the route, one of its ancestors, or the route config
//     source its use resolves to. Its slot must be an HTTP filter of the HCM
//     serving the route, and the route's chain must have per-filter config
//     for that slot at some level or on a weighted backend. When two links
//     from one record on one route pass these checks, both are dropped.
//
// The index keeps snap to answer [Index.Dependencies]; treat snap as read-only.
func Compose(snap *model.Snapshot, modules ...Module) (*Index, error) {
	seen := map[string]bool{}
	for _, m := range modules {
		if seen[m.Name()] {
			return nil, fmt.Errorf("provenance: duplicate module name %q", m.Name())
		}
		seen[m.Name()] = true
	}
	ix := &Index{snap: snap, st: newStructure(snap)}
	type ordered struct {
		c        Claim
		mod, pos int
	}
	var kept []ordered
	for mi, m := range modules {
		claims := m.Claims(cloneSnapshot(snap))
		refs := map[string]Claim{}
		var links []ordered
		for n, c := range claims {
			c.ID, c.Module = fmt.Sprintf("%s/%d", m.Name(), n), m.Name()
			if reason := ix.st.check(c); reason != "" {
				ix.drop(c, model.WarnClaimDropped, reason, true)
				continue
			}
			if c.Ref != "" {
				if _, dup := refs[c.Ref]; dup {
					ix.drop(c, model.WarnClaimDropped, fmt.Sprintf("ref %q repeats", c.Ref), true)
					continue
				}
				refs[c.Ref] = c
			}
			if _, ok := c.Assertion.(FilterLink); ok {
				links = append(links, ordered{c, mi, n})
				continue
			}
			kept = append(kept, ordered{c, mi, n})
		}
		// Only links that pass the other checks compete for a record on a route.
		reasons := make([]string, len(links))
		count := map[[2]string]int{}
		for i, l := range links {
			link := l.c.Assertion.(FilterLink)
			if reasons[i] = ix.st.checkLink(l.c, link, refs); reasons[i] == "" {
				count[[2]string{link.Record, string(l.c.Subject)}]++
			}
		}
		for i, l := range links {
			if reasons[i] == "" && count[[2]string{l.c.Assertion.(FilterLink).Record, string(l.c.Subject)}] > 1 {
				reasons[i] = "another link from the same record on this route"
			}
			if reasons[i] != "" {
				ix.drop(l.c, model.WarnLinkDropped, reasons[i], false)
				continue
			}
			kept = append(kept, l)
		}
	}
	slices.SortStableFunc(kept, func(a, b ordered) int {
		if c := model.CompareID(a.c.Subject, b.c.Subject); c != 0 {
			return c
		}
		if a.mod != b.mod {
			return a.mod - b.mod
		}
		return a.pos - b.pos
	})
	ix.bySubject = map[model.ID][]int{}
	ix.byRef = map[[2]string]int{}
	for i, o := range kept {
		ix.claims = append(ix.claims, o.c)
		ix.bySubject[o.c.Subject] = append(ix.bySubject[o.c.Subject], i)
		if o.c.Ref != "" {
			ix.byRef[[2]string{o.c.Module, o.c.Ref}] = i
		}
	}
	return ix, nil
}

// drop records a dropped claim; rule-3 drops become gaps (design §5).
func (ix *Index) drop(c Claim, code, reason string, gap bool) {
	kind := "none"
	if c.Assertion != nil {
		kind = c.Assertion.Kind()
	}
	ix.warnings = append(ix.warnings, model.Warning{Subject: c.Subject, Code: code,
		Message: fmt.Sprintf("dropped claim %s (%s on %q): %s", c.ID, kind, c.Subject, reason)})
	if gap {
		ix.dropped = append(ix.dropped, c)
	}
}

// structure is what Compose and the index know about snap's occurrences.
type structure struct {
	kinds          map[model.ID]subjectKind
	hcms           []model.ID
	useSource      map[model.ID]model.ID   // route config use → its source
	slots          map[model.ID][]model.ID // HCM → its HTTP filter slots
	chains         map[model.ID]model.EffectiveChain
	backendFilters map[model.ID]map[string]bool // route → filter names its weighted backends configure
}

func newStructure(snap *model.Snapshot) *structure {
	st := &structure{kinds: map[model.ID]subjectKind{}, useSource: map[model.ID]model.ID{}, slots: map[model.ID][]model.ID{},
		chains: chain.Effective(snap), backendFilters: map[model.ID]map[string]bool{}}
	for _, s := range snap.RouteConfigs {
		st.kinds[s.ID] = subjectSource
	}
	for _, l := range snap.Listeners {
		st.kinds[l.ID] = subjectListener
		for _, fc := range l.FilterChains {
			st.kinds[fc.ID] = subjectFilterChain
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			st.kinds[hcm.ID] = subjectHCM
			st.hcms = append(st.hcms, hcm.ID)
			st.kinds[hcm.RouteConfigUse.ID] = subjectUse
			if hcm.RouteConfigUse.Source != "" {
				st.useSource[hcm.RouteConfigUse.ID] = hcm.RouteConfigUse.Source
			}
			for _, f := range hcm.HTTPFilters {
				st.kinds[f.ID] = subjectSlot
				st.slots[hcm.ID] = append(st.slots[hcm.ID], f.ID)
			}
			for _, f := range hcm.UpstreamFilters {
				st.kinds[f.ID] = subjectUpstreamSlot
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for _, vh := range hcm.RouteConfig.VirtualHosts {
				st.kinds[vh.ID] = subjectVirtualHost
				for _, r := range vh.Routes {
					st.kinds[r.ID] = subjectRoute
					for _, w := range r.WeightedClusters {
						st.kinds[w.ID] = subjectBackend
						for name := range w.TypedPerFilterConfig {
							if st.backendFilters[r.ID] == nil {
								st.backendFilters[r.ID] = map[string]bool{}
							}
							st.backendFilters[r.ID][name] = true
						}
					}
				}
			}
		}
	}
	return st
}

// check applies Compose rule 3 and returns why c breaks it, or "".
func (st *structure) check(c Claim) string {
	if c.Assertion == nil {
		return "no assertion"
	}
	spec := c.Assertion.spec()
	k, ok := st.kinds[c.Subject]
	if !ok {
		return "subject not in the snapshot"
	}
	if !slices.Contains(spec.subjects, k) {
		return "subject is the wrong kind of occurrence"
	}
	if r, ok := c.Assertion.(PolicyRecord); ok && policyLevels[k] != r.Level {
		return fmt.Sprintf("level %q does not match the subject", r.Level)
	}
	if len(c.Evidence) == 0 {
		return "no evidence"
	}
	for _, o := range c.Assertion.objects() {
		if !slices.ContainsFunc(c.Evidence, func(e Evidence) bool { return e.Object != nil && *e.Object == o }) {
			return fmt.Sprintf("no evidence names %s", o)
		}
	}
	if c.Certainty > spec.cap {
		return fmt.Sprintf("certainty %s exceeds the cap %s", c.Certainty, spec.cap)
	}
	return ""
}

// checkLink applies Compose rule 4 to one link and returns why it fails, or "".
func (st *structure) checkLink(c Claim, l FilterLink, refs map[string]Claim) string {
	rec, ok := refs[l.Record]
	if !ok {
		return fmt.Sprintf("record %q is not a claim of this module", l.Record)
	}
	if _, ok := rec.Assertion.(PolicyRecord); !ok {
		return fmt.Sprintf("record %q is not a policy record", l.Record)
	}
	route := c.Subject
	if !route.Within(rec.Subject) && st.source(route) != rec.Subject {
		return fmt.Sprintf("record %q is on %s, which does not apply to the route", l.Record, rec.Subject)
	}
	hcm := st.hcmOf(route)
	if !slices.Contains(st.slots[hcm], l.Slot) {
		return fmt.Sprintf("slot %s is not an HTTP filter of the HCM serving the route", l.Slot)
	}
	if !st.configured(route, l.Slot) {
		return fmt.Sprintf("the route has no per-filter config for slot %s", l.Slot)
	}
	return ""
}

// hcmOf returns the HCM an occurrence lies in, or "".
func (st *structure) hcmOf(id model.ID) model.ID {
	for _, hcm := range st.hcms {
		if id.Within(hcm) {
			return hcm
		}
	}
	return ""
}

// source returns the route config source of the use an occurrence lies in, or "".
func (st *structure) source(id model.ID) model.ID {
	for use, src := range st.useSource {
		if id.Within(use) {
			return src
		}
	}
	return ""
}

// configured reports whether route's chain has per-filter config for slot, at
// any level or on a weighted backend (chain.Effective records the level).
func (st *structure) configured(route, slot model.ID) bool {
	c := st.chains[route]
	for _, e := range c.Filters {
		if e.Slot != slot {
			continue
		}
		if e.Level != "" {
			return true
		}
		return st.backendFilters[route][e.Name]
	}
	return false
}

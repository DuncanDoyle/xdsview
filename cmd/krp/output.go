package main

import (
	"encoding/json"
	"io"
	"slices"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

const (
	outputText = "text"
	outputJSON = "json"
)

// schemaVersion versions the -o json envelope. It changes when a field is
// removed or renamed, not when one is added (decision D-005).
const schemaVersion = "v1alpha2"

// jsonEnvelope is the -o json document (docs/json-schema.md). The golden files
// in testdata/golden pin its shape. Every field but selection is projected
// onto the subjects snapshot retains.
type jsonEnvelope struct {
	SchemaVersion string                            `json:"schemaVersion"`
	Snapshot      *model.Snapshot                   `json:"snapshot"`
	Chains        map[model.ID]model.EffectiveChain `json:"chains"`
	Claims        []provenance.Claim                `json:"claims"`
	Selection     *selection                        `json:"selection,omitempty"`
	Warnings      []model.Warning                   `json:"warnings"`
}

// selection is krp's route selection. It describes the whole dump, so its
// entries may name routes absent from snapshot; they carry route and vhost names.
type selection struct {
	Query      *selectionQuery `json:"query,omitempty"`
	Routes     []model.ID      `json:"routes"`     // the routes krp prints
	Ambiguous  []routeName     `json:"ambiguous"`  // may belong to the HTTPRoute, but krp can't tell, so it leaves them out
	Unresolved []routeName     `json:"unresolved"` // the name could name the HTTPRoute, but has no valid reading
	Narrowed   []narrowing     `json:"narrowed"`   // readings the known HTTPRoutes narrowed
}

// selectionQuery is --route and --rule.
type selectionQuery struct {
	Route     model.Ref `json:"route"`
	RuleIndex *int      `json:"ruleIndex,omitempty"`
	RuleName  string    `json:"ruleName,omitempty"`
}

// routeName identifies a route by ID and by name, for routes snapshot may lack.
type routeName struct {
	ID          model.ID `json:"id"`
	Name        string   `json:"name"`
	VirtualHost string   `json:"virtualHost"`
}

// narrowing is one route whose readings the known HTTPRoutes narrowed.
// Selected marks a route krp prints because the one reading kept matches
// --route, though kgateway.Select leaves it ambiguous (D-022).
type narrowing struct {
	routeName
	Kept     []provenance.Claim `json:"kept"`
	Dropped  []provenance.Claim `json:"dropped"`
	Selected bool               `json:"selected,omitempty"`
}

// newSelection is sel as krp applies it: it also selects a narrowed route
// whose one kept reading matches q (D-022).
func newSelection(snap *model.Snapshot, q kgateway.Query, sel kgateway.Selection) *selection {
	names := routeNames(snap)
	out := &selection{
		Query:      &selectionQuery{Route: q.Route, RuleIndex: q.RuleIndex, RuleName: q.RuleName},
		Routes:     slices.Clone(sel.Routes),
		Ambiguous:  []routeName{},
		Unresolved: []routeName{},
	}
	for _, n := range sel.Narrowed {
		if r, ok := oneReading(n.Kept); ok && matches(q, r) {
			out.Routes = append(out.Routes, n.Route)
		}
	}
	slices.SortFunc(out.Routes, model.CompareID)
	if out.Routes == nil {
		out.Routes = []model.ID{}
	}
	for _, id := range sel.Ambiguous {
		if !slices.Contains(out.Routes, id) {
			out.Ambiguous = append(out.Ambiguous, names[id])
		}
	}
	for _, id := range sel.Unresolved {
		out.Unresolved = append(out.Unresolved, names[id])
	}
	out.Narrowed = narrowed(snap, sel.Narrowed, out.Routes)
	return out
}

// liveSelection is the selection in live mode without --route: the
// narrowings only.
func liveSelection(snap *model.Snapshot, ns []provenance.Narrowing) *selection {
	return &selection{Routes: []model.ID{}, Ambiguous: []routeName{}, Unresolved: []routeName{}, Narrowed: narrowed(snap, ns, nil)}
}

// narrowed converts narrowings, marking those in selected.
func narrowed(snap *model.Snapshot, ns []provenance.Narrowing, selected []model.ID) []narrowing {
	names := routeNames(snap)
	out := []narrowing{}
	for _, n := range ns {
		out = append(out, narrowing{routeName: names[n.Route], Kept: n.Kept, Dropped: n.Dropped, Selected: slices.Contains(selected, n.Route)})
	}
	return out
}

// matches reports whether a reading is the queried HTTPRoute and rule.
func matches(q kgateway.Query, o provenance.RouteSource) bool {
	return o.Route == q.Route && (q.RuleIndex == nil || o.RuleIndex == *q.RuleIndex) && (q.RuleName == "" || o.RuleName == q.RuleName)
}

// oneReading returns the reading kept claims share, if they name one. Like
// provenance.Resolve, it compares route, rule index, rule name and match index.
func oneReading(kept []provenance.Claim) (provenance.RouteSource, bool) {
	if len(kept) == 0 {
		return provenance.RouteSource{}, false
	}
	first := kept[0].Assertion.(provenance.RouteSource)
	for _, c := range kept[1:] {
		o := c.Assertion.(provenance.RouteSource)
		if o.Route != first.Route || o.RuleIndex != first.RuleIndex || o.RuleName != first.RuleName || o.MatchIndex != first.MatchIndex {
			return provenance.RouteSource{}, false
		}
	}
	return first, true
}

// routeNames maps every route of snap to its name and its virtual host's.
func routeNames(snap *model.Snapshot) map[model.ID]routeName {
	out := map[model.ID]routeName{}
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					out[r.ID] = routeName{ID: r.ID, Name: r.Name, VirtualHost: vh.Name}
				}
			}
		}
	}
	return out
}

// writeJSON writes res as one indented v1alpha2 document.
func writeJSON(w io.Writer, res result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope(res))
}

// envelope projects res onto the subjects its snapshot retains: chains,
// claims and warnings of pruned subjects are left out, and so is a filter link
// whose record is. Warnings move from the
// snapshot to the envelope, beside the index's.
func envelope(res result) jsonEnvelope {
	snap := *res.snap
	snap.Warnings = nil
	if snap.Listeners == nil {
		snap.Listeners = []model.Listener{} // emit [] rather than null
	}
	present := ids(res.snap)
	all := ids(res.full)
	env := jsonEnvelope{SchemaVersion: schemaVersion, Snapshot: &snap, Chains: map[model.ID]model.EffectiveChain{},
		Claims: []provenance.Claim{}, Selection: res.selection, Warnings: []model.Warning{}}
	for id, c := range res.chains {
		if present[id] {
			env.Chains[id] = c
		}
	}
	for _, c := range res.ix.Claims() {
		// A link stays only with the record it names.
		if rec, ok := res.ix.Record(c); projected(c, present) && (!ok || projected(rec, present)) {
			env.Claims = append(env.Claims, c)
		}
	}
	env.Warnings = append(env.Warnings, res.snap.Warnings...)
	for _, w := range res.ix.Warnings() {
		switch {
		case w.Subject == "" || present[w.Subject]:
			env.Warnings = append(env.Warnings, w)
		case !all[w.Subject]:
			// The claim named a subject the dump doesn't have; the message says which.
			w.Subject = ""
			env.Warnings = append(env.Warnings, w)
		}
	}
	return env
}

// projected reports whether every ID claim c refers to is present.
func projected(c provenance.Claim, present map[model.ID]bool) bool {
	if !present[c.Subject] {
		return false
	}
	for _, e := range c.Evidence {
		if !present[e.Subject] {
			return false
		}
	}
	if l, ok := c.Assertion.(provenance.FilterLink); ok && !present[l.Slot] {
		return false
	}
	return true
}

// ids returns every occurrence ID in snap.
func ids(snap *model.Snapshot) map[model.ID]bool {
	out := map[model.ID]bool{}
	for _, s := range snap.RouteConfigs {
		out[s.ID] = true
	}
	for _, l := range snap.Listeners {
		out[l.ID] = true
		for _, fc := range l.FilterChains {
			out[fc.ID] = true
			for _, nf := range fc.NetworkFilters {
				out[nf.ID] = true
			}
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			out[hcm.ID] = true
			out[hcm.RouteConfigUse.ID] = true
			for _, f := range append(slices.Clone(hcm.HTTPFilters), hcm.UpstreamFilters...) {
				out[f.ID] = true
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for _, vh := range hcm.RouteConfig.VirtualHosts {
				out[vh.ID] = true
				for _, r := range vh.Routes {
					out[r.ID] = true
					for _, wc := range r.WeightedClusters {
						out[wc.ID] = true
					}
				}
			}
		}
	}
	delete(out, "")
	return out
}

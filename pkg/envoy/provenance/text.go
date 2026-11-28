package provenance

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// Labels returns a [render.Options.Labels] hook with the on-row origin text:
// a listener's Gateway and port or section, a virtual host's listener, a
// route's reading (a narrowed one marked as chosen from known objects), and on
// a filter row the extension that defines the filter and the policy records
// linked to it on that route. Consumers with their own context add theirs.
func Labels(ix *Index, narrowings []Narrowing) func(render.Row) []string {
	narrowed := map[model.ID]Narrowing{}
	for _, n := range narrowings {
		narrowed[n.Route] = n
	}
	return func(r render.Row) []string {
		if r.Slot != "" {
			return ix.filterLabels(r)
		}
		var s string
		switch ix.st.kinds[r.Subject] {
		case subjectListener:
			s = ix.listenerLabel(r.Subject, true)
		case subjectVirtualHost:
			s = ix.listenerLabel(r.Subject, false)
		case subjectRoute:
			n, ok := narrowed[r.Subject]
			s = ix.routeLabel(r.Subject, n, ok)
		}
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

// Labels of the [Notes] lines. A caller that rewords a line matches on these.
const (
	PoliciesLabel   = "policies"
	AboveRouteLabel = "policies recorded above the route"
)

// Notes returns a [render.Options.Trailer] hook with the lines printed after
// a route's filters: [PoliciesLabel] for the route's own policy records, and
// [AboveRouteLabel] for those at its virtual host, route config and listener,
// each policy field once with its levels. A record says where the controller
// recorded a policy, not whether it applies to the route.
func Notes(ix *Index) func(render.Row) []render.Line {
	return func(r render.Row) []render.Line {
		if r.Route == "" || r.Slot != "" {
			return nil
		}
		var own []string
		var inherited policyGroups
		for _, c := range Records(ix, ix.snap, r.Route) {
			p := c.Assertion.(PolicyRecord)
			if p.Level == LevelRoute {
				own = append(own, policyText(p.Policy, []string{p.Field}, c.Certainty))
			} else {
				inherited = inherited.add(p, c.Certainty)
			}
		}
		var out []render.Line
		if len(own) > 0 {
			out = append(out, render.Line{Label: PoliciesLabel, Text: strings.Join(own, ", ")})
		}
		if len(inherited) > 0 {
			out = append(out, render.Line{Label: AboveRouteLabel, Text: inherited.text()})
		}
		return out
	}
}

var levelWords = map[Level]string{LevelRoute: "route", LevelVirtualHost: "vhost", LevelRouteConfig: "route config", LevelListener: "listener"}

// specificity ranks levels as Envoy reads per-filter config: route first.
var specificity = map[Level]int{LevelListener: 0, LevelRouteConfig: 1, LevelVirtualHost: 2, LevelRoute: 3}

// policyGroup is one policy field on a row with every level it is recorded at.
type policyGroup struct {
	policy    model.Ref
	field     string
	levels    []Level
	certainty model.Certainty
}

// policyGroups keeps groups in the order their first record was added.
type policyGroups []policyGroup

// add files p under its policy and field, with the lowest certainty seen.
func (gs policyGroups) add(p PolicyRecord, c model.Certainty) policyGroups {
	for i := range gs {
		if g := &gs[i]; g.policy == p.Policy && g.field == p.Field {
			if !slices.Contains(g.levels, p.Level) {
				g.levels = append(g.levels, p.Level)
			}
			g.certainty = min(g.certainty, c)
			return gs
		}
	}
	return append(gs, policyGroup{policy: p.Policy, field: p.Field, levels: []Level{p.Level}, certainty: c})
}

// texts formats each group, e.g. "TrafficPolicy ns/x (cors, inferred; route config, listener)".
// Levels print most specific first; a route-only group prints none.
func (gs policyGroups) texts() []string {
	var out []string
	for _, g := range gs {
		var clauses []string
		if notes := noteText([]string{g.field}, g.certainty); notes != "" {
			clauses = append(clauses, notes)
		}
		levels := slices.SortedFunc(slices.Values(g.levels), func(a, b Level) int { return specificity[b] - specificity[a] })
		if !slices.Equal(levels, []Level{LevelRoute}) {
			clauses = append(clauses, levelText(levels))
		}
		out = append(out, withClauses(g.policy, clauses))
	}
	return out
}

func (gs policyGroups) text() string { return strings.Join(gs.texts(), ", ") }

func levelText(levels []Level) string {
	var words []string
	for _, l := range levels {
		words = append(words, levelWords[l])
	}
	return strings.Join(words, ", ")
}

// withClauses writes ref with its clauses in parentheses, separated by "; ".
func withClauses(ref model.Ref, clauses []string) string {
	if len(clauses) == 0 {
		return ref.String()
	}
	return ref.String() + " (" + strings.Join(clauses, "; ") + ")"
}

// policyText formats a policy with its notes and certainty unless exact,
// e.g. "TrafficPolicy ns/x (extAuth, inferred)".
func policyText(ref model.Ref, notes []string, c model.Certainty) string {
	if s := noteText(notes, c); s != "" {
		return ref.String() + " (" + s + ")"
	}
	return ref.String()
}

// noteText joins the non-empty notes and the certainty unless exact.
func noteText(notes []string, c model.Certainty) string {
	notes = slices.DeleteFunc(slices.Clone(notes), func(s string) bool { return s == "" })
	if c != model.Exact {
		notes = append(notes, c.String())
	}
	return strings.Join(notes, ", ")
}

// suffix names a certainty unless it is exact, which prints bare.
func suffix(c model.Certainty) string {
	if c == model.Exact {
		return ""
	}
	return " (" + c.String() + ")"
}

// listenerLabel formats a listener's or vhost's origin, e.g.
// "Gateway ns/gw port 80 (inferred)". Only a listener shows its Gateway.
func (ix *Index) listenerLabel(id model.ID, gateway bool) string {
	var parts []string
	certainty := model.Unknown
	if gateway {
		if g := Resolve[GatewaySource](ix, id); g.Certainty >= model.Inferred && len(g.Candidates) == 1 {
			parts = append(parts, g.Candidates[0].Gateway.String())
			certainty = g.Certainty
		}
	}
	ls := Resolve[ListenerSource](ix, id)
	switch {
	case ls.Certainty == model.Ambiguous:
		var alts []string
		for _, c := range ls.Candidates {
			alts = append(alts, listenerPart(c))
		}
		parts = append(parts, "ambiguous: "+strings.Join(alts, ", "))
		certainty = model.Exact // already said
	case len(ls.Candidates) == 1:
		if p := listenerPart(ls.Candidates[0]); p != "" {
			parts = append(parts, p)
		}
		certainty = ls.Certainty
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + suffix(certainty)
}

func listenerPart(l ListenerSource) string {
	switch {
	case l.Section != "":
		return "listener " + l.Section
	case l.Port > 0:
		return fmt.Sprintf("port %d", l.Port)
	}
	return ""
}

// routeLabel formats a route's origin, e.g. "HTTPRoute ns/web · rule 1 (api) · match 0".
func (ix *Index) routeLabel(id model.ID, n Narrowing, narrowed bool) string {
	res := Resolve[RouteSource](ix, id)
	cands := res.Candidates
	if narrowed {
		cands = nil
		for _, c := range n.Kept {
			if a, ok := c.Assertion.(RouteSource); ok && !slices.ContainsFunc(cands, func(b RouteSource) bool { return b == a }) {
				cands = append(cands, a)
			}
		}
	}
	if len(cands) == 0 {
		return ""
	}
	if len(cands) > 1 {
		var refs []string
		for _, c := range cands {
			refs = append(refs, c.Route.String())
		}
		return "ambiguous: " + strings.Join(slices.Compact(refs), ", ")
	}
	o := cands[0]
	s := o.Route.String()
	if o.RuleIndex >= 0 {
		s += fmt.Sprintf(" · rule %d", o.RuleIndex)
	}
	if o.RuleName != "" {
		s += " (" + o.RuleName + ")"
	}
	if o.MatchIndex >= 0 {
		s += fmt.Sprintf(" · match %d", o.MatchIndex)
	}
	if o.IndexCertainty == model.Inferred {
		s += " (partly from name)"
	}
	if narrowed {
		return s + " (chosen from known " + o.Route.Kind + "s)"
	}
	return s + suffix(res.Certainty)
}

// filterLabels lists what defines a filter row's slot and the policy records
// linked to it on the row's route.
func (ix *Index) filterLabels(r render.Row) []string {
	if r.Route == "" {
		return nil
	}
	var out []string
	if ext := Resolve[ExtensionSource](ix, r.Slot); len(ext.Candidates) == 1 && ext.Candidates[0].Extension != nil {
		out = append(out, policyText(*ext.Candidates[0].Extension, nil, ext.Certainty))
	}
	var linked policyGroups
	for _, c := range ix.About(r.Route) {
		l, ok := c.Assertion.(FilterLink)
		if !ok || l.Slot != r.Slot {
			continue
		}
		if rec, ok := ix.Record(c); ok {
			linked = linked.add(rec.Assertion.(PolicyRecord), min(rec.Certainty, c.Certainty))
		}
	}
	return append(out, linked.texts()...)
}

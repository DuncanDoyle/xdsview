package kgateway

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// Query names the routes to select: those of one HTTPRoute, optionally one rule.
type Query struct {
	Route     model.Ref   // an HTTPRoute (gateway.networking.k8s.io)
	RuleIndex *int        // nil: any rule. With RuleName, both must match
	RuleName  string      // empty: any rule
	Known     []model.Ref // HTTPRoutes that exist; narrows readings (see Select)
}

// Selection is the result of [Select]. Every list is in snapshot order.
type Selection struct {
	Routes     []model.ID  // one reading, matching the query, not narrowed by Known
	Ambiguous  []model.ID  // several readings, or one left only by Known; at least one matches
	Unresolved []model.ID  // the route name could name the queried HTTPRoute and rule index, but has no valid reading
	Narrowed   []Narrowing // every route in the lists above that Known narrowed
}

// Narrowing records which readings of a route Known kept and removed.
type Narrowing = provenance.Narrowing

// Select returns the routes of snap that q names, from the RouteSource claims
// in ix (rules from design §6):
//  1. q.Route is an HTTPRoute; other route kinds are not selected.
//  2. A reading matches when its HTTPRoute is q.Route, and its rule index and
//     rule name are q's where set.
//  3. Routes: the route resolves to one reading at inferred or exact, which
//     matches, and Known removed no reading. Readings are counted as
//     [provenance.Resolve] merges them; one rated ambiguous stays ambiguous.
//  4. Ambiguous: several readings and at least one matches, Known's removals
//     included; or one reading left only because Known removed the others.
//  5. Known removes a reading whose HTTPRoute it lacks, never an exact one,
//     and never the last: when every reading is outside Known, Known is
//     ignored for that route and nothing is recorded.
//  6. Known never raises certainty: a narrowed route stays ambiguous, and its
//     Narrowing names what was kept.
//
// The index keeps every candidate; prune with model.Prune and Routes.
func Select(ix *provenance.Index, snap *model.Snapshot, q Query) Selection {
	var sel Selection
	if q.Route.Group != gatewayGroup || q.Route.Kind != "HTTPRoute" {
		return sel
	}
	walkRoutes(snap, func(vh model.VirtualHost, r model.Route) {
		readings, top := readingsOf(ix, r.ID)
		if len(readings) == 0 {
			if marker(q, routePart(vh.Name, r.Name)) {
				sel.Unresolved = append(sel.Unresolved, r.ID)
			}
			return
		}
		if !slices.ContainsFunc(readings, func(c provenance.Claim) bool { return q.matches(c.Assertion.(provenance.RouteSource)) }) {
			return
		}
		n, narrowed := narrow(r.ID, readings, q.Known)
		// Resolve rates several distinct readings, or one rated ambiguous, as ambiguous.
		if !narrowed && top >= model.Inferred {
			sel.Routes = append(sel.Routes, r.ID)
			return
		}
		sel.Ambiguous = append(sel.Ambiguous, r.ID)
		if narrowed {
			sel.Narrowed = append(sel.Narrowed, n)
		}
	})
	return sel
}

// Narrow applies known to every route's readings, without a query, by the
// rules of [Select]. It returns the routes Known narrowed, in ID order.
func Narrow(ix *provenance.Index, known []model.Ref) []Narrowing {
	var routes []model.ID
	for _, c := range ix.Claims() {
		if _, ok := c.Assertion.(provenance.RouteSource); ok && !slices.Contains(routes, c.Subject) {
			routes = append(routes, c.Subject)
		}
	}
	var out []Narrowing
	for _, id := range routes {
		readings, _ := readingsOf(ix, id)
		if n, ok := narrow(id, readings, known); ok {
			out = append(out, n)
		}
	}
	return out
}

// readingsOf returns the RouteSource claims at the highest certainty, and the
// route's resolved certainty: ambiguous unless they merge into one reading.
func readingsOf(ix *provenance.Index, route model.ID) ([]provenance.Claim, model.Certainty) {
	res := provenance.Resolve[provenance.RouteSource](ix, route)
	top := model.Unknown
	for _, c := range res.Claims {
		top = max(top, c.Certainty)
	}
	var out []provenance.Claim
	for _, c := range res.Claims {
		if c.Certainty == top && top > model.Unknown {
			out = append(out, c)
		}
	}
	return out, res.Certainty
}

// narrow applies rule 5 to one route's readings.
func narrow(route model.ID, readings []provenance.Claim, known []model.Ref) (Narrowing, bool) {
	if len(known) == 0 {
		return Narrowing{}, false
	}
	n := Narrowing{Route: route}
	for _, c := range readings {
		if c.Certainty == model.Exact || slices.Contains(known, c.Assertion.(provenance.RouteSource).Route) {
			n.Kept = append(n.Kept, c)
		} else {
			n.Dropped = append(n.Dropped, c)
		}
	}
	if len(n.Kept) == 0 || len(n.Dropped) == 0 {
		return Narrowing{}, false
	}
	return n, true
}

// matches applies rule 2.
func (q Query) matches(o provenance.RouteSource) bool {
	return o.Route == q.Route &&
		(q.RuleIndex == nil || o.RuleIndex == *q.RuleIndex) &&
		(q.RuleName == "" || o.RuleName == q.RuleName)
}

// marker reports whether a route name's text could name q's HTTPRoute and
// rule index: "httproute-<name>-<ns>-[<rule>-]".
func marker(q Query, name string) bool {
	base := fmt.Sprintf("httproute-%s-%s-", q.Route.Name, q.Route.Namespace)
	if q.RuleIndex != nil {
		base += fmt.Sprintf("%d-", *q.RuleIndex)
	}
	return strings.Contains(name, base)
}

// walkRoutes calls fn for every route in snap, in snapshot order.
func walkRoutes(snap *model.Snapshot, fn func(model.VirtualHost, model.Route)) {
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					fn(vh, r)
				}
			}
		}
	}
}

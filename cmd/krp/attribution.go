package main

import (
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// contributions holds the objects krp attributes to chain entries, by route
// ID and then slot ID. It is provenance, so it sits beside the chains, not in them.
type contributions map[model.ID]map[model.ID][]model.Contribution

// attribute computes the contributions of every route's chain from m. Each
// entry lists the extension that defines its filter, then, when m implements
// [origin.PolicyFilters], the policy fields attributed to it on the route, in
// m.Policies order.
//
// An extension contribution carries the mapper's filter origin certainty. A
// policy field is attributed only when exactly one entry with per-filter
// config for the route matches it, at any level or on a weighted backend. The
// link is at most [model.Inferred], and no stronger than the policy ref: the
// metadata names the field, not the filter, so with several candidates none gets it.
func attribute(snap *model.Snapshot, chains map[model.ID]model.EffectiveChain, m origin.Mapper) contributions {
	out := contributions{}
	if m == nil {
		return out
	}
	pf, _ := m.(origin.PolicyFilters)
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			filters := fc.HCM.HTTPFilters
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					c, ok := chains[r.ID]
					if !ok || len(c.Filters) != len(filters) {
						continue
					}
					add := func(slot model.ID, ct model.Contribution) {
						if out[r.ID] == nil {
							out[r.ID] = map[model.ID][]model.Contribution{}
						}
						out[r.ID][slot] = append(out[r.ID][slot], ct)
					}
					for _, f := range filters {
						if o := m.HTTPFilter(f); o.Certainty > model.Unknown && o.Extension != nil {
							add(f.ID, model.Contribution{Kind: model.ContributionExtension, Ref: *o.Extension, Certainty: o.Certainty})
						}
					}
					if pf == nil {
						continue
					}
					for _, p := range m.Policies(r) {
						match, n := -1, 0
						for i, f := range filters {
							if configured(c.Filters[i], r) && pf.PolicyFilter(p, f) {
								match, n = i, n+1
							}
						}
						if n == 1 {
							add(filters[match].ID, model.Contribution{Kind: model.ContributionPolicy, Ref: p.Policy, Field: p.Field, Certainty: min(p.Certainty, model.Inferred)})
						}
					}
				}
			}
		}
	}
	return out
}

// configured reports whether e has per-filter config for r: at a route-config
// level, or on one of r's weighted backends, which may override the route's.
func configured(e model.ChainEntry, r model.Route) bool {
	if e.Level != "" {
		return true
	}
	for _, wc := range r.WeightedClusters {
		if _, ok := wc.TypedPerFilterConfig[e.Name]; ok {
			return true
		}
	}
	return false
}

// labels returns the contribution labels of a filter row, for render's Labels hook.
func (c contributions) labels(r render.Row) []string {
	if r.Route == "" || r.Slot == "" {
		return nil
	}
	var out []string
	for _, ct := range c[r.Route][r.Slot] {
		out = append(out, contributionLabel(ct))
	}
	return out
}

// contributionLabel formats a contribution, e.g. "TrafficPolicy ns/x (extAuth, inferred)".
func contributionLabel(c model.Contribution) string {
	var notes []string
	if c.Kind == model.ContributionPolicy && c.Field != "" {
		notes = append(notes, c.Field)
	}
	if c.Certainty != model.Exact {
		notes = append(notes, c.Certainty.String())
	}
	if len(notes) == 0 {
		return c.Ref.String()
	}
	return c.Ref.String() + " (" + strings.Join(notes, ", ") + ")"
}

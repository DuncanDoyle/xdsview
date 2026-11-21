package chain

import (
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// Annotate returns a copy of snap with Route.Chain set on every route of every
// HCM. snap is not modified. When m is set, an entry lists the extension that
// defines its filter and, if m implements [origin.PolicyFilters] and the entry
// has per-filter config, the policy fields that configure it on the route.
// Like origin.Annotate, the copy shares unannotated parts with snap.
func Annotate(snap *model.EnvoySnapshot, m origin.Mapper) *model.EnvoySnapshot {
	out := &model.EnvoySnapshot{Listeners: make([]model.Listener, len(snap.Listeners))}
	for i, l := range snap.Listeners {
		chains := make([]model.NetworkFilterChain, len(l.FilterChains))
		for j, fc := range l.FilterChains {
			if fc.HCM != nil && fc.HCM.RouteConfig != nil {
				hcm := *fc.HCM
				rc := *hcm.RouteConfig
				rc.VirtualHosts = make([]model.VirtualHost, len(hcm.RouteConfig.VirtualHosts))
				for k, vh := range hcm.RouteConfig.VirtualHosts {
					routes := make([]model.Route, len(vh.Routes))
					for n, r := range vh.Routes {
						c := Effective(hcm, vh, r)
						attribute(&c, hcm.HTTPFilters, r, m)
						r.Chain = &c
						routes[n] = r
					}
					vh.Routes = routes
					rc.VirtualHosts[k] = vh
				}
				hcm.RouteConfig = &rc
				fc.HCM = &hcm
			}
			chains[j] = fc
		}
		l.FilterChains = chains
		out.Listeners[i] = l
	}
	return out
}

// attribute adds contributions to c's entries, which are in filters' order:
// the filter's extension first, then policies in m.Policies order. A policy
// field goes to the entry it configures that has the most specific per-filter
// config: the route's own policies set route-level config, while an inherited
// provider sits at a wider level. When several match at that level, the dump
// doesn't say which, so none gets it.
func attribute(c *model.EffectiveChain, filters []model.HTTPFilter, r model.Route, m origin.Mapper) {
	if m == nil {
		return
	}
	for i, f := range filters {
		if o := m.HTTPFilter(f); o.Certainty > model.Unknown && o.Extension != nil {
			c.Filters[i].Contributions = append(c.Filters[i].Contributions, model.Contribution{Kind: model.ContributionExtension, Ref: *o.Extension})
		}
	}
	pf, ok := m.(origin.PolicyFilters)
	if !ok {
		return
	}
	for _, p := range m.Policies(r) {
		match, best, ties := -1, len(specificity), 0
		for i, f := range filters {
			rank, ok := specificity[c.Filters[i].Level]
			if !ok || !pf.PolicyFilter(p, f) {
				continue
			}
			switch {
			case rank < best:
				match, best, ties = i, rank, 1
			case rank == best:
				ties++
			}
		}
		if ties == 1 {
			c.Filters[match].Contributions = append(c.Filters[match].Contributions, model.Contribution{Kind: model.ContributionPolicy, Ref: p.Policy, Field: p.Field})
		}
	}
}

// specificity ranks per-filter config levels, most specific first.
var specificity = map[model.ConfigLevel]int{model.LevelRoute: 0, model.LevelVirtualHost: 1, model.LevelRouteConfig: 2}

package chain

import (
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// Annotate returns a copy of snap with Route.Chain set on every route of every
// HCM. snap is not modified. When m is set, an entry lists the extension that
// defines its filter and, if m implements [origin.PolicyFilters] and the entry
// has per-filter config, the policy fields attributed to it on the route.
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
// the filter's extension first, then policies in m.Policies order.
//
// An extension contribution carries the mapper's filter origin certainty. A
// policy field is attributed only when exactly one entry with per-filter
// config for the route matches it, at any level or on a weighted backend. The
// link is at most [model.Inferred], and no stronger than the policy ref: the
// metadata names the field, not the filter, so with several candidates none gets it.
func attribute(c *model.EffectiveChain, filters []model.HTTPFilter, r model.Route, m origin.Mapper) {
	if m == nil {
		return
	}
	for i, f := range filters {
		if o := m.HTTPFilter(f); o.Certainty > model.Unknown && o.Extension != nil {
			c.Filters[i].Contributions = append(c.Filters[i].Contributions,
				model.Contribution{Kind: model.ContributionExtension, Ref: *o.Extension, Certainty: o.Certainty})
		}
	}
	pf, ok := m.(origin.PolicyFilters)
	if !ok {
		return
	}
	for _, p := range m.Policies(r) {
		match, n := -1, 0
		for i, f := range filters {
			if configured(c.Filters[i], r) && pf.PolicyFilter(p, f) {
				match, n = i, n+1
			}
		}
		if n == 1 {
			c.Filters[match].Contributions = append(c.Filters[match].Contributions,
				model.Contribution{Kind: model.ContributionPolicy, Ref: p.Policy, Field: p.Field, Certainty: min(p.Certainty, model.Inferred)})
		}
	}
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

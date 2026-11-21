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
// the filter's extension first, then policies in m.Policies order.
func attribute(c *model.EffectiveChain, filters []model.HTTPFilter, r model.Route, m origin.Mapper) {
	if m == nil {
		return
	}
	pf, _ := m.(origin.PolicyFilters)
	var policies []model.PolicyRef
	if pf != nil {
		policies = m.Policies(r)
	}
	for i, f := range filters {
		e := &c.Filters[i]
		if o := m.HTTPFilter(f); o.Certainty > model.Unknown && o.Extension != nil {
			e.Contributions = append(e.Contributions, model.Contribution{Kind: model.ContributionExtension, Ref: *o.Extension})
		}
		if e.Level == "" {
			continue
		}
		for _, p := range policies {
			if pf.PolicyFilter(p, f) {
				e.Contributions = append(e.Contributions, model.Contribution{Kind: model.ContributionPolicy, Ref: p.Policy, Field: p.Field})
			}
		}
	}
}

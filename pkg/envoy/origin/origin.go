// Package origin maps Envoy objects back to the Gateway API objects that
// produced them.
//
// A [Mapper] answers one question per Envoy object kind. Every answer carries a
// [model.Certainty]:
//   - Exact: read from metadata the control plane wrote.
//   - Inferred: parsed from a name, with one reading.
//   - Ambiguous: the name has several readings; all are returned.
//   - Unknown: not recognised.
//
// [Kgateway] is the mapper for kgateway OSS. Consumers can replace or extend it:
// [Chain] composes mappers, so a vendor plugin is a Mapper that recognises its
// own names and returns Unknown for the rest. [Annotate] writes a mapper's
// answers into a copy of a snapshot.
package origin

import "github.com/DuncanDoyle/krp/pkg/envoy/model"

// Mapper maps Envoy objects to the Gateway API objects that produced them.
// Implementations are pure and safe for concurrent use.
type Mapper interface {
	// Listener maps an Envoy listener to its Gateway listener.
	Listener(l model.Listener) model.ListenerOrigin
	// RouteConfig maps a route config or filter chain name to its Gateway listener.
	RouteConfig(name string) model.ListenerOrigin
	// VirtualHost maps a virtual host to its listener and hostname.
	VirtualHost(vh model.VirtualHost) model.VirtualHostOrigin
	// Route maps a route to the HTTPRoute rule and match it came from.
	Route(r model.Route) model.RouteAnnotation
	// Cluster maps a cluster name to its backend.
	Cluster(name string) model.BackendOrigin
	// HTTPFilter says what a filter is and which object configured it.
	HTTPFilter(f model.HTTPFilter) model.FilterOrigin
	// Policies lists the policies that contributed to a route.
	Policies(r model.Route) []model.PolicyRef
}

// Chain returns a Mapper that asks each mapper in order and returns the first
// answer better than Unknown. Policies come from the first mapper that returns any.
func Chain(mappers ...Mapper) Mapper { return chain(mappers) }

type chain []Mapper

func first[T any](c chain, ask func(Mapper) T, certainty func(T) model.Certainty) T {
	var zero T
	for _, m := range c {
		if v := ask(m); certainty(v) > model.Unknown {
			return v
		}
	}
	return zero
}

func (c chain) Listener(l model.Listener) model.ListenerOrigin {
	return first(c, func(m Mapper) model.ListenerOrigin { return m.Listener(l) },
		func(o model.ListenerOrigin) model.Certainty { return o.Certainty })
}

func (c chain) RouteConfig(name string) model.ListenerOrigin {
	return first(c, func(m Mapper) model.ListenerOrigin { return m.RouteConfig(name) },
		func(o model.ListenerOrigin) model.Certainty { return o.Certainty })
}

func (c chain) VirtualHost(vh model.VirtualHost) model.VirtualHostOrigin {
	return first(c, func(m Mapper) model.VirtualHostOrigin { return m.VirtualHost(vh) },
		func(o model.VirtualHostOrigin) model.Certainty { return o.Certainty })
}

func (c chain) Route(r model.Route) model.RouteAnnotation {
	return first(c, func(m Mapper) model.RouteAnnotation { return m.Route(r) },
		func(o model.RouteAnnotation) model.Certainty { return o.Certainty })
}

func (c chain) Cluster(name string) model.BackendOrigin {
	return first(c, func(m Mapper) model.BackendOrigin { return m.Cluster(name) },
		func(o model.BackendOrigin) model.Certainty { return o.Certainty })
}

func (c chain) HTTPFilter(f model.HTTPFilter) model.FilterOrigin {
	return first(c, func(m Mapper) model.FilterOrigin { return m.HTTPFilter(f) },
		func(o model.FilterOrigin) model.Certainty { return o.Certainty })
}

func (c chain) Policies(r model.Route) []model.PolicyRef {
	for _, m := range c {
		if p := m.Policies(r); len(p) > 0 {
			return p
		}
	}
	return nil
}

// ObjectForName maps a bare Envoy name, as found in proxy stats, to an object:
// a cluster to its backend, or a route config to its Gateway. It asks m only,
// and reports false unless the answer is Inferred or Exact.
func ObjectForName(m Mapper, name string) (model.Ref, bool) {
	if b := m.Cluster(name); b.Certainty >= model.Inferred {
		return b.Backend, true
	}
	if l := m.RouteConfig(name); l.Certainty >= model.Inferred && l.Gateway != nil {
		return *l.Gateway, true
	}
	return model.Ref{}, false
}

// Annotate returns a copy of snap with every listener, virtual host, route and
// HTTP filter annotated by m. snap is not modified. Annotations of Unknown
// certainty are left nil. Like the filter package, the copy shares the parts
// it doesn't annotate with snap; treat both as read-only.
func Annotate(snap *model.EnvoySnapshot, m Mapper) *model.EnvoySnapshot {
	out := &model.EnvoySnapshot{Listeners: make([]model.Listener, len(snap.Listeners))}
	for i, l := range snap.Listeners {
		l.Origin = known(m.Listener(l), func(o model.ListenerOrigin) model.Certainty { return o.Certainty })
		chains := make([]model.NetworkFilterChain, len(l.FilterChains))
		for j, fc := range l.FilterChains {
			if fc.HCM != nil {
				hcm := *fc.HCM
				hcm.HTTPFilters = annotateFilters(hcm.HTTPFilters, m)
				hcm.UpstreamFilters = annotateFilters(hcm.UpstreamFilters, m)
				if hcm.RouteConfig != nil {
					rc := *hcm.RouteConfig
					rc.VirtualHosts = annotateVirtualHosts(rc.VirtualHosts, m)
					hcm.RouteConfig = &rc
				}
				fc.HCM = &hcm
			}
			chains[j] = fc
		}
		l.FilterChains = chains
		out.Listeners[i] = l
	}
	return out
}

func annotateFilters(filters []model.HTTPFilter, m Mapper) []model.HTTPFilter {
	if filters == nil {
		return nil
	}
	out := make([]model.HTTPFilter, len(filters))
	for i, f := range filters {
		f.Origin = known(m.HTTPFilter(f), func(o model.FilterOrigin) model.Certainty { return o.Certainty })
		out[i] = f
	}
	return out
}

func annotateVirtualHosts(vhs []model.VirtualHost, m Mapper) []model.VirtualHost {
	out := make([]model.VirtualHost, len(vhs))
	for i, vh := range vhs {
		vh.Origin = known(m.VirtualHost(vh), func(o model.VirtualHostOrigin) model.Certainty { return o.Certainty })
		routes := make([]model.Route, len(vh.Routes))
		for j, r := range vh.Routes {
			r.Origin = known(m.Route(r), func(o model.RouteAnnotation) model.Certainty { return o.Certainty })
			r.Policies = m.Policies(r)
			routes[j] = r
		}
		vh.Routes = routes
		out[i] = vh
	}
	return out
}

// known returns &v, or nil when v is Unknown.
func known[T any](v T, certainty func(T) model.Certainty) *T {
	if certainty(v) == model.Unknown {
		return nil
	}
	return &v
}

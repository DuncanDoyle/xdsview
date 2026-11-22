package model

// EffectiveChain is the HTTP filter chain one route runs, resolved from the HCM
// filters and the per-filter config at route, virtual host and route config level.
type EffectiveChain struct {
	Filters     []ChainEntry `json:"filters"`               // every HCM filter, in HCM order; Runs marks the ones that run
	Conditional bool         `json:"conditional,omitempty"` // a backend's per-filter config may override (weighted clusters)
}

// ChainEntry is one HCM filter as a route sees it.
type ChainEntry struct {
	Name          string         `json:"name"`
	Type          string         `json:"type,omitempty"` // HTTPFilter.Type
	Runs          bool           `json:"runs"`
	State         FilterState    `json:"state"`
	Level         ConfigLevel    `json:"level,omitempty"`         // where the deciding per-filter config is; empty for the HCM default
	PerFilterType string         `json:"perFilterType,omitempty"` // @type of the deciding per-filter config
	Request       Side           `json:"request"`                 // has a request (decoder) side
	Response      Side           `json:"response"`                // has a response (encoder) side
	Contributions []Contribution `json:"contributions,omitempty"` // what configured this entry, when the mapper knows
}

// FilterState says why a filter runs or not on a route.
type FilterState string

// Filter states. Enabled and disabled mean per-filter config changed the HCM default.
const (
	FilterOn       FilterState = "on"       // enabled in the HCM, not disabled for the route
	FilterOff      FilterState = "off"      // disabled in the HCM, not enabled for the route
	FilterEnabled  FilterState = "enabled"  // disabled in the HCM, enabled for the route
	FilterDisabled FilterState = "disabled" // enabled in the HCM, disabled for the route
	FilterUnknown  FilterState = "unknown"  // the deciding per-filter config is one Envoy would reject
)

// ConfigLevel is where a per-filter config entry sits.
type ConfigLevel string

// Per-filter config levels, most specific first.
const (
	LevelRoute       ConfigLevel = "route"
	LevelVirtualHost ConfigLevel = "virtualHost"
	LevelRouteConfig ConfigLevel = "routeConfig"
)

// Side says whether a filter type handles one direction of a stream.
type Side string

// Sides. Unknown is for filter types xdsview has no record of.
const (
	SideYes     Side = "yes"
	SideNo      Side = "no"
	SideUnknown Side = "unknown"
)

// ContributionKind is the kind of object behind a chain entry.
type ContributionKind string

// Contribution kinds.
const (
	ContributionExtension ContributionKind = "extension" // defines the filter instance, e.g. a GatewayExtension
	ContributionPolicy    ContributionKind = "policy"    // configures the filter on this route
)

// Contribution is one object attributed to a chain entry: the extension that
// defines the filter instance, or a policy field attributed to it on this route.
//
// Certainty is the mapper's [FilterOrigin] certainty for an extension. A policy
// contribution is at most [Inferred]: the route metadata records the field, and
// the link to this one entry comes from the mapper's field-to-filter table.
type Contribution struct {
	Kind      ContributionKind `json:"kind"`
	Ref       Ref              `json:"ref"`
	Field     string           `json:"field,omitempty"` // policy field, e.g. "extAuth"
	Certainty Certainty        `json:"certainty"`
}

// RequestOrder returns the running entries that handle requests, in HCM order.
// Entries with an unknown request side are kept.
func (c EffectiveChain) RequestOrder() []ChainEntry {
	var out []ChainEntry
	for _, e := range c.Filters {
		if e.Runs && e.Request != SideNo {
			out = append(out, e)
		}
	}
	return out
}

// ResponseOrder returns the running entries that handle responses, in the
// reverse of HCM order, which is the order Envoy runs encoders in. Entries with
// an unknown response side are kept.
func (c EffectiveChain) ResponseOrder() []ChainEntry {
	var out []ChainEntry
	for i := len(c.Filters) - 1; i >= 0; i-- {
		if e := c.Filters[i]; e.Runs && e.Response != SideNo {
			out = append(out, e)
		}
	}
	return out
}

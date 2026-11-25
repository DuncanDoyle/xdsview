package model

// EffectiveChain is the HTTP filter chain one route runs, resolved from the HCM
// filters and the per-filter config at route, virtual host and route config level.
type EffectiveChain struct {
	Filters     []ChainEntry `json:"filters"`               // every HCM filter, in HCM order; Runs marks the ones that run
	Conditional bool         `json:"conditional,omitempty"` // a backend's per-filter config may override (weighted clusters)
}

// ChainEntry is one HCM filter slot as a route sees it.
type ChainEntry struct {
	Slot          ID          `json:"slot"` // the HTTPFilter's ID
	Name          string      `json:"name"`
	Type          string      `json:"type,omitempty"` // HTTPFilter.Type
	Runs          Tri         `json:"runs"`           // unknown when the deciding per-filter config is one Envoy would reject
	State         FilterState `json:"state"`
	Level         ConfigLevel `json:"level,omitempty"`         // where the deciding per-filter config is; empty for the HCM default
	PerFilterType string      `json:"perFilterType,omitempty"` // @type of the deciding per-filter config
	Request       Tri         `json:"request"`                 // has a request (decoder) side
	Response      Tri         `json:"response"`                // has a response (encoder) side
	Wrapper       string      `json:"wrapper,omitempty"`       // WrapperComposite or WrapperExtensionWithMatcher; empty otherwise
	Conditional   bool        `json:"conditional,omitempty"`   // a matcher can pick a delegate or skip the filter
	ConfigSource  string      `json:"configSource,omitempty"`  // ConfigInline, ConfigECDS or ConfigECDSDefault; empty when unresolved
}

// Tri is a yes, no or unknown answer. Unknown means the dump doesn't establish it.
type Tri string

// Tri values.
const (
	TriYes     Tri = "yes"
	TriNo      Tri = "no"
	TriUnknown Tri = "unknown"
)

// Filter wrappers: the installed filter decides at runtime what runs inside it.
const (
	WrapperComposite            = "composite"            // a Composite filter, which runs the delegate a match picks
	WrapperExtensionWithMatcher = "extensionWithMatcher" // an ExtensionWithMatcher, which skips its inner filter unless a match allows it
)

// Config sources: where a filter slot's config comes from.
const (
	ConfigInline      = "inline"      // the HCM's http_filters entry
	ConfigECDS        = "ecds"        // an extension config discovery entry
	ConfigECDSDefault = "ecdsDefault" // the default_config of a filter with config_discovery
)

// FilterState says why a filter runs or not on a route.
type FilterState string

// Filter states. Enabled and disabled mean per-filter config changed the HCM default.
const (
	FilterOn       FilterState = "on"       // enabled in the HCM, not disabled for the route
	FilterOff      FilterState = "off"      // disabled in the HCM, not enabled for the route
	FilterEnabled  FilterState = "enabled"  // disabled in the HCM, enabled for the route
	FilterDisabled FilterState = "disabled" // enabled in the HCM, disabled for the route
	FilterUnknown  FilterState = "unknown"  // the deciding per-filter config is one Envoy would reject
	// FilterUnresolved: the filter takes its config from ECDS and the dump
	// has neither a matching config nor a default in use.
	FilterUnresolved FilterState = "unresolved"
)

// ConfigLevel is where a per-filter config entry sits.
type ConfigLevel string

// Per-filter config levels, most specific first.
const (
	LevelRoute       ConfigLevel = "route"
	LevelVirtualHost ConfigLevel = "virtualHost"
	LevelRouteConfig ConfigLevel = "routeConfig"
)

// ContributionKind is the kind of object behind a chain entry.
type ContributionKind string

// Contribution kinds.
const (
	ContributionExtension ContributionKind = "extension" // defines the filter instance, e.g. a GatewayExtension
	ContributionPolicy    ContributionKind = "policy"    // configures the filter on this route
)

// Contribution is one object attributed to a chain entry: the extension that
// defines the filter instance, or a policy field attributed to it on one route.
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

// RequestOrder returns the entries that may run and handle requests, in HCM
// order. Entries whose run state or request side is unknown are kept.
func (c EffectiveChain) RequestOrder() []ChainEntry {
	var out []ChainEntry
	for _, e := range c.Filters {
		if e.Runs != TriNo && e.Request != TriNo {
			out = append(out, e)
		}
	}
	return out
}

// ResponseOrder returns the entries that may run and handle responses, in the
// reverse of HCM order, which is the order Envoy runs encoders in. Entries
// whose run state or response side is unknown are kept.
func (c EffectiveChain) ResponseOrder() []ChainEntry {
	var out []ChainEntry
	for i := len(c.Filters) - 1; i >= 0; i-- {
		if e := c.Filters[i]; e.Runs != TriNo && e.Response != TriNo {
			out = append(out, e)
		}
	}
	return out
}

// Package model defines the domain types that represent an Envoy configuration
// snapshot. The types are intentionally decoupled from the raw Envoy JSON format
// so that the renderer and future interactive phases are not tied to Envoy's
// wire-format field names.
//
// Data flows: parser → model → renderer.
package model

// Snapshot is the parsed Envoy configuration of one config dump. Listeners
// join their route configs from RouteConfigs. Every occurrence in it carries
// an [ID] that is unique in the snapshot.
type Snapshot struct {
	Capture      Capture             `json:"capture"`
	Listeners    []Listener          `json:"listeners"`
	RouteConfigs []RouteConfigSource `json:"routeConfigs,omitempty"` // every route config source, in dump order
	Warnings     []Warning           `json:"warnings,omitempty"`
}

// Listener represents an Envoy listener (e.g. listener~80, listener~443).
// An LDS-wide error with no listener name is a rejected Listener with an empty Name.
type Listener struct {
	ID           ID                   `json:"id"`
	Name         string               `json:"name"`
	Address      string               `json:"address"` // e.g. "0.0.0.0:80"
	FilterChains []NetworkFilterChain `json:"filterChains"`
	State        ListenerState        `json:"state"`
	Static       bool                 `json:"static,omitempty"` // a static_listeners entry (bootstrap); always active
	Origin       *ListenerOrigin      `json:"origin,omitempty"` // set by origin.Annotate
}

// ListenerStatus is the version of a listener the snapshot shows.
type ListenerStatus string

// Listener statuses. The shown version is active, else warming, else draining;
// a listener with only an error_state is rejected.
const (
	ListenerActive   ListenerStatus = "active"
	ListenerWarming  ListenerStatus = "warming"
	ListenerDraining ListenerStatus = "draining"
	ListenerRejected ListenerStatus = "rejected"
)

// ListenerState is a listener's lifecycle state from its dynamic_listeners entry.
type ListenerState struct {
	Status      ListenerStatus `json:"status"`
	Updating    bool           `json:"updating,omitempty"`    // active, with a newer version warming
	Draining    bool           `json:"draining,omitempty"`    // active, with an older version draining
	LastUpdated string         `json:"lastUpdated,omitempty"` // of the version shown, as Envoy writes it
	Error       *ListenerError `json:"error,omitempty"`       // the last rejected update
}

// ListenerError is a listener's error_state: Envoy rejected its last update.
type ListenerError struct {
	Details     string `json:"details"`
	LastAttempt string `json:"lastAttempt,omitempty"`
}

// NetworkFilterChain is a network-level filter chain within a listener.
// For HTTPS listeners, there is typically one filter chain per SNI host.
type NetworkFilterChain struct {
	ID   ID          `json:"id"`
	Name string      `json:"name"`
	TLS  *TLSContext `json:"tls,omitempty"` // nil for plaintext
	HCM  *HCMConfig  `json:"hcm,omitempty"` // extracted from the network filter list
}

// TLSContext holds TLS/SNI information for a filter chain.
type TLSContext struct {
	SNIHosts []string `json:"sniHosts"` // from filter_chain_match.server_names
}

// HCMConfig represents the HTTP Connection Manager configuration. Its ID is
// that of the network filter it is.
type HCMConfig struct {
	ID              ID             `json:"id"`
	RouteConfigName string         `json:"routeConfigName"`           // rds.route_config_name
	HTTPFilters     []HTTPFilter   `json:"httpFilters"`               // the HTTP filter pipeline
	UpstreamFilters []HTTPFilter   `json:"upstreamFilters,omitempty"` // router upstream_http_filters, run per upstream request
	RouteConfigUse  RouteConfigUse `json:"routeConfigUse"`
	RouteConfig     *RouteConfig   `json:"routeConfig,omitempty"` // the use's content, copied from its source; nil unless found
}

// HTTPFilter is a single filter in the HTTP filter pipeline.
type HTTPFilter struct {
	ID          ID             `json:"id"`                    // the filter slot
	Name        string         `json:"name"`                  // label chosen by the control plane, e.g. "ext_auth/ns/name"
	Type        string         `json:"type,omitempty"`        // typed_config @type; the inner type for an ExtensionWithMatcher
	TypedConfig map[string]any `json:"typedConfig,omitempty"` // raw typed config; displayed in interactive mode (Phase 3)
	Disabled    bool           `json:"disabled,omitempty"`    // filter disabled at HCM level, enabled per-route
	Origin      *FilterOrigin  `json:"origin,omitempty"`      // set by origin.Annotate
}

// RouteConfig is an Envoy route configuration (from RDS).
type RouteConfig struct {
	Name                 string         `json:"name"`
	VirtualHosts         []VirtualHost  `json:"virtualHosts"`
	TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` // least specific per-filter config
}

// VirtualHost is an Envoy virtual host within a route config.
type VirtualHost struct {
	ID                   ID                 `json:"id"`
	Name                 string             `json:"name"`    // e.g. "listener~80~api_example_com"
	Domains              []string           `json:"domains"` // e.g. ["api.example.com"]
	Routes               []Route            `json:"routes"`
	TypedPerFilterConfig map[string]any     `json:"typedPerFilterConfig,omitempty"` // overrides the route config's
	Origin               *VirtualHostOrigin `json:"origin,omitempty"`               // set by origin.Annotate
}

// Route is an Envoy route within a virtual host.
type Route struct {
	ID                      ID                `json:"id"`
	Name                    string            `json:"name"`
	Match                   RouteMatch        `json:"match"`
	Cluster                 string            `json:"cluster"`                           // backend cluster name; empty for a traffic split
	WeightedClusters        []WeightedCluster `json:"weightedClusters,omitempty"`        // weighted_clusters (traffic split)
	Rewrite                 *RouteRewrite     `json:"rewrite,omitempty"`                 // path rewrite (URLRewrite HTTPRouteFilter)
	MirrorClusters          []string          `json:"mirrorClusters,omitempty"`          // request_mirror_policies cluster names
	RequestHeadersToAdd     []HeaderOperation `json:"requestHeadersToAdd,omitempty"`     // request_headers_to_add
	ResponseHeadersToAdd    []HeaderOperation `json:"responseHeadersToAdd,omitempty"`    // response_headers_to_add
	ResponseHeadersToRemove []string          `json:"responseHeadersToRemove,omitempty"` // response_headers_to_remove
	TypedPerFilterConfig    map[string]any    `json:"typedPerFilterConfig,omitempty"`    // per-route filter config (Phase 2)
	Metadata                map[string]any    `json:"metadata,omitempty"`                // filter_metadata (Phase 4)
	Origin                  *RouteAnnotation  `json:"origin,omitempty"`                  // set by origin.Annotate
	Policies                []PolicyRef       `json:"policies,omitempty"`                // set by origin.Annotate
	Chain                   *EffectiveChain   `json:"effectiveChain,omitempty"`          // a copy of chain.Effective's result, set by callers that embed it; the core doesn't read it
}

// RouteRewrite captures path rewrite configuration on a route action.
// Kgateway expresses URLRewrite HTTPRouteFilters as a regex_rewrite in the Envoy route action.
type RouteRewrite struct {
	RegexPattern string `json:"regexPattern"`     // regex_rewrite.pattern.regex
	Substitution string `json:"substitution"`     // regex_rewrite.substitution
	Prefix       string `json:"prefix,omitempty"` // prefix_rewrite
}

// WeightedCluster is one backend of a traffic-split route.
type WeightedCluster struct {
	ID                   ID             `json:"id"`
	Name                 string         `json:"name"`
	Weight               int            `json:"weight"`
	TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` // overrides the route's when Envoy picks this backend
}

// HeaderOperation is a key/value header to add to a request or response.
type HeaderOperation struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RouteMatch describes what traffic a route matches.
type RouteMatch struct {
	Prefix              string            `json:"prefix,omitempty"`
	PathSeparatedPrefix string            `json:"pathSeparatedPrefix,omitempty"` // path_separated_prefix in Envoy
	Path                string            `json:"path,omitempty"`
	Regex               string            `json:"regex,omitempty"` // safe_regex in Envoy
	Headers             []HeaderMatch     `json:"headers,omitempty"`
	QueryParams         []QueryParamMatch `json:"queryParams,omitempty"`
}

// HeaderMatch is a header-based match condition on a route.
type HeaderMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Regex bool   `json:"regex,omitempty"` // true when value is a safe_regex pattern
}

// QueryParamMatch is a query parameter match condition on a route.
type QueryParamMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Regex bool   `json:"regex,omitempty"` // true when value is a safe_regex pattern
}

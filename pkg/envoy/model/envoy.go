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
	RouteConfigs []RouteConfigSource `json:"routeConfigs,omitempty"` // every route config source, in section order; static before dynamic in a section
	Coverage     Coverage            `json:"coverage"`
	Warnings     []Warning           `json:"warnings,omitempty"`
}

// Coverage says what the parser did with each config dump section.
type Coverage struct {
	Sections []SectionCoverage `json:"sections,omitempty"` // in dump order
}

// SectionCoverage is one config dump section and what the parser did with it.
type SectionCoverage struct {
	Type   string         `json:"type"` // the section's @type; empty when unreadable
	Status CoverageStatus `json:"status"`
}

// CoverageStatus is what the parser did with a section.
type CoverageStatus string

// Coverage statuses.
const (
	CoverageParsed  CoverageStatus = "parsed"
	CoveragePartial CoverageStatus = "partial" // parsed, but entries were skipped; the warnings say which
	CoverageIgnored CoverageStatus = "ignored" // not read: clusters, endpoints, secrets, scoped routes, bootstrap and unknown types
)

// Listener represents an Envoy listener (e.g. listener~80, listener~443).
// An LDS-wide error with no listener name is a rejected Listener with an empty Name.
type Listener struct {
	ID           ID                   `json:"id"`
	Name         string               `json:"name"`
	Address      string               `json:"address"` // e.g. "0.0.0.0:80"
	FilterChains []NetworkFilterChain `json:"filterChains"`
	State        ListenerState        `json:"state"`
	Static       bool                 `json:"static,omitempty"`   // a static_listeners entry (bootstrap); status active, though Envoy also dumps it before its workers start
	Metadata     map[string]any       `json:"metadata,omitempty"` // filter_metadata of the version shown
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
	ID             ID              `json:"id"`
	Name           string          `json:"name"`
	TLS            *TLSContext     `json:"tls,omitempty"`            // nil for plaintext
	Default        bool            `json:"default,omitempty"`        // the listener's default_filter_chain, used when no chain matches; always last
	NetworkFilters []NetworkFilter `json:"networkFilters,omitempty"` // every network filter, in chain order
	HCM            *HCMConfig      `json:"hcm,omitempty"`            // extracted from the network filter list
}

// NetworkFilter is one entry of a filter chain's network filter list. An HCM
// has the ID of the network filter it is.
type NetworkFilter struct {
	ID   ID     `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // typed_config @type
	// Discovery is set when the config comes from ECDS. The parser doesn't
	// resolve it: Source stays empty, and whether the chain has an HCM is unknown.
	Discovery *Discovery `json:"discovery,omitempty"`
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
	ScopedRoutes    bool           `json:"scopedRoutes,omitempty"`    // routes come from scoped_routes, which the parser doesn't read; the use is unknown
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
	Discovery   *Discovery     `json:"discovery,omitempty"`   // set when the config comes from ECDS; TypedConfig is then the resolved config
}

// Discovery is a filter's config_discovery: its config comes from extension
// config discovery (ECDS), not from the filter entry.
type Discovery struct {
	TypeURLs                   []string `json:"typeUrls"`                             // the types an ECDS config may have
	DefaultConfig              bool     `json:"defaultConfig,omitempty"`              // a default_config is set
	ApplyDefaultWithoutWarming bool     `json:"applyDefaultWithoutWarming,omitempty"` // the default applies before the first ECDS response
	Source                     string   `json:"source,omitempty"`                     // ConfigECDS or ConfigECDSDefault; empty when unresolved
}

// RouteConfig is an Envoy route configuration (from RDS).
type RouteConfig struct {
	Name                 string         `json:"name"`
	VirtualHosts         []VirtualHost  `json:"virtualHosts"`
	TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` // least specific per-filter config
}

// VirtualHost is an Envoy virtual host within a route config.
type VirtualHost struct {
	ID ID `json:"id"`
	// SourcePath is the virtual host's place in its route config source, e.g.
	// "RD3/V2". It is equal across the uses of a shared source; it is not an occurrence ID.
	SourcePath           ID             `json:"sourcePath,omitempty"`
	Name                 string         `json:"name"`    // e.g. "listener~80~api_example_com"
	Domains              []string       `json:"domains"` // e.g. ["api.example.com"]
	Routes               []Route        `json:"routes"`
	TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` // overrides the route config's
	Metadata             map[string]any `json:"metadata,omitempty"`             // filter_metadata
}

// Route is an Envoy route within a virtual host.
type Route struct {
	ID                      ID                `json:"id"`
	SourcePath              ID                `json:"sourcePath,omitempty"` // e.g. "RD3/V2/R5"; see VirtualHost.SourcePath
	Name                    string            `json:"name"`
	Match                   RouteMatch        `json:"match"`
	Action                  RouteAction       `json:"action"`
	Cluster                 string            `json:"cluster"`                           // backend cluster name; empty for a traffic split or another selector in Action
	WeightedClusters        []WeightedCluster `json:"weightedClusters,omitempty"`        // weighted_clusters (traffic split)
	Rewrite                 *RouteRewrite     `json:"rewrite,omitempty"`                 // path rewrite (URLRewrite HTTPRouteFilter)
	MirrorClusters          []string          `json:"mirrorClusters,omitempty"`          // request_mirror_policies cluster names
	RequestHeadersToAdd     []HeaderOperation `json:"requestHeadersToAdd,omitempty"`     // request_headers_to_add
	ResponseHeadersToAdd    []HeaderOperation `json:"responseHeadersToAdd,omitempty"`    // response_headers_to_add
	ResponseHeadersToRemove []string          `json:"responseHeadersToRemove,omitempty"` // response_headers_to_remove
	TypedPerFilterConfig    map[string]any    `json:"typedPerFilterConfig,omitempty"`    // per-route filter config (Phase 2)
	Metadata                map[string]any    `json:"metadata,omitempty"`                // filter_metadata (Phase 4)
}

// RouteAction is what a route does with a request it matches: the route's
// action oneof (route_components.proto, Route.action).
type RouteAction struct {
	Kind ActionKind `json:"kind,omitempty"` // empty when the route sets no action
	// Cluster selectors other than a name or weights, for Kind route
	// (RouteAction.cluster_specifier). Route.Cluster is empty with any of them:
	// the cluster is chosen per request.
	ClusterHeader                string     `json:"clusterHeader,omitempty"`                // the request header naming the cluster
	ClusterSpecifierPlugin       string     `json:"clusterSpecifierPlugin,omitempty"`       // a plugin the route config defines, by name
	InlineClusterSpecifierPlugin *Extension `json:"inlineClusterSpecifierPlugin,omitempty"` // a plugin defined on the route

	Redirect         *Redirect       `json:"redirect,omitempty"`         // for Kind redirect
	DirectResponse   *DirectResponse `json:"directResponse,omitempty"`   // for Kind directResponse
	FilterActionType string          `json:"filterActionType,omitempty"` // for Kind filterAction: the action's @type
}

// ActionKind is the kind of a route's action.
type ActionKind string

// Action kinds.
const (
	ActionRoute          ActionKind = "route"          // forward to a cluster
	ActionRedirect       ActionKind = "redirect"       // answer with a redirect
	ActionDirectResponse ActionKind = "directResponse" // answer directly
	ActionFilter         ActionKind = "filterAction"   // a filter-defined action; not implemented in Envoy
	ActionNonForwarding  ActionKind = "nonForwarding"  // answered without an upstream; for non-proxy xDS clients
)

// Redirect is a redirect action (route_components.proto, RedirectAction).
type Redirect struct {
	ResponseCode int    `json:"responseCode"`     // 301 unless the dump says otherwise
	Scheme       string `json:"scheme,omitempty"` // "https" for https_redirect
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	// At most one path rewrite is set.
	Path              string `json:"path,omitempty"`              // path_redirect
	PrefixRewrite     string `json:"prefixRewrite,omitempty"`     // prefix_rewrite
	RegexPattern      string `json:"regexPattern,omitempty"`      // regex_rewrite.pattern.regex
	RegexSubstitution string `json:"regexSubstitution,omitempty"` // regex_rewrite.substitution
	PathRewrite       string `json:"pathRewrite,omitempty"`       // path_rewrite, a substitution format string
	StripQuery        bool   `json:"stripQuery,omitempty"`
}

// DirectResponse is a direct response action. It keeps the body's size and
// source, never the body.
type DirectResponse struct {
	Status     int    `json:"status"`
	BodySource string `json:"bodySource,omitempty"` // "inline", "file" or "env"; empty without a body
	BodySize   *int   `json:"bodySize,omitempty"`   // bytes of an inline body
	BodyFormat bool   `json:"bodyFormat,omitempty"` // body_format sets how the body is written
}

// Body sources of a direct response (core.v3.DataSource).
const (
	BodyInline = "inline"
	BodyFile   = "file"
	BodyEnv    = "env"
)

// Extension names a typed extension: its name and its typed_config @type.
type Extension struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
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
	Regex               string            `json:"regex,omitempty"`         // safe_regex in Envoy
	Connect             bool              `json:"connect,omitempty"`       // connect_matcher: CONNECT requests
	CaseSensitive       *bool             `json:"caseSensitive,omitempty"` // case_sensitive as the dump sets it; Envoy's default is true
	Headers             []HeaderMatch     `json:"headers,omitempty"`
	QueryParams         []QueryParamMatch `json:"queryParams,omitempty"`
	// Unparsed lists the match fields the parser doesn't model, by their
	// Envoy name (e.g. "grpc", "runtime_fraction"). The route matches less
	// than the other fields say.
	Unparsed []string `json:"unparsed,omitempty"`
}

// MatchKind is how a header or query parameter matcher compares.
type MatchKind string

// Match kinds (route_components.proto HeaderMatcher and QueryParameterMatcher,
// string.proto StringMatcher).
const (
	MatchExact    MatchKind = "exact"
	MatchPrefix   MatchKind = "prefix"
	MatchSuffix   MatchKind = "suffix"
	MatchContains MatchKind = "contains"
	MatchRegex    MatchKind = "regex"
	MatchPresent  MatchKind = "present" // also a matcher with no match specifier
	MatchAbsent   MatchKind = "absent"  // present_match: false
	MatchRange    MatchKind = "range"   // Value is "[start,end)"
	MatchCustom   MatchKind = "custom"  // a custom StringMatcher extension; Value is its type
	// MatchAnyValue is a legacy top-level exact_match of "": a present header
	// matches whatever its value (header_utility.h, HeaderDataExactMatch).
	MatchAnyValue MatchKind = "anyValue"
)

// HeaderMatch is a header-based match condition on a route.
type HeaderMatch struct {
	Name           string    `json:"name"`
	Value          string    `json:"value"`
	Regex          bool      `json:"regex,omitempty"` // true when Kind is MatchRegex
	Kind           MatchKind `json:"kind"`
	IgnoreCase     bool      `json:"ignoreCase,omitempty"`
	Invert         bool      `json:"invert,omitempty"`         // invert_match
	MissingAsEmpty bool      `json:"missingAsEmpty,omitempty"` // treat_missing_header_as_empty
}

// QueryParamMatch is a query parameter match condition on a route.
type QueryParamMatch struct {
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	Regex      bool      `json:"regex,omitempty"` // true when Kind is MatchRegex
	Kind       MatchKind `json:"kind"`
	IgnoreCase bool      `json:"ignoreCase,omitempty"`
}

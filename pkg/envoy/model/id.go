package model

import "strings"

// ID names one occurrence in one [Snapshot]. It is opaque and valid only in
// the snapshot that produced it: IDs shift when a sibling is added, so they
// never join two dumps. Compare IDs and call [ID.Within]; don't parse them.
type ID string

// Within reports whether id is block or lies inside it. An empty block
// contains nothing.
func (id ID) Within(block ID) bool {
	if block == "" {
		return false
	}
	return id == block || strings.HasPrefix(string(id), string(block)+"/")
}

// Capture says whether a snapshot is known to hold the whole config dump.
type Capture string

// Captures. A dump read from a file may have been filtered, so it is unverified
// unless the caller states otherwise.
const (
	CaptureUnverified Capture = "unverified"
	CaptureComplete   Capture = "complete" // fetched from /config_dump with no filter or mask
)

// Warning is a non-fatal problem found while parsing a dump.
type Warning struct {
	Subject ID     `json:"subject,omitempty"` // empty for dump-wide warnings
	Code    string `json:"code"`              // one of the Warn* codes
	Message string `json:"message"`
}

// Warning codes.
const (
	WarnSectionSkipped     = "section-skipped"      // a config dump section could not be read
	WarnRouteConfigSkipped = "route-config-skipped" // a route config entry could not be read
	WarnHCMSkipped         = "hcm-skipped"          // an HTTP connection manager could not be read
	WarnRDSAmbiguous       = "rds-ambiguous"        // several dynamic route configs share the RDS name
	WarnRDSMissing         = "rds-missing"          // an active HCM's RDS name has no route config in a complete capture
	WarnECDSAmbiguous      = "ecds-ambiguous"       // several ECDS configs match a filter's name and types
	WarnECDSSkipped        = "ecds-skipped"         // an ECDS config entry could not be read

	// Provenance warnings, on a composed index rather than a snapshot.
	WarnClaimDropped = "claim-dropped" // a module's claim broke a composition rule
	WarnLinkDropped  = "link-dropped"  // a filter link could not be resolved
)

// RouteConfigSourceKind says where in the dump a route configuration sits.
type RouteConfigSourceKind string

// Route config source kinds.
const (
	SourceDynamic RouteConfigSourceKind = "dynamic" // a dynamic_route_configs entry (RDS)
	SourceStatic  RouteConfigSourceKind = "static"  // a static_route_configs entry
	SourceInline  RouteConfigSourceKind = "inline"  // an HCM's inline route_config
)

// RouteConfigSource is one route configuration as the dump holds it. Its
// virtual hosts and routes appear under each [RouteConfigUse] of it.
type RouteConfigSource struct {
	ID   ID                    `json:"id"`
	Kind RouteConfigSourceKind `json:"kind"`
	Name string                `json:"name"`
	// Reported is the static entry Envoy lists for an inline config, when one
	// has the same name and content. That entry is itself a source, so the ID
	// resolves in the snapshot.
	Reported ID `json:"reported,omitempty"`
	// Route-configuration-level data, kept even for a source with no use:
	// filter_metadata, and the per-filter config each use also copies.
	Metadata             map[string]any `json:"metadata,omitempty"`
	TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"`
}

// RouteConfigState says whether an HCM's route config use joined a source.
type RouteConfigState string

// Route config states.
const (
	RouteConfigFound     RouteConfigState = "found"
	RouteConfigMissing   RouteConfigState = "missing"   // no source has the RDS name, in a complete capture
	RouteConfigAmbiguous RouteConfigState = "ambiguous" // several dynamic sources have the RDS name
	RouteConfigUnknown   RouteConfigState = "unknown"   // not established from this dump
)

// RouteConfigUse is an HCM's use of a route configuration. Source is empty
// unless State is found.
type RouteConfigUse struct {
	ID     ID               `json:"id"`
	Source ID               `json:"source,omitempty"`
	State  RouteConfigState `json:"state"`
}

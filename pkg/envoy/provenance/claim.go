// Package provenance is the contract between the Envoy core and product
// modules. A [Module] reads a [model.Snapshot] and returns [Claim]s about
// where its occurrences came from; [Compose] checks and joins the claims of
// several modules into an [Index]; consumers read the index by occurrence ID.
//
// Execution (what Envoy runs) never depends on claims: [Compose] gives each
// module its own copy of the snapshot, and the core packages don't import
// this one.
package provenance

import (
	"encoding/json"
	"fmt"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Claim is one assertion a module makes about one occurrence.
type Claim struct {
	ID        string          `json:"id"`            // set by Compose: "<module>/<n>", n the emission index
	Ref       string          `json:"ref,omitempty"` // module-local name, unique in the module; a FilterLink refers to it
	Subject   model.ID        `json:"subject"`
	Assertion Assertion       `json:"assertion"`
	Certainty model.Certainty `json:"certainty"`
	Evidence  []Evidence      `json:"evidence"`
	Module    string          `json:"module"` // set by Compose
}

// EvidenceKind says what kind of input a claim rests on.
type EvidenceKind string

// Evidence kinds.
const (
	EvidenceMetadata  EvidenceKind = "metadata"  // metadata the control plane wrote
	EvidenceName      EvidenceKind = "name"      // a naming convention
	EvidenceTable     EvidenceKind = "table"     // a module's version-keyed table
	EvidenceStructure EvidenceKind = "structure" // the shape of the config, e.g. a filter's type
)

// Evidence is one input a claim rests on.
type Evidence struct {
	Kind    EvidenceKind `json:"kind"`
	Subject model.ID     `json:"subject"`          // the occurrence it was read from
	Field   string       `json:"field"`            // e.g. "metadata.filter_metadata.merge.TrafficPolicy.gateway.kgateway.dev"
	Object  *model.Ref   `json:"object,omitempty"` // the Kubernetes object the evidence names, if any
}

// Assertion is what a claim asserts. It is sealed: the kinds are this
// package's, and new kinds are added here.
type Assertion interface {
	// Kind names the assertion kind, e.g. "routeSource". JSON carries it.
	Kind() string
	spec() kindSpec
	objects() []model.Ref
}

// subjectKind is the kind of occurrence an ID names.
type subjectKind int

const (
	subjectNone subjectKind = iota
	subjectListener
	subjectFilterChain
	subjectHCM
	subjectUse
	subjectSource
	subjectVirtualHost
	subjectRoute
	subjectBackend // a weighted backend
	subjectSlot    // an HCM HTTP filter slot
	subjectUpstreamSlot
)

// kindSpec is the composition rule of an assertion kind (design §5 table).
type kindSpec struct {
	subjects    []subjectKind
	cap         model.Certainty
	cumulative  bool // a list of facts; otherwise alternatives that compete
	unmarshaler func([]byte) (Assertion, error)
}

// Assertion kind names.
const (
	KindListenerSource  = "listenerSource"
	KindGatewaySource   = "gatewaySource"
	KindHostSource      = "hostSource"
	KindRouteSource     = "routeSource"
	KindExtensionSource = "extensionSource"
	KindBackendSource   = "backendSource"
	KindPolicyRecord    = "policyRecord"
	KindFilterLink      = "filterLink"
)

// ListenerSource is the Gateway listener an occurrence came from, read from a
// name: a port, or a listener (section) name.
type ListenerSource struct {
	Port    int    `json:"port,omitempty"`
	Section string `json:"section,omitempty"`
}

// GatewaySource is the Gateway a listener or route config came from. Modules
// claim it from metadata only, never from caller context.
type GatewaySource struct {
	Gateway model.Ref `json:"gateway"`
}

// HostSource is the hostname a virtual host serves.
type HostSource struct {
	Hostname string `json:"hostname"` // "*" when routes have no hostname
}

// RouteSource is the route rule and match an Envoy route came from.
// IndexCertainty is [model.Inferred] when the indices or rule name came from
// a name rather than metadata; unset when they share the claim's certainty.
type RouteSource struct {
	Route          model.Ref       `json:"route"`
	RuleIndex      int             `json:"ruleIndex"`          // -1 when unknown
	RuleName       string          `json:"ruleName,omitempty"` // empty when the rule has no name
	MatchIndex     int             `json:"matchIndex"`         // -1 when unknown
	IndexCertainty model.Certainty `json:"indexCertainty,omitempty"`
}

// ExtensionSource says what an HTTP filter slot is and which object defines it.
type ExtensionSource struct {
	Extension  *model.Ref `json:"extension,omitempty"`  // the object that defines the filter, e.g. a GatewayExtension
	FilterKind string     `json:"filterKind,omitempty"` // Envoy filter kind from its type, e.g. "ext_authz"
	Helper     bool       `json:"helper,omitempty"`     // control-plane plumbing, not a policy
}

// BackendSource is the backend object and port behind a route's or weighted
// backend's cluster.
type BackendSource struct {
	Backend model.Ref `json:"backend"`
	Port    int       `json:"port,omitempty"`
}

// Level is where a policy record sits.
type Level string

// Policy record levels.
const (
	LevelListener    Level = "listener"
	LevelRouteConfig Level = "routeConfig"
	LevelVirtualHost Level = "virtualHost"
	LevelRoute       Level = "route"
)

// PolicyRecord is a policy the dump records at one level, and the field it
// set. It is level-local: nothing says which level governs.
type PolicyRecord struct {
	Policy model.Ref `json:"policy"`
	Field  string    `json:"field,omitempty"` // e.g. "extAuth"
	Level  Level     `json:"level"`
}

// FilterLink says that a policy record configures one HTTP filter slot on
// the claim's route. Record is the module-local Ref of the record's claim.
type FilterLink struct {
	Record string   `json:"record"`
	Slot   model.ID `json:"slot"`
}

var (
	allAnchors   = []subjectKind{subjectListener, subjectFilterChain, subjectSource, subjectVirtualHost}
	policyLevels = map[subjectKind]Level{subjectListener: LevelListener, subjectSource: LevelRouteConfig, subjectVirtualHost: LevelVirtualHost, subjectRoute: LevelRoute}
)

var specs = map[string]kindSpec{
	KindListenerSource:  {subjects: allAnchors, cap: model.Exact, unmarshaler: decode[ListenerSource]},
	KindGatewaySource:   {subjects: []subjectKind{subjectListener, subjectSource}, cap: model.Exact, unmarshaler: decode[GatewaySource]},
	KindHostSource:      {subjects: []subjectKind{subjectVirtualHost}, cap: model.Exact, unmarshaler: decode[HostSource]},
	KindRouteSource:     {subjects: []subjectKind{subjectRoute}, cap: model.Exact, unmarshaler: decode[RouteSource]},
	KindExtensionSource: {subjects: []subjectKind{subjectSlot, subjectUpstreamSlot}, cap: model.Exact, unmarshaler: decode[ExtensionSource]},
	KindBackendSource:   {subjects: []subjectKind{subjectRoute, subjectBackend}, cap: model.Exact, unmarshaler: decode[BackendSource]},
	KindPolicyRecord:    {subjects: []subjectKind{subjectListener, subjectSource, subjectVirtualHost, subjectRoute}, cap: model.Exact, cumulative: true, unmarshaler: decode[PolicyRecord]},
	KindFilterLink:      {subjects: []subjectKind{subjectRoute}, cap: model.Inferred, cumulative: true, unmarshaler: decode[FilterLink]},
}

// Kind implements [Assertion].
func (ListenerSource) Kind() string { return KindListenerSource }

// Kind implements [Assertion].
func (GatewaySource) Kind() string { return KindGatewaySource }

// Kind implements [Assertion].
func (HostSource) Kind() string { return KindHostSource }

// Kind implements [Assertion].
func (RouteSource) Kind() string { return KindRouteSource }

// Kind implements [Assertion].
func (ExtensionSource) Kind() string { return KindExtensionSource }

// Kind implements [Assertion].
func (BackendSource) Kind() string { return KindBackendSource }

// Kind implements [Assertion].
func (PolicyRecord) Kind() string { return KindPolicyRecord }

// Kind implements [Assertion].
func (FilterLink) Kind() string { return KindFilterLink }

func (a ListenerSource) spec() kindSpec  { return specs[a.Kind()] }
func (a GatewaySource) spec() kindSpec   { return specs[a.Kind()] }
func (a HostSource) spec() kindSpec      { return specs[a.Kind()] }
func (a RouteSource) spec() kindSpec     { return specs[a.Kind()] }
func (a ExtensionSource) spec() kindSpec { return specs[a.Kind()] }
func (a BackendSource) spec() kindSpec   { return specs[a.Kind()] }
func (a PolicyRecord) spec() kindSpec    { return specs[a.Kind()] }
func (a FilterLink) spec() kindSpec      { return specs[a.Kind()] }

func (ListenerSource) objects() []model.Ref  { return nil }
func (a GatewaySource) objects() []model.Ref { return []model.Ref{a.Gateway} }
func (HostSource) objects() []model.Ref      { return nil }
func (a RouteSource) objects() []model.Ref   { return []model.Ref{a.Route} }
func (a ExtensionSource) objects() []model.Ref {
	if a.Extension == nil {
		return nil
	}
	return []model.Ref{*a.Extension}
}
func (a BackendSource) objects() []model.Ref { return []model.Ref{a.Backend} }
func (a PolicyRecord) objects() []model.Ref  { return []model.Ref{a.Policy} }
func (FilterLink) objects() []model.Ref      { return nil }

// MarshalJSON writes the assertion's kind and fields.
func (a ListenerSource) MarshalJSON() ([]byte, error) {
	type plain ListenerSource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a GatewaySource) MarshalJSON() ([]byte, error) {
	type plain GatewaySource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a HostSource) MarshalJSON() ([]byte, error) {
	type plain HostSource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a RouteSource) MarshalJSON() ([]byte, error) {
	type plain RouteSource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a ExtensionSource) MarshalJSON() ([]byte, error) {
	type plain ExtensionSource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a BackendSource) MarshalJSON() ([]byte, error) {
	type plain BackendSource
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a PolicyRecord) MarshalJSON() ([]byte, error) {
	type plain PolicyRecord
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// MarshalJSON writes the assertion's kind and fields.
func (a FilterLink) MarshalJSON() ([]byte, error) {
	type plain FilterLink
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{a.Kind(), plain(a)})
}

// decode reads an assertion of kind T, ignoring its "kind" field.
func decode[T Assertion](b []byte) (Assertion, error) {
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// UnmarshalJSON reads a claim, picking its assertion's type by "kind".
func (c *Claim) UnmarshalJSON(b []byte) error {
	type plain Claim
	var raw struct {
		plain
		Assertion json.RawMessage `json:"assertion"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*c = Claim(raw.plain)
	c.Assertion = nil
	if len(raw.Assertion) == 0 || string(raw.Assertion) == "null" {
		return nil
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw.Assertion, &head); err != nil {
		return err
	}
	s, ok := specs[head.Kind]
	if !ok {
		return fmt.Errorf("unknown assertion kind %q", head.Kind)
	}
	a, err := s.unmarshaler(raw.Assertion)
	if err != nil {
		return err
	}
	c.Assertion = a
	return nil
}

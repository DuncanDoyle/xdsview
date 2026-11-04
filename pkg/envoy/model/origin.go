package model

import "fmt"

// Ref names a Kubernetes object. Group is empty for the core group.
type Ref struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (r Ref) String() string { return fmt.Sprintf("%s %s/%s", r.Kind, r.Namespace, r.Name) }

// Certainty says how an origin was derived.
type Certainty int

const (
	Unknown   Certainty = iota // not recognised
	Ambiguous                  // several candidates
	Inferred                   // parsed from a name
	Exact                      // read from metadata
)

var certaintyNames = [...]string{"unknown", "ambiguous", "inferred", "exact"}

func (c Certainty) String() string {
	if c < 0 || int(c) >= len(certaintyNames) {
		return fmt.Sprintf("Certainty(%d)", int(c))
	}
	return certaintyNames[c]
}

// MarshalText writes the certainty as its name, e.g. "inferred".
func (c Certainty) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads a certainty name.
func (c *Certainty) UnmarshalText(b []byte) error {
	for i, n := range certaintyNames {
		if n == string(b) {
			*c = Certainty(i)
			return nil
		}
	}
	return fmt.Errorf("unknown certainty %q", b)
}

// RouteOrigin is the HTTPRoute rule and match an Envoy route came from.
type RouteOrigin struct {
	Route      Ref    `json:"route"`
	RuleIndex  int    `json:"ruleIndex"`          // -1 when unknown
	RuleName   string `json:"ruleName,omitempty"` // empty when the rule has no name
	MatchIndex int    `json:"matchIndex"`         // -1 when unknown
}

// RouteAnnotation holds a route's origin: one candidate unless Ambiguous.
type RouteAnnotation struct {
	Certainty  Certainty     `json:"certainty"`
	Candidates []RouteOrigin `json:"candidates,omitempty"`
}

// ListenerOrigin is the Gateway listener an Envoy listener, route config or
// filter chain came from. Gateway is empty when the caller didn't name it.
type ListenerOrigin struct {
	Certainty Certainty `json:"certainty"`
	Gateway   *Ref      `json:"gateway,omitempty"`
	Port      int       `json:"port,omitempty"`
	Section   string    `json:"section,omitempty"` // listener name, when the Envoy name carries it
}

// VirtualHostOrigin is the listener and hostname of an Envoy virtual host.
type VirtualHostOrigin struct {
	Certainty Certainty      `json:"certainty"`
	Listener  ListenerOrigin `json:"listener"`
	Hostname  string         `json:"hostname,omitempty"` // "*" when routes have no hostname
}

// BackendOrigin is the backend object and port behind an Envoy cluster.
type BackendOrigin struct {
	Certainty Certainty `json:"certainty"`
	Backend   Ref       `json:"backend"`
	Port      int       `json:"port,omitempty"`
}

// FilterOrigin says what an HTTP filter is and which extension configured it.
type FilterOrigin struct {
	Certainty Certainty `json:"certainty"`
	Kind      string    `json:"kind,omitempty"`      // Envoy filter kind from its type, e.g. "ext_authz"
	Extension *Ref      `json:"extension,omitempty"` // configuring object, e.g. a GatewayExtension
	Helper    bool      `json:"helper,omitempty"`    // control-plane plumbing, not a policy
}

// PolicyRef is a policy that contributed to a route, and the policy field it set.
type PolicyRef struct {
	Policy Ref    `json:"policy"`
	Field  string `json:"field,omitempty"` // e.g. "extAuth"
}

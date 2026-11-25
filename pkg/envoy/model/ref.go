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

// Certainty says how an attribution was derived.
type Certainty int

const (
	Unknown   Certainty = iota // not recognised
	Ambiguous                  // several candidates
	Inferred                   // from a naming convention or a module's tables
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

package provenance

import (
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// NameKind is the kind of Envoy resource a bare name belongs to.
type NameKind string

// Name kinds.
const (
	NameCluster     NameKind = "cluster"
	NameRouteConfig NameKind = "routeConfig"
	NameListener    NameKind = "listener"
)

// NameMapper maps a bare Envoy name, as found in stats or logs, to the
// objects that produced it. Implementations are pure and safe for concurrent use.
type NameMapper interface {
	Name(kind NameKind, name string) NameAnswer
}

// NameAnswer is what a name maps to. An ambiguous answer lists every
// candidate the name can read as, never a subset. Each candidate carries its
// own kind.
type NameAnswer struct {
	Certainty  model.Certainty `json:"certainty"`
	Candidates []model.Ref     `json:"candidates,omitempty"` // several when ambiguous
}

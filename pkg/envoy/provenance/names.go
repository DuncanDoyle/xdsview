package provenance

import (
	"slices"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// NameKind is the kind of Envoy resource a bare name belongs to.
type NameKind string

// Name kinds.
const (
	NameCluster     NameKind = "cluster"
	NameRouteConfig NameKind = "routeConfig"
	NameListener    NameKind = "listener"
	NameRoute       NameKind = "route" // a route's name, as in per-route stats
)

// NameKinds lists every kind, in the order [Guess] tries them.
var NameKinds = []NameKind{NameCluster, NameRouteConfig, NameListener, NameRoute}

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

// Guess maps a name whose kind is unknown: it asks m for every kind. One
// kind's answer is returned as is; answers for different kinds make the
// result ambiguous, with every candidate of each.
func Guess(m NameMapper, name string) NameAnswer {
	var answers []NameAnswer
	for _, k := range NameKinds {
		if a := m.Name(k, name); a.Certainty > model.Unknown {
			answers = append(answers, a)
		}
	}
	switch len(answers) {
	case 0:
		return NameAnswer{}
	case 1:
		return answers[0]
	}
	out := NameAnswer{Certainty: model.Ambiguous}
	for _, a := range answers {
		for _, c := range a.Candidates {
			if !slices.Contains(out.Candidates, c) {
				out.Candidates = append(out.Candidates, c)
			}
		}
	}
	return out
}

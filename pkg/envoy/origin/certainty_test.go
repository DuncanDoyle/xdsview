package origin_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// TestPolicies_CertaintyAndKeyCheck: the built-in reader's refs are exact, and
// an ID whose group or kind disagrees with its merge key is skipped.
func TestPolicies_CertaintyAndKeyCheck(t *testing.T) {
	r := model.Route{Metadata: map[string]any{
		"merge.TrafficPolicy.gateway.kgateway.dev": map[string]any{
			"extAuth": []any{
				"gateway.kgateway.dev/TrafficPolicy/ns/ok",
				"example.com/TrafficPolicy/ns/wrong-group",
				"gateway.kgateway.dev/AcmePolicy/ns/wrong-kind",
			},
		},
		"merge.AcmePolicy.example.com": map[string]any{"acmeWaf": []any{"example.com/AcmePolicy/ns/waf"}},
	}}
	want := []model.PolicyRef{
		{Policy: model.Ref{Group: "example.com", Kind: "AcmePolicy", Namespace: "ns", Name: "waf"}, Field: "acmeWaf", Certainty: model.Exact},
		{Policy: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "ok"}, Field: "extAuth", Certainty: model.Exact},
	}
	if got := origin.Kgateway(origin.KgatewayOptions{}).Policies(r); !reflect.DeepEqual(got, want) {
		t.Errorf("Policies =\n %+v\nwant\n %+v", got, want)
	}
}

// TestRoute_IndexCertainty: route_source identity is exact; indices that came
// from the Envoy route name are marked inferred, metadata indices are not.
func TestRoute_IndexCertainty(t *testing.T) {
	ref := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}
	md := func(rule, match string) map[string]any {
		src := map[string]any{"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "ns", "name": "web", "rule": rule}
		if match != "" {
			src["match"] = match
		}
		return map[string]any{"dev.kgateway.route_source": src}
	}
	const named = "listener~80~*-route-0-httproute-web-ns-1-0-api-matcher-0"
	cases := []struct {
		label    string
		r        model.Route
		want     model.RouteOrigin
		indexCer model.Certainty
	}{
		{"rule name, index from the name", model.Route{Name: named, Metadata: md("api", "_match-0")},
			model.RouteOrigin{Route: ref, RuleIndex: 1, RuleName: "api", MatchIndex: 0}, model.Inferred},
		{"rule name, name has no convention", model.Route{Name: "custom", Metadata: md("api", "_match-0")},
			model.RouteOrigin{Route: ref, RuleIndex: -1, RuleName: "api", MatchIndex: 0}, model.Unknown},
		{"full indices in metadata", model.Route{Name: named, Metadata: md("_rule-1", "_match-0")},
			model.RouteOrigin{Route: ref, RuleIndex: 1, RuleName: "api", MatchIndex: 0}, model.Inferred},
		{"full indices, no name", model.Route{Name: "custom", Metadata: md("_rule-1", "_match-0")},
			model.RouteOrigin{Route: ref, RuleIndex: 1, MatchIndex: 0}, model.Unknown},
		{"v2.4 shape", model.Route{Name: "custom", Metadata: md("httproute-web-ns-1-0", "")},
			model.RouteOrigin{Route: ref, RuleIndex: 1, MatchIndex: 0}, model.Unknown},
	}
	m := origin.Kgateway(origin.KgatewayOptions{})
	for _, c := range cases {
		got := m.Route(c.r)
		c.want.IndexCertainty = c.indexCer
		if got.Certainty != model.Exact || !reflect.DeepEqual(got.Candidates, []model.RouteOrigin{c.want}) {
			t.Errorf("%s: got %v %+v, want exact %+v", c.label, got.Certainty, got.Candidates, c.want)
		}
	}
}

// TestRoute_MalformedIndices: an index route_source carries but that doesn't
// parse stays unknown, so a valid name can supply it as inferred (8.1 review #1).
func TestRoute_MalformedIndices(t *testing.T) {
	ref := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}
	meta := map[string]any{"dev.kgateway.route_source": map[string]any{
		"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "ns", "name": "web", "rule": "_rule-bad", "match": "_match-bad"}}
	m := origin.Kgateway(origin.KgatewayOptions{})
	cases := []struct {
		name string
		want model.RouteOrigin
	}{
		{"custom", model.RouteOrigin{Route: ref, RuleIndex: -1, MatchIndex: -1}},
		{"listener~80~*-route-0-httproute-web-ns-2-1-matcher-1", model.RouteOrigin{Route: ref, RuleIndex: 2, MatchIndex: 1, IndexCertainty: model.Inferred}},
	}
	for _, c := range cases {
		got := m.Route(model.Route{Name: c.name, Metadata: meta})
		if got.Certainty != model.Exact || !reflect.DeepEqual(got.Candidates, []model.RouteOrigin{c.want}) {
			t.Errorf("%s: got %v %+v, want exact %+v", c.name, got.Certainty, got.Candidates, c.want)
		}
	}
}

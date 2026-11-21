package chain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

func tp(ns, name, field string) model.Contribution {
	return model.Contribution{Kind: model.ContributionPolicy, Field: field,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: ns, Name: name}}
}

// contributionsByRoute returns, per route whose name contains a key of want,
// each entry's contributions by filter name.
func contributionsOf(r model.Route) map[string][]model.Contribution {
	out := map[string][]model.Contribution{}
	for _, e := range r.Chain.Filters {
		out[e.Name] = e.Contributions
	}
	return out
}

// TestAnnotate_Corpus09 is kgateway v2.5.1 from kgwctl's fixtures (#28): local
// rate limit, the OSS transformation filter, and an ext auth disable.
func TestAnnotate_Corpus09(t *testing.T) {
	snap := parseFile(t, "../../../testdata/scenarios/09-kgwctl-policies/envoy/kgateway-v2.5.1/config_dump.json")
	ext := model.Contribution{Kind: model.ContributionExtension,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "kgwctl-proxy", Name: "authz"}}
	want := map[string]map[string][]model.Contribution{
		"orders_kgwctl_example-route-0-httproute-orders-": {
			"global_disable/ext_auth":          nil,
			"ext_auth/kgwctl-proxy/authz":      {ext, tp("kgwctl-proxy", "orders-extauth", "extAuth")},
			"ratelimit/local":                  {tp("kgwctl-proxy", "orders-ratelimit", "rateLimit.local")},
			"dynamic_modules/simple_mutations": {tp("kgwctl-proxy", "orders-transformation", "transformation")},
			"envoy.filters.http.router":        nil,
		},
		"public_kgwctl_example-route-0-httproute-public-": {
			"global_disable/ext_auth":          {tp("kgwctl-proxy", "public-no-extauth", "extAuth")},
			"ext_auth/kgwctl-proxy/authz":      {ext},
			"ratelimit/local":                  nil,
			"dynamic_modules/simple_mutations": nil,
			"envoy.filters.http.router":        nil,
		},
	}
	seen := 0
	for _, r := range routes(chain.Annotate(snap, origin.Kgateway(origin.KgatewayOptions{}))) {
		for key, w := range want {
			if !strings.Contains(r.Name, key) {
				continue
			}
			seen++
			if got := contributionsOf(r); !reflect.DeepEqual(got, w) {
				t.Errorf("%s:\n got %+v\nwant %+v", key, got, w)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("matched %d routes, want %d", seen, len(want))
	}
}

// TestAnnotate_Corpus02_1Transformation: OSS kgateway's transformation filter
// is the dynamic module, on v2.4.7 and v2.5.0 alike.
func TestAnnotate_Corpus02_1Transformation(t *testing.T) {
	want := []model.Contribution{tp("kgateway-system", "transformation", "transformation")}
	for version, path := range recordings(t, "02_1-single-policy") {
		rs := routes(chain.Annotate(parseFile(t, path), origin.Kgateway(origin.KgatewayOptions{})))
		if len(rs) == 0 {
			t.Fatalf("%s: no routes", version)
		}
		for _, r := range rs {
			if got := contributionsOf(r)["dynamic_modules/simple_mutations"]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s: transformation contributions = %+v, want %+v", version, r.Name, got, want)
			}
		}
	}
}

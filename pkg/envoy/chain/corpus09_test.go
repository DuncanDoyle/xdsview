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

// annotateOne runs chain.Annotate with the kgateway mapper on one route over
// the given HCM filters, route config and route, and returns contributions by filter.
func annotateOne(t *testing.T, filters []model.HTTPFilter, rcPFC map[string]any, r model.Route) map[string][]model.Contribution {
	t.Helper()
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		HTTPFilters: filters,
		RouteConfig: &model.RouteConfig{TypedPerFilterConfig: rcPFC, VirtualHosts: []model.VirtualHost{{Routes: []model.Route{r}}}},
	}}}}}}
	return contributionsOf(routes(chain.Annotate(snap, origin.Kgateway(origin.KgatewayOptions{})))[0])
}

func merge(fields map[string]string) map[string]any {
	m := map[string]any{}
	for field, name := range fields {
		m[field] = []any{"gateway.kgateway.dev/TrafficPolicy/ns/" + name}
	}
	return map[string]any{"merge.TrafficPolicy.gateway.kgateway.dev": m}
}

const (
	dynModType  = "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"
	setMetaType = "type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config"
	extProcType = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"
)

// TestAnnotate_TransformationBesideAuthMetadata: kgateway's ext_auth_enabled
// helper is the same rustformation module; transformation still goes to
// dynamic_modules/simple_mutations (kgateway v2.5.1 trafficpolicy/utils.go).
func TestAnnotate_TransformationBesideAuthMetadata(t *testing.T) {
	rust := map[string]any{"@type": dynModType, "filter_name": "rustformation"}
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	got := annotateOne(t, []model.HTTPFilter{
		{Name: "ext_auth_enabled", Type: dynModType, TypedConfig: rust, Disabled: true},
		{Name: "dynamic_modules/simple_mutations", Type: dynModType, TypedConfig: rust, Disabled: true},
	}, nil, model.Route{
		TypedPerFilterConfig: map[string]any{"ext_auth_enabled": enable, "dynamic_modules/simple_mutations": enable},
		Metadata:             merge(map[string]string{"transformation": "tx"}),
	})
	if want := []model.Contribution{tp("ns", "tx", "transformation")}; !reflect.DeepEqual(got["dynamic_modules/simple_mutations"], want) || got["ext_auth_enabled"] != nil {
		t.Errorf("contributions = %+v, want transformation on simple_mutations only", got)
	}
}

// TestAnnotate_DisableBesideInheritedProvider: a provider configured at route
// config level and a route-level disable helper both match extAuth; the most
// specific one gets the route's policy. Also covers ext_proc's helper.
func TestAnnotate_DisableBesideInheritedProvider(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	for _, c := range []struct{ field, helper, provider, typ string }{
		{"extAuth", "global_disable/ext_auth", "ext_auth/ns/authz", extAuthzType},
		{"extProc", "global_disable/ext_proc", "ext_proc/ns/proc", extProcType},
	} {
		got := annotateOne(t, []model.HTTPFilter{
			{Name: c.helper, Type: setMetaType, Disabled: true},
			{Name: c.provider, Type: c.typ, Disabled: true},
		}, map[string]any{c.provider: enable}, model.Route{
			TypedPerFilterConfig: map[string]any{c.helper: enable},
			Metadata:             merge(map[string]string{c.field: "off"}),
		})
		if want := []model.Contribution{tp("ns", "off", c.field)}; !reflect.DeepEqual(got[c.helper], want) {
			t.Errorf("%s: helper contributions = %+v, want %+v", c.field, got[c.helper], want)
		}
		for _, ct := range got[c.provider] {
			if ct.Kind == model.ContributionPolicy {
				t.Errorf("%s: provider got policy %+v", c.field, ct)
			}
		}
	}
}

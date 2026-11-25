package chain_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// TestEffective_CompositeSides: a composite runs on both sides whatever its
// delegate. The parser sets Type to the delegate; sides come from the installed filter.
func TestEffective_CompositeSides(t *testing.T) {
	wrapper := map[string]any{
		"@type": "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher",
		"extension_config": map[string]any{"name": "c", "typed_config": map[string]any{
			"@type": "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"}},
	}
	hcm := model.HCMConfig{HTTPFilters: []model.HTTPFilter{{Name: "c", Type: rbacType, TypedConfig: wrapper}}}
	e := chain.Effective(hcm, model.VirtualHost{}, model.Route{}).Filters[0]
	if e.Type != rbacType || e.Request != model.SideYes || e.Response != model.SideYes {
		t.Errorf("Type, Request, Response = %q, %q, %q; want the delegate type and yes, yes", e.Type, e.Request, e.Response)
	}
}

// TestEffective_FilterConfigExactType: only Envoy's own FilterConfig is the
// wrapper, and a null config counts as unset.
func TestEffective_FilterConfigExactType(t *testing.T) {
	hcm, vh, r := setup(true, at{route: map[string]any{"@type": "type.googleapis.com/acme.envoy.config.route.v3.FilterConfig", "disabled": true}})
	if e := chain.Effective(hcm, vh, r).Filters[0]; !e.Runs || e.State != model.FilterEnabled {
		t.Errorf("look-alike type: Runs, State = %v, %q; want true, enabled (a filter-specific config)", e.Runs, e.State)
	}
	hcm, vh, r = setup(false, at{route: map[string]any{"@type": filterConfigType, "config": nil}})
	if e := chain.Effective(hcm, vh, r).Filters[0]; e.Runs || e.State != model.FilterUnknown {
		t.Errorf("null config: Runs, State = %v, %q; want false, unknown", e.Runs, e.State)
	}
}

// TestAnnotate_PolicyNeedsOneMatch: with two ext_authz instances configured
// for the route, the extAuth policy can't be placed, so neither gets it.
func TestAnnotate_PolicyNeedsOneMatch(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	snap := func(both bool) *model.Snapshot {
		pfc := map[string]any{"ext_auth/ns/a": enable}
		if both {
			pfc["ext_auth/ns/b"] = enable
		}
		return &model.Snapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
			HTTPFilters: []model.HTTPFilter{
				{Name: "ext_auth/ns/a", Type: extAuthzType, Disabled: true},
				{Name: "ext_auth/ns/b", Type: extAuthzType, Disabled: true},
			},
			RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Routes: []model.Route{{
				TypedPerFilterConfig: pfc,
				Metadata: map[string]any{"merge.TrafficPolicy.gateway.kgateway.dev": map[string]any{
					"extAuth": []any{"gateway.kgateway.dev/TrafficPolicy/ns/auth"}}},
			}}}}},
		}}}}}}
	}
	policies := func(s *model.Snapshot) map[string]int {
		out := map[string]int{}
		for _, e := range routes(chain.Annotate(s, origin.Kgateway(origin.KgatewayOptions{})))[0].Chain.Filters {
			for _, c := range e.Contributions {
				if c.Kind == model.ContributionPolicy {
					out[e.Name]++
				}
			}
		}
		return out
	}
	if got, want := policies(snap(false)), map[string]int{"ext_auth/ns/a": 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("one instance: policy contributions = %v, want %v", got, want)
	}
	if got := policies(snap(true)); len(got) != 0 {
		t.Errorf("two instances: policy contributions = %v, want none", got)
	}
}

package chain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// composed runs origin.Annotate then chain.Annotate, as krp does, and returns
// the first route.
func composed(t *testing.T, snap *model.EnvoySnapshot, m origin.Mapper) model.Route {
	t.Helper()
	rs := routes(chain.Annotate(origin.Annotate(snap, m), m))
	if len(rs) == 0 {
		t.Fatal("no routes")
	}
	return rs[0]
}

// exactWaf is a vendor plugin that reads its extension from config, so its
// filter origin is exact.
type exactWaf struct{ origin.Mapper }

func (exactWaf) HTTPFilter(f model.HTTPFilter) model.FilterOrigin {
	ns, ok := strings.CutPrefix(f.Name, "acme_waf/")
	if !ok {
		return model.FilterOrigin{}
	}
	return model.FilterOrigin{Certainty: model.Exact, Kind: "waf",
		Extension: &model.Ref{Group: "acme.io", Kind: "WafExtension", Namespace: ns, Name: "waf"}}
}

// TestCertainty_Contributions: the OSS mapper's extension and policy
// contributions are inferred; a plugin's exact extension origin stays exact;
// Route.Policies refs read from metadata are exact.
func TestCertainty_Contributions(t *testing.T) {
	snap := parseString(t, pluginDump)
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	auth := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "auth"}
	authz := model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "ns", Name: "authz"}

	r := composed(t, snap, kgw)
	want := []model.Contribution{
		{Kind: model.ContributionExtension, Ref: authz, Certainty: model.Inferred},
		{Kind: model.ContributionPolicy, Ref: auth, Field: "extAuth", Certainty: model.Inferred},
	}
	if got := contributionsOf(r)["ext_auth/ns/authz"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ext_auth contributions =\n %+v\nwant\n %+v", got, want)
	}
	if !hasPolicy(r, model.PolicyRef{Policy: auth, Field: "extAuth", Certainty: model.Exact}) {
		t.Errorf("Route.Policies = %+v, want exact %v extAuth", r.Policies, auth)
	}

	r = composed(t, snap, origin.Chain(exactWaf{kgw}, kgw))
	wantWaf := model.Contribution{Kind: model.ContributionExtension, Certainty: model.Exact,
		Ref: model.Ref{Group: "acme.io", Kind: "WafExtension", Namespace: "ns", Name: "waf"}}
	if got := contributionsOf(r)["acme_waf/ns"]; len(got) == 0 || !reflect.DeepEqual(got[0], wantWaf) {
		t.Errorf("acme_waf contributions = %+v, want first %+v", got, wantWaf)
	}
}

// TestCertainty_OneCandidate: a policy field is linked only when exactly one
// entry with per-filter config matches it, at any level. A route-level disable
// helper beside a provider configured at a wider level is two candidates: no
// link, and the policy stays on Route.Policies. Covers ext_proc's helper too.
func TestCertainty_OneCandidate(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	for _, c := range []struct{ field, helper, provider, typ string }{
		{"extAuth", "global_disable/ext_auth", "ext_auth/ns/authz", extAuthzType},
		{"extProc", "global_disable/ext_proc", "ext_proc/ns/proc", extProcType},
	} {
		for _, level := range []string{"routeConfig", "virtualHost"} {
			vh := model.VirtualHost{Routes: []model.Route{{
				TypedPerFilterConfig: map[string]any{c.helper: enable},
				Metadata:             merge(map[string]string{c.field: "off"}),
			}}}
			rc := &model.RouteConfig{}
			if level == "routeConfig" {
				rc.TypedPerFilterConfig = map[string]any{c.provider: enable}
			} else {
				vh.TypedPerFilterConfig = map[string]any{c.provider: enable}
			}
			rc.VirtualHosts = []model.VirtualHost{vh}
			snap := &model.EnvoySnapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
				HTTPFilters: []model.HTTPFilter{
					{Name: c.helper, Type: setMetaType, Disabled: true},
					{Name: c.provider, Type: c.typ, Disabled: true},
				},
				RouteConfig: rc,
			}}}}}}
			r := composed(t, snap, origin.Kgateway(origin.KgatewayOptions{}))
			for name, cts := range contributionsOf(r) {
				for _, ct := range cts {
					if ct.Kind == model.ContributionPolicy {
						t.Errorf("%s, provider at %s: %s got policy %+v, want none", c.field, level, name, ct)
					}
				}
			}
			off := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "off"}
			if !hasPolicy(r, model.PolicyRef{Policy: off, Field: c.field, Certainty: model.Exact}) {
				t.Errorf("%s, provider at %s: Route.Policies = %+v, want %v", c.field, level, r.Policies, off)
			}
		}
	}
}

func hasPolicy(r model.Route, want model.PolicyRef) bool {
	for _, p := range r.Policies {
		if p == want {
			return true
		}
	}
	return false
}

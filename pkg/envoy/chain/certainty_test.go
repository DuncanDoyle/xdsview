package chain_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
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

// unsureAcme is a plugin whose policy records carry no certainty.
type unsureAcme struct{ origin.Mapper }

func (unsureAcme) Policies(model.Route) []model.PolicyRef {
	return []model.PolicyRef{{Policy: model.Ref{Group: "acme.io", Kind: "AcmePolicy", Namespace: "ns", Name: "waf"}, Field: "acmeWaf"}}
}

func (unsureAcme) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "acmeWaf" && f.Type == "type.googleapis.com/acme.waf.v1.Waf"
}

// TestCertainty_PolicyLinkNoStrongerThanRef: a unique filter match adds no
// evidence for the policy's identity, so an unknown ref stays unknown (8.1 review #3).
func TestCertainty_PolicyLinkNoStrongerThanRef(t *testing.T) {
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	r := composed(t, parseString(t, pluginDump), origin.Chain(unsureAcme{kgw}, kgw))
	got := contributionsOf(r)["acme_waf/ns"]
	if len(got) != 1 || got[0].Certainty != model.Unknown {
		t.Errorf("acme_waf contributions = %+v, want one with unknown certainty", got)
	}
}

// TestCertainty_BackendOverrideIsACandidate: a matching filter that a weighted
// backend configures counts as a candidate, so the route-level one alone
// doesn't get the policy (8.1 review #4).
func TestCertainty_BackendOverrideIsACandidate(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	snap := func(backendFilter string) *model.EnvoySnapshot {
		return &model.EnvoySnapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
			HTTPFilters: []model.HTTPFilter{
				{Name: "ext_auth/ns/a", Type: extAuthzType, Disabled: true},
				{Name: "ext_auth/ns/b", Type: extAuthzType, Disabled: true},
			},
			RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Routes: []model.Route{{
				TypedPerFilterConfig: map[string]any{"ext_auth/ns/a": enable},
				WeightedClusters:     []model.WeightedCluster{{Name: "c", Weight: 1, TypedPerFilterConfig: map[string]any{backendFilter: enable}}},
				Metadata:             merge(map[string]string{"extAuth": "auth"}),
			}}}}},
		}}}}}}
	}
	policies := func(s *model.EnvoySnapshot) int {
		n := 0
		for _, cts := range contributionsOf(composed(t, s, origin.Kgateway(origin.KgatewayOptions{}))) {
			for _, c := range cts {
				if c.Kind == model.ContributionPolicy {
					n++
				}
			}
		}
		return n
	}
	if got := policies(snap("ext_auth/ns/b")); got != 0 {
		t.Errorf("backend configures another match: %d policy contributions, want 0", got)
	}
	if got := policies(snap("ext_auth/ns/a")); got != 1 {
		t.Errorf("backend configures the same filter: %d policy contributions, want 1", got)
	}
}

// TestCertainty_ComposedOutput: an exact plugin contribution and name-derived
// route indices reach the JSON and text output (8.1 review #7).
func TestCertainty_ComposedOutput(t *testing.T) {
	dump := strings.Replace(pluginDump, `"routes": [{"name": "r",`,
		`"routes": [{"name": "listener~80~*-route-0-httproute-web-ns-1-0-api-matcher-0",`, 1)
	dump = strings.Replace(dump, `"metadata": {"filter_metadata": {`,
		`"metadata": {"filter_metadata": {"dev.kgateway.route_source": {"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "ns", "name": "web", "rule": "api", "match": "_match-0"},`, 1)
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	m := origin.Chain(exactWaf{kgw}, kgw)
	snap := chain.Annotate(origin.Annotate(parseString(t, dump), m), m)

	b, err := json.Marshal(routes(snap)[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"indexCertainty":"inferred"`, `"name":"waf"},"certainty":"exact"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON lacks %s:\n%s", want, b)
		}
	}
	var out bytes.Buffer
	if err := render.Text(&out, snap, render.Options{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "HTTPRoute ns/web · rule 1 (api) · match 0 (partly from name)") {
		t.Errorf("text lacks the name-derived indices:\n%s", text)
	}
	// Exact prints bare: no qualifier after the extension.
	if !strings.Contains(text, "WafExtension ns/waf") || strings.Contains(text, "WafExtension ns/waf (") {
		t.Errorf("want the exact extension bare:\n%s", text)
	}
}

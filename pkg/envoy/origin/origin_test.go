package origin_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/pkg/envoy/model"
	"github.com/DuncanDoyle/krp/pkg/envoy/origin"
	"github.com/DuncanDoyle/krp/pkg/envoy/parser"
)

// pluginDump has one made-up vendor filter and one OSS ext auth filter.
const pluginDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [
   {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
    {"name": "acme_waf/default", "typed_config": {"@type": "type.googleapis.com/acme.Waf"}},
    {"name": "ext_auth/ns/authz", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "listener~80", "virtual_hosts": [
  {"name": "listener~80~*", "domains": ["*"], "routes": [
   {"name": "listener~80~*-route-0-httproute-web-shop-0-0-matcher-0", "match": {"prefix": "/"}, "route": {"cluster": "kube_shop_web_8080"}}]}]}}]}
]}`

// acmeMapper is a vendor plugin: it knows "acme_waf/<ns>" and nothing else.
type acmeMapper struct{ origin.Mapper }

func (acmeMapper) HTTPFilter(f model.HTTPFilter) model.FilterOrigin {
	ns, ok := strings.CutPrefix(f.Name, "acme_waf/")
	if !ok {
		return model.FilterOrigin{}
	}
	return model.FilterOrigin{Certainty: model.Inferred, Kind: "waf",
		Extension: &model.Ref{Group: "acme.example", Kind: "WafPolicy", Namespace: ns, Name: "default"}}
}

func parseString(t *testing.T, s string) *model.EnvoySnapshot {
	t.Helper()
	res, err := parser.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return res.Snapshot
}

// TestAnnotate_PluginAndFallback checks what a consumer of a chained mapper
// sees: the plugin's origin on its filter, OSS origins everywhere else.
func TestAnnotate_PluginAndFallback(t *testing.T) {
	snap := parseString(t, pluginDump)
	oss := origin.Kgateway(origin.KgatewayOptions{Known: []model.Ref{{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"}}})
	out := origin.Annotate(snap, origin.Chain(acmeMapper{origin.Chain()}, oss))

	filters := out.Listeners[0].FilterChains[0].HCM.HTTPFilters
	if o := filters[0].Origin; o == nil || o.Kind != "waf" || o.Extension.Kind != "WafPolicy" {
		t.Errorf("plugin filter origin = %+v", o)
	}
	if o := filters[1].Origin; o == nil || o.Kind != "ext_authz" || o.Extension.Name != "authz" {
		t.Errorf("OSS filter origin = %+v", o)
	}
	route := out.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	if route.Origin == nil || route.Origin.Certainty != model.Inferred || route.Origin.Candidates[0].Route.Name != "web" {
		t.Errorf("route origin = %+v", route.Origin)
	}
	if out.Listeners[0].Origin == nil || out.Listeners[0].Origin.Port != 80 {
		t.Errorf("listener origin = %+v", out.Listeners[0].Origin)
	}
	// The input is untouched.
	if snap.Listeners[0].Origin != nil || snap.Listeners[0].FilterChains[0].HCM.HTTPFilters[0].Origin != nil {
		t.Error("Annotate modified its input")
	}
}

func TestObjectForName(t *testing.T) {
	gw := model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "infra", Name: "edge"}
	withGW := origin.Kgateway(origin.KgatewayOptions{Gateway: &gw})
	cases := []struct {
		name   string
		m      origin.Mapper
		want   model.Ref
		wantOK bool
	}{
		{"kube_shop_httpbin-nack_8000", withGW, model.Ref{Kind: "Service", Namespace: "shop", Name: "httpbin-nack"}, true},
		{"listener~8080", withGW, gw, true},
		{"https", withGW, gw, true},
		{"listener~8080", origin.Kgateway(origin.KgatewayOptions{}), model.Ref{}, false}, // Gateway unknown
		{"backend_shop_static-be_0", withGW, model.Ref{}, false},                         // not a kube_ cluster, and "_" rules out a section
		{"xds_cluster", withGW, model.Ref{}, false},
		{"a/b/c", withGW, model.Ref{}, false}, // ListenerSet chains aren't mapped yet
	}
	for _, c := range cases {
		got, ok := origin.ObjectForName(c.m, c.name)
		if ok != c.wantOK || got != c.want {
			t.Errorf("ObjectForName(%q) = %v, %v; want %v, %v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

// TestRoute_RuleNameWithDigits checks a rule name that itself looks like
// "<rule>-<match>": the reading must keep the real indices.
func TestRoute_RuleNameWithDigits(t *testing.T) {
	m := origin.Kgateway(origin.KgatewayOptions{Known: []model.Ref{{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}}})
	got := m.Route(model.Route{Name: "listener~80~*-route-0-httproute-web-ns-1-0-v2-0-matcher-0"})
	want := []model.RouteOrigin{{Route: model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "ns", Name: "web"}, RuleIndex: 1, RuleName: "v2-0", MatchIndex: 0}}
	if got.Certainty != model.Inferred || !reflect.DeepEqual(got.Candidates, want) {
		t.Errorf("got %v %v, want inferred %v", got.Certainty, got.Candidates, want)
	}
}

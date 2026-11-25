package renderer_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/renderer"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// claims is a test module that returns the claims it holds.
type claims []provenance.Claim

func (claims) Name() string                                { return "test" }
func (c claims) Claims(*model.Snapshot) []provenance.Claim { return c }

// renderRoute renders a one-route snapshot as plain text, with the
// provenance labels and policies line from the given claims.
func renderRoute(t *testing.T, hcm model.HCMConfig, route model.Route, cs ...provenance.Claim) string {
	t.Helper()
	hcm.RouteConfig = &model.RouteConfig{Name: "rc", VirtualHosts: []model.VirtualHost{
		{Name: "vh", Domains: []string{"*"}, Routes: []model.Route{route}},
	}}
	v := view(&model.Snapshot{Listeners: []model.Listener{{
		Name: "listener~80", Address: "[::]:80",
		FilterChains: []model.NetworkFilterChain{{HCM: &hcm}},
	}}})
	ix, err := provenance.Compose(v.Snapshot, claims(cs))
	if err != nil {
		t.Fatal(err)
	}
	if w := ix.Warnings(); len(w) > 0 {
		t.Fatalf("claims dropped: %v", w)
	}
	v.Labels, v.Trailer = provenance.Labels(ix, nil), provenance.Notes(ix)
	return ansi.ReplaceAllString(renderer.Render(v), "")
}

// claim is a claim on subject with one metadata evidence naming obj.
func claim(subject model.ID, a provenance.Assertion, c model.Certainty, obj *model.Ref) provenance.Claim {
	return provenance.Claim{Subject: subject, Assertion: a, Certainty: c,
		Evidence: []provenance.Evidence{{Kind: provenance.EvidenceMetadata, Subject: subject, Field: "test", Object: obj}}}
}

var routerOnly = model.HCMConfig{HTTPFilters: []model.HTTPFilter{{Name: "envoy.filters.http.router"}}}

func TestRender_WeightedClusters(t *testing.T) {
	out := renderRoute(t, routerOnly, model.Route{WeightedClusters: []model.WeightedCluster{
		{Name: "kube_ns_v1_8080", Weight: 80}, {Name: "kube_ns_v2_8080", Weight: 20},
	}})
	if !strings.Contains(out, "backends: kube_ns_v1_8080 (80), kube_ns_v2_8080 (20)") {
		t.Errorf("weighted backends missing:\n%s", out)
	}
}

func TestRender_PrefixRewrite(t *testing.T) {
	out := renderRoute(t, routerOnly, model.Route{Cluster: "c", Rewrite: &model.RouteRewrite{Prefix: "/new"}})
	if !strings.Contains(out, "rewrite prefix: /new") {
		t.Errorf("prefix rewrite missing:\n%s", out)
	}
	if strings.Contains(out, "rewrite: ") {
		t.Errorf("empty regex rewrite rendered:\n%s", out)
	}
}

func TestRender_UpstreamFilters(t *testing.T) {
	hcm := routerOnly
	hcm.UpstreamFilters = []model.HTTPFilter{{Name: "late"}, {Name: "envoy.filters.http.upstream_codec"}}
	out := renderRoute(t, hcm, model.Route{Cluster: "c"})
	i := strings.Index(out, "router upstream filters (as configured): late, envoy.filters.http.upstream_codec")
	if i < 0 {
		t.Fatalf("upstream filters missing or out of order:\n%s", out)
	}
	if strings.Index(out, "envoy.filters.http.router") > i {
		t.Errorf("upstream filters should follow the HTTP filter list:\n%s", out)
	}
}

var testRoute = model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"}

// TestRender_Origins checks the origin text: a route's HTTPRoute rule, a
// filter row's extension, and the route's policies.
func TestRender_Origins(t *testing.T) {
	const route, slot = "L0/C0/N0/RC/V0/R0", "L0/C0/N0/F0"
	hcm := model.HCMConfig{HTTPFilters: []model.HTTPFilter{
		{Name: "ext_auth/shop/authz"},
		{Name: "envoy.filters.http.router"},
	}}
	authz := model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "shop", Name: "authz"}
	auth := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "shop", Name: "auth"}
	out := renderRoute(t, hcm, model.Route{Cluster: "c"},
		claim(route, provenance.RouteSource{Route: testRoute, RuleIndex: 1, RuleName: "api", MatchIndex: 0}, model.Exact, &testRoute),
		claim(slot, provenance.ExtensionSource{Extension: &authz}, model.Inferred, &authz),
		claim(route, provenance.PolicyRecord{Policy: auth, Field: "extAuth", Level: provenance.LevelRoute}, model.Exact, &auth))
	for _, want := range []string{
		"HTTPRoute shop/web · rule 1 (api) · match 0",
		"ext_auth/shop/authz        on · GatewayExtension shop/authz",
		"policies: TrafficPolicy shop/auth (extAuth)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestRender_AmbiguousOriginLabelled checks that an ambiguous route origin
// lists its candidates as ambiguous rather than picking one.
func TestRender_AmbiguousOriginLabelled(t *testing.T) {
	other := testRoute
	other.Namespace, other.Name = "web-shop", "x"
	const route = "L0/C0/N0/RC/V0/R0"
	out := renderRoute(t, routerOnly, model.Route{Cluster: "c"},
		claim(route, provenance.RouteSource{Route: testRoute, RuleIndex: 1, MatchIndex: -1}, model.Inferred, &testRoute),
		claim(route, provenance.RouteSource{Route: other, RuleIndex: 1, MatchIndex: -1}, model.Inferred, &other))
	if !strings.Contains(out, "ambiguous: HTTPRoute shop/web, HTTPRoute web-shop/x") || strings.Contains(out, "rule 1") {
		t.Errorf("ambiguous origin not labelled, or a candidate printed as the origin:\n%s", out)
	}
}

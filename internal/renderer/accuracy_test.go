package renderer_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/internal/renderer"
	"github.com/DuncanDoyle/krp/pkg/envoy/model"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// renderRoute renders a one-route snapshot as plain text.
func renderRoute(hcm model.HCMConfig, route model.Route) string {
	hcm.RouteConfig = &model.RouteConfig{Name: "rc", VirtualHosts: []model.VirtualHost{
		{Name: "vh", Domains: []string{"*"}, Routes: []model.Route{route}},
	}}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{
		Name: "listener~80", Address: "[::]:80",
		FilterChains: []model.NetworkFilterChain{{HCM: &hcm}},
	}}}
	return ansi.ReplaceAllString(renderer.Render(snap), "")
}

var routerOnly = model.HCMConfig{HTTPFilters: []model.HTTPFilter{{Name: "envoy.filters.http.router"}}}

func TestRender_WeightedClusters(t *testing.T) {
	out := renderRoute(routerOnly, model.Route{WeightedClusters: []model.WeightedCluster{
		{Name: "kube_ns_v1_8080", Weight: 80}, {Name: "kube_ns_v2_8080", Weight: 20},
	}})
	if !strings.Contains(out, "Backends: kube_ns_v1_8080 (80), kube_ns_v2_8080 (20)") {
		t.Errorf("weighted backends missing:\n%s", out)
	}
}

func TestRender_PrefixRewrite(t *testing.T) {
	out := renderRoute(routerOnly, model.Route{Cluster: "c", Rewrite: &model.RouteRewrite{Prefix: "/new"}})
	if !strings.Contains(out, "rewrite-prefix: /new") {
		t.Errorf("prefix rewrite missing:\n%s", out)
	}
	if strings.Contains(out, "rewrite: ") {
		t.Errorf("empty regex rewrite rendered:\n%s", out)
	}
}

func TestRender_UpstreamFilters(t *testing.T) {
	hcm := routerOnly
	hcm.UpstreamFilters = []model.HTTPFilter{{Name: "late"}, {Name: "envoy.filters.http.upstream_codec"}}
	out := renderRoute(hcm, model.Route{Cluster: "c"})
	i := strings.Index(out, "Upstream Filters:")
	if i < 0 {
		t.Fatalf("upstream filters missing:\n%s", out)
	}
	rest := out[i:]
	a, b := strings.Index(rest, "late"), strings.Index(rest, "envoy.filters.http.upstream_codec")
	if a < 0 || b < a {
		t.Errorf("upstream filters not listed in order:\n%s", out)
	}
	if strings.Index(out, "envoy.filters.http.router") > i {
		t.Errorf("upstream filters should follow the HTTP filter list:\n%s", out)
	}
}

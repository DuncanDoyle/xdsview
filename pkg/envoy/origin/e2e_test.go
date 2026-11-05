package origin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// Expectations are written from the scenario manifests, not computed.

func httpRoute(name string) model.Ref {
	return model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: name}
}

func ro(route string, rule int, ruleName string, match int) model.RouteOrigin {
	return model.RouteOrigin{Route: httpRoute(route), RuleIndex: rule, RuleName: ruleName, MatchIndex: match}
}

func r80(n int, tail string) string {
	return fmt.Sprintf("listener~80~api_example_com-route-%d-httproute-api-example-com-kgateway-system-%s", n, tail)
}

const (
	http0      = "listener~80~api_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
	httpsAPI   = "https-api~api_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
	httpsDev   = "https-developer~developer_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
	adminRoute = "listener~80~api_example_com-route-0-httproute-api-example-com-admin-kgateway-system-0-0-matcher-0"
)

var simple = map[string]model.RouteOrigin{http0: ro("api-example-com", 0, "", 0)}

// routeOrigins maps each Envoy route name of a scenario to its origin.
var routeOrigins = map[string]map[string]model.RouteOrigin{
	"00-simple": simple, "01_1-matchers": simple, "01_2-matchers": simple, "01_3-matchers": simple,
	"01_4-matchers": simple, "01_5-matchers": simple, "01_6-matchers": simple, "01_7-matchers": simple,
	"01_8-matchers": simple, "01_9-matchers": simple,
	"02_1-single-policy": {httpsAPI: ro("api-example-com", 0, "", 0), httpsDev: ro("api-example-com", 0, "", 0)},
	"02_2-single-policy": simple, "02_3-single-policy": simple, "02_4-single-policy": simple,
	"02_5-single-policy": simple, "02_6-single-policy": simple, "02_7-single-policy": simple,
	"02_8-single-policy": simple, "03-multi-policy": simple, "07-traffic-split": simple,
	"04-multi-rule": {
		r80(0, "0-0-matcher-0"): ro("api-example-com", 0, "", 0),
		r80(1, "1-0-matcher-0"): ro("api-example-com", 1, "", 0),
		r80(2, "1-1-matcher-1"): ro("api-example-com", 1, "", 1),
		r80(3, "2-0-matcher-0"): ro("api-example-com", 2, "", 0),
	},
	"05-multi-listener": {http0: ro("api-example-com", 0, "", 0), httpsAPI: ro("api-example-com", 0, "", 0)},
	"06-multi-extproc": {
		"listener~80~api_example_com-route-1-httproute-api-example-com-kgateway-system-0-0-matcher-0": ro("api-example-com", 0, "", 0),
		adminRoute: ro("api-example-com-admin", 0, "", 0),
	},
	"08-named-rules": {
		r80(2, "0-0-a-matcher-0"):           ro("api-example-com", 0, "a", 0),
		r80(1, "1-0-a-b-matcher-0"):         ro("api-example-com", 1, "a-b", 0),
		r80(0, "2-0-a-matcher-b-matcher-0"): ro("api-example-com", 2, "a-matcher-b", 0),
	},
}

func gwExt(name string) *model.Ref {
	return &model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: "kgateway-system", Name: name}
}

func fo(kind string, ext *model.Ref, helper bool) model.FilterOrigin {
	return model.FilterOrigin{Certainty: model.Inferred, Kind: kind, Extension: ext, Helper: helper}
}

// policyFilters maps the policy filters of a scenario to their origins.
var policyFilters = map[string]map[string]model.FilterOrigin{
	"02_1-single-policy": {"dynamic_modules/simple_mutations": fo("dynamic_modules", nil, false)},
	"02_7-single-policy": {
		"ext_auth/kgateway-system/ext-authz": fo("ext_authz", gwExt("ext-authz"), false),
		"global_disable/ext_auth":            fo("set_metadata", nil, true),
	},
	"02_8-single-policy": {"ratelimit/kgateway-system/ratelimit": fo("ratelimit", gwExt("ratelimit"), false)},
	"03-multi-policy": {
		"envoy.filters.http.cors":             fo("cors", nil, false),
		"ext_auth/kgateway-system/ext-authz":  fo("ext_authz", gwExt("ext-authz"), false),
		"global_disable/ext_auth":             fo("set_metadata", nil, true),
		"ratelimit/kgateway-system/ratelimit": fo("ratelimit", gwExt("ratelimit"), false),
	},
	"06-multi-extproc": {
		"ext_proc/kgateway-system/ext-proc-a": fo("ext_proc", gwExt("ext-proc-a"), false),
		"ext_proc/kgateway-system/ext-proc-b": fo("ext_proc", gwExt("ext-proc-b"), false),
		"global_disable/ext_proc":             fo("set_metadata", nil, true),
	},
}

func trafficPolicy(name, field string) model.PolicyRef {
	return model.PolicyRef{Policy: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "kgateway-system", Name: name}, Field: field}
}

// routePolicies maps route names to the policies their metadata names.
var routePolicies = map[string]map[string][]model.PolicyRef{
	"02_1-single-policy": {httpsAPI: {trafficPolicy("transformation", "transformation")}, httpsDev: {trafficPolicy("transformation", "transformation")}},
	"02_6-single-policy": {http0: {trafficPolicy("cors", "cors")}},
	"02_7-single-policy": {http0: {trafficPolicy("ext-auth", "extAuth")}},
	"02_8-single-policy": {http0: {trafficPolicy("rate-limit", "rateLimit.global")}},
	"03-multi-policy": {http0: {
		trafficPolicy("cors", "cors"), trafficPolicy("ext-auth", "extAuth"), trafficPolicy("rate-limit", "rateLimit.global"),
	}},
	"06-multi-extproc": {
		"listener~80~api_example_com-route-1-httproute-api-example-com-kgateway-system-0-0-matcher-0": {trafficPolicy("ext-proc-a", "extProc")},
		adminRoute: {trafficPolicy("ext-proc-b", "extProc")},
	},
}

// known lists a scenario's HTTPRoutes, as a caller with cluster access would.
func known(scenario string) []model.Ref {
	if scenario == "06-multi-extproc" {
		return []model.Ref{httpRoute("api-example-com"), httpRoute("api-example-com-admin")}
	}
	return []model.Ref{httpRoute("api-example-com")}
}

func parse(t *testing.T, path string) *model.EnvoySnapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := parser.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return res.Snapshot
}

// dumps returns every OSS recording of a scenario, keyed by version directory.
func dumps(t *testing.T, scenario string) map[string]string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join("../../../testdata/scenarios", scenario, "envoy/kgateway-*/config_dump.json"))
	if len(paths) == 0 {
		t.Fatalf("no recordings for %s", scenario)
	}
	out := map[string]string{}
	for _, p := range paths {
		out[filepath.Base(filepath.Dir(p))] = p
	}
	return out
}

func eachRoute(snap *model.EnvoySnapshot, fn func(model.Route)) {
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					fn(r)
				}
			}
		}
	}
}

// TestKgateway_RouteOrigins checks every route of every OSS recording. With the
// scenario's HTTPRoutes known, each route has exactly its origin. Without them,
// the candidates contain it. Recordings with route_source metadata are Exact.
func TestKgateway_RouteOrigins(t *testing.T) {
	for scenario, want := range routeOrigins {
		for version, path := range dumps(t, scenario) {
			t.Run(scenario+"/"+version, func(t *testing.T) {
				snap := parse(t, path)
				exact := strings.HasSuffix(version, "-route-source")
				withKnown := origin.Kgateway(origin.KgatewayOptions{Known: known(scenario)})
				bare := origin.Kgateway(origin.KgatewayOptions{})
				seen := map[string]bool{}
				eachRoute(snap, func(r model.Route) {
					w, ok := want[r.Name]
					if !ok {
						t.Errorf("unexpected route %s", r.Name)
						return
					}
					seen[r.Name] = true
					got := withKnown.Route(r)
					wantCertainty := model.Inferred
					if exact {
						wantCertainty = model.Exact
					}
					if got.Certainty != wantCertainty || !reflect.DeepEqual(got.Candidates, []model.RouteOrigin{w}) {
						t.Errorf("%s with Known: %v %v, want %v %v", r.Name, got.Certainty, got.Candidates, wantCertainty, w)
					}
					if g := bare.Route(r); !slices.Contains(g.Candidates, w) {
						t.Errorf("%s without Known: candidates %v lack %v", r.Name, g.Candidates, w)
					}
				})
				for name := range want {
					if !seen[name] {
						t.Errorf("route %s missing from the dump", name)
					}
				}
			})
		}
	}
}

// TestKgateway_ExactMatchesInferred compares metadata and name readings on the
// route_source recordings: the name candidates contain the metadata origin.
func TestKgateway_ExactMatchesInferred(t *testing.T) {
	m := origin.Kgateway(origin.KgatewayOptions{})
	for version, path := range dumps(t, "04-multi-rule") {
		if !strings.HasSuffix(version, "-route-source") {
			continue
		}
		eachRoute(parse(t, path), func(r model.Route) {
			exact := m.Route(r)
			if exact.Certainty != model.Exact {
				t.Fatalf("%s %s: certainty %v, want exact", version, r.Name, exact.Certainty)
			}
			stripped := r
			stripped.Metadata = nil
			if inferred := m.Route(stripped); !slices.Contains(inferred.Candidates, exact.Candidates[0]) {
				t.Errorf("%s %s: name candidates %v lack metadata origin %v", version, r.Name, inferred.Candidates, exact.Candidates[0])
			}
		})
	}
}

func TestKgateway_PolicyFilters(t *testing.T) {
	m := origin.Kgateway(origin.KgatewayOptions{})
	for scenario, want := range policyFilters {
		for version, path := range dumps(t, scenario) {
			got := map[string]model.FilterOrigin{}
			for _, l := range parse(t, path).Listeners {
				for _, fc := range l.FilterChains {
					if fc.HCM == nil {
						continue
					}
					for _, f := range fc.HCM.HTTPFilters {
						if _, ok := want[f.Name]; ok {
							got[f.Name] = m.HTTPFilter(f)
						}
					}
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s/%s: filter origins\n got %v\nwant %v", scenario, version, got, want)
			}
		}
	}
}

func TestKgateway_Policies(t *testing.T) {
	m := origin.Kgateway(origin.KgatewayOptions{})
	for scenario, want := range routePolicies {
		for version, path := range dumps(t, scenario) {
			eachRoute(parse(t, path), func(r model.Route) {
				if got := m.Policies(r); !reflect.DeepEqual(got, want[r.Name]) {
					t.Errorf("%s/%s %s: policies %v, want %v", scenario, version, r.Name, got, want[r.Name])
				}
			})
		}
	}
}

// TestKgateway_VirtualHostsAndClusters checks listener, virtual host and
// cluster origins on the multi-listener and traffic-split recordings.
func TestKgateway_VirtualHostsAndClusters(t *testing.T) {
	gw := &model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "kgateway-system", Name: "gw"}
	m := origin.Kgateway(origin.KgatewayOptions{Gateway: gw})
	wantVH := map[string]model.VirtualHostOrigin{
		"listener~80~api_example_com": {Certainty: model.Inferred, Hostname: "api.example.com",
			Listener: model.ListenerOrigin{Certainty: model.Inferred, Gateway: gw, Port: 80}},
		"https-api~api_example_com": {Certainty: model.Inferred, Hostname: "api.example.com",
			Listener: model.ListenerOrigin{Certainty: model.Inferred, Gateway: gw, Section: "https-api"}},
	}
	for version, path := range dumps(t, "05-multi-listener") {
		got := map[string]model.VirtualHostOrigin{}
		for _, l := range parse(t, path).Listeners {
			for _, fc := range l.FilterChains {
				if fc.HCM != nil && fc.HCM.RouteConfig != nil {
					for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
						got[vh.Name] = m.VirtualHost(vh)
					}
				}
			}
		}
		if !reflect.DeepEqual(got, wantVH) {
			t.Errorf("05/%s: virtual hosts\n got %v\nwant %v", version, got, wantVH)
		}
	}
	wantSplit := []model.BackendOrigin{
		{Certainty: model.Inferred, Backend: model.Ref{Kind: "Service", Namespace: "httpbin", Name: "httpbin"}, Port: 8000},
		{Certainty: model.Inferred, Backend: model.Ref{Kind: "Service", Namespace: "httpbin", Name: "httpbin-v2"}, Port: 8000},
	}
	for version, path := range dumps(t, "07-traffic-split") {
		eachRoute(parse(t, path), func(r model.Route) {
			var got []model.BackendOrigin
			for _, wc := range r.WeightedClusters {
				got = append(got, m.Cluster(wc.Name))
			}
			if !reflect.DeepEqual(got, wantSplit) {
				t.Errorf("07/%s: backends %v, want %v", version, got, wantSplit)
			}
		})
	}
}

package filter_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/pkg/envoy/filter"
	"github.com/DuncanDoyle/krp/pkg/envoy/model"
	"github.com/DuncanDoyle/krp/pkg/envoy/parser"
)

// ossVersions are the kgateway versions every OSS scenario is recorded on.
var ossVersions = []string{"kgateway-v2.4.7", "kgateway-v2.5.0"}

// Envoy route names kgateway generates for the scenarios. They are written out
// from the manifests, not computed with the filter's own naming logic.
const (
	httpRoute0     = "listener~80~api_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
	httpsAPI       = "https-api~api_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
	httpsDeveloper = "https-developer~developer_example_com-route-0-httproute-api-example-com-kgateway-system-0-0-matcher-0"
)

// httpRoute is one HTTPRoute of a scenario and the exact Envoy route names each
// of its rules produces.
type httpRoute struct {
	name, namespace string
	rules           [][]string
}

func single(name string) []httpRoute {
	return []httpRoute{{"api-example-com", "kgateway-system", [][]string{{name}}}}
}

// scenarioRoutes lists the HTTPRoutes of every recorded OSS scenario.
var scenarioRoutes = map[string][]httpRoute{
	"00-simple":          single(httpRoute0),
	"01_1-matchers":      single(httpRoute0),
	"01_2-matchers":      single(httpRoute0),
	"01_3-matchers":      single(httpRoute0),
	"01_4-matchers":      single(httpRoute0),
	"01_5-matchers":      single(httpRoute0),
	"01_6-matchers":      single(httpRoute0),
	"01_7-matchers":      single(httpRoute0),
	"01_8-matchers":      single(httpRoute0),
	"01_9-matchers":      single(httpRoute0),
	"02_1-single-policy": {{"api-example-com", "kgateway-system", [][]string{{httpsAPI, httpsDeveloper}}}}, // two HTTPS hostnames
	"02_2-single-policy": single(httpRoute0),
	"02_3-single-policy": single(httpRoute0),
	"02_4-single-policy": single(httpRoute0),
	"02_5-single-policy": single(httpRoute0),
	"02_6-single-policy": single(httpRoute0),
	"02_7-single-policy": single(httpRoute0),
	"02_8-single-policy": single(httpRoute0),
	// The second name extends the first, so a prefix-only match would keep both.
	// Envoy orders the more specific /admin route first.
	"06-multi-extproc": {
		{"api-example-com", "kgateway-system", [][]string{{"listener~80~api_example_com-route-1-httproute-api-example-com-kgateway-system-0-0-matcher-0"}}},
		{"api-example-com-admin", "kgateway-system", [][]string{{"listener~80~api_example_com-route-0-httproute-api-example-com-admin-kgateway-system-0-0-matcher-0"}}},
	},
}

// enterpriseRoutes is scenarioRoutes for the enterprise corpus: its 00-simple
// HTTPRoute lives in "default", and its 06 dump is Gloo Gateway v1, whose route
// names carry no HTTPRoute identity to filter on.
func enterpriseRoutes() map[string][]httpRoute {
	m := map[string][]httpRoute{}
	for k, v := range scenarioRoutes {
		m[k] = v
	}
	m["00-simple"] = []httpRoute{{"api-example-com", "default", [][]string{{
		"listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0",
	}}}}
	delete(m, "06-multi-extproc")
	return m
}

func parseDump(t *testing.T, path string) *model.EnvoySnapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	defer f.Close()
	result, err := parser.Parse(f)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return result.Snapshot
}

// routeNames returns the names of all routes in snap, in order.
func routeNames(snap *model.EnvoySnapshot) []string {
	var names []string
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					names = append(names, r.Name)
				}
			}
		}
	}
	return names
}

// checkScenario asserts that filtering snap keeps exactly the expected route
// names for each HTTPRoute, for all rules and for each rule, and nothing for a
// rule index past the last one.
func checkScenario(t *testing.T, snap *model.EnvoySnapshot, routes []httpRoute) {
	t.Helper()
	keep := func(hr httpRoute, rule int) []string {
		names := routeNames(filter.Filter(snap, filter.Options{
			HTTPRouteName: hr.name, HTTPRouteNamespace: hr.namespace, RuleIndex: rule,
		}))
		slices.Sort(names)
		return names
	}
	for _, hr := range routes {
		var want []string
		for rule, names := range hr.rules {
			want = append(want, names...)
			ruleWant := slices.Sorted(slices.Values(names))
			if got := keep(hr, rule); !slices.Equal(got, ruleWant) {
				t.Errorf("%s rule %d: kept %v, want %v", hr.name, rule, got, ruleWant)
			}
		}
		slices.Sort(want)
		if got := keep(hr, filter.AllRules); !slices.Equal(got, want) {
			t.Errorf("%s: kept %v, want %v", hr.name, got, want)
		}
		if got := keep(hr, len(hr.rules)); len(got) != 0 {
			t.Errorf("%s rule %d (no such rule): kept %v", hr.name, len(hr.rules), got)
		}
	}
}

// TestFilter_E2E_OSSScenarios runs checkScenario on every OSS scenario and
// kgateway version. Passing on every version shows the route naming convention
// still holds there.
func TestFilter_E2E_OSSScenarios(t *testing.T) {
	for scenario, routes := range scenarioRoutes {
		for _, version := range ossVersions {
			t.Run(scenario+"/"+version, func(t *testing.T) {
				checkScenario(t, parseDump(t, filepath.Join("../../../testdata/scenarios", scenario, "envoy", version, "config_dump.json")), routes)
			})
		}
	}
}

// TestFilter_E2E_EnterpriseScenarios runs checkScenario on the enterprise corpus.
func TestFilter_E2E_EnterpriseScenarios(t *testing.T) {
	for scenario, routes := range enterpriseRoutes() {
		t.Run(scenario, func(t *testing.T) {
			checkScenario(t, parseDump(t, filepath.Join("../../../testdata/enterprise/scenarios", scenario, "envoy", "config_dump.json")), routes)
		})
	}
}

// TestFilter_E2E_OSSCorpusComplete fails when a scenario with an HTTPRoute
// manifest has no expectations or no recording, so a skipped recording cannot go
// unnoticed.
func TestFilter_E2E_OSSCorpusComplete(t *testing.T) {
	manifests, _ := filepath.Glob("../../../testdata/scenarios/*/k8s/*.yaml")
	for _, m := range manifests {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "\nkind: HTTPRoute") && !strings.HasPrefix(string(data), "kind: HTTPRoute") {
			continue
		}
		scenario := filepath.Base(filepath.Dir(filepath.Dir(m)))
		if _, ok := scenarioRoutes[scenario]; !ok {
			t.Errorf("scenario %s has an HTTPRoute but no entry in scenarioRoutes", scenario)
		}
	}
	for scenario := range scenarioRoutes {
		for _, version := range ossVersions {
			if _, err := os.Stat(filepath.Join("../../../testdata/scenarios", scenario, "envoy", version, "config_dump.json")); err != nil {
				t.Errorf("missing recording: %v", err)
			}
		}
	}
}

// TestFilter_E2E_NoMatch verifies that an unknown HTTPRoute yields no listeners.
func TestFilter_E2E_NoMatch(t *testing.T) {
	snap := parseDump(t, "../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
	none := filter.Filter(snap, filter.Options{
		HTTPRouteName: "does-not-exist", HTTPRouteNamespace: "kgateway-system", RuleIndex: filter.AllRules,
	})
	if len(none.Listeners) != 0 {
		t.Errorf("expected empty snapshot, got %d listeners", len(none.Listeners))
	}
}

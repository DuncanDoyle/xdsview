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

// httpRoute is one HTTPRoute of a scenario and the number of Envoy routes each of
// its rules produces, counted from the scenario's manifests: matches per rule
// times the virtual hosts (listener hostnames) the route attaches to.
type httpRoute struct {
	name, namespace string
	routesPerRule   []int
}

// scenarioRoutes lists the HTTPRoutes of every recorded OSS scenario.
var scenarioRoutes = map[string][]httpRoute{
	"00-simple":          {{"api-example-com", "kgateway-system", []int{1}}},
	"01_1-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_2-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_3-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_4-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_5-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_6-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_7-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_8-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"01_9-matchers":      {{"api-example-com", "kgateway-system", []int{1}}},
	"02_1-single-policy": {{"api-example-com", "kgateway-system", []int{2}}}, // two HTTPS hostnames
	"02_2-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_3-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_4-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_5-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_6-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_7-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	"02_8-single-policy": {{"api-example-com", "kgateway-system", []int{1}}},
	// The second name extends the first, so a prefix-only match would keep both.
	"06-multi-extproc": {
		{"api-example-com", "kgateway-system", []int{1}},
		{"api-example-com-admin", "kgateway-system", []int{1}},
	},
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

// TestFilter_E2E_OSSScenarios checks, for every OSS scenario and kgateway
// version, that filtering keeps exactly the routes each HTTPRoute (and rule)
// produces and nothing from other HTTPRoutes. Passing on every version shows the
// route naming convention still holds there.
func TestFilter_E2E_OSSScenarios(t *testing.T) {
	for scenario, routes := range scenarioRoutes {
		for _, version := range ossVersions {
			t.Run(scenario+"/"+version, func(t *testing.T) {
				snap := parseDump(t, filepath.Join("../../../testdata/scenarios", scenario, "envoy", version, "config_dump.json"))
				all := routeNames(snap)

				for _, hr := range routes {
					marker := "httproute-" + hr.name + "-" + hr.namespace + "-"
					want := 0
					for _, n := range hr.routesPerRule {
						want += n
					}

					kept := routeNames(filter.Filter(snap, filter.Options{
						HTTPRouteName: hr.name, HTTPRouteNamespace: hr.namespace, RuleIndex: filter.AllRules,
					}))
					if len(kept) != want {
						t.Errorf("%s: kept %d routes, want %d: %v", hr.name, len(kept), want, kept)
					}
					for _, name := range kept {
						if !strings.Contains(name, marker) {
							t.Errorf("%s: kept a route of another HTTPRoute: %s", hr.name, name)
						}
						if !slices.Contains(all, name) {
							t.Errorf("%s: kept route %s is not in the dump", hr.name, name)
						}
					}

					for rule, n := range hr.routesPerRule {
						byRule := routeNames(filter.Filter(snap, filter.Options{
							HTTPRouteName: hr.name, HTTPRouteNamespace: hr.namespace, RuleIndex: rule,
						}))
						if len(byRule) != n {
							t.Errorf("%s rule %d: kept %d routes, want %d: %v", hr.name, rule, len(byRule), n, byRule)
						}
					}
					outOfRange := filter.Filter(snap, filter.Options{
						HTTPRouteName: hr.name, HTTPRouteNamespace: hr.namespace, RuleIndex: len(hr.routesPerRule),
					})
					if names := routeNames(outOfRange); len(names) != 0 {
						t.Errorf("%s rule %d (no such rule): kept %v", hr.name, len(hr.routesPerRule), names)
					}
				}
			})
		}
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

// TestFilter_E2E_Enterprise keeps the enterprise 00-simple check, whose HTTPRoute
// lives in namespace "default".
func TestFilter_E2E_Enterprise(t *testing.T) {
	snap := parseDump(t, "../../../testdata/enterprise/scenarios/00-simple/envoy/config_dump.json")
	kept := routeNames(filter.Filter(snap, filter.Options{
		HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: filter.AllRules,
	}))
	want := []string{"listener~80~api_example_com-route-0-httproute-api-example-com-default-0-0-matcher-0"}
	if !slices.Equal(kept, want) {
		t.Errorf("kept %v, want %v", kept, want)
	}

	none := filter.Filter(snap, filter.Options{
		HTTPRouteName: "does-not-exist", HTTPRouteNamespace: "default", RuleIndex: filter.AllRules,
	})
	if len(none.Listeners) != 0 {
		t.Errorf("expected empty snapshot, got %d listeners", len(none.Listeners))
	}
}

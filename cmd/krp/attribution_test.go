package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

const (
	filterConfigType = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
	extAuthzType     = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"
	extProcType      = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"
	dynModType       = "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"
	setMetaType      = "type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config"
	matcherType      = "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	// xformType is a downstream transformation filter that the core does not know.
	xformType = "type.googleapis.com/example.transformation.v1.Transformation"
)

func parseFile(t *testing.T, path string) *model.Snapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := parser.Parse(f, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func parseString(t *testing.T, s string) *model.Snapshot {
	t.Helper()
	res, err := parser.Parse(strings.NewReader(s), parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// recordings returns a scenario's OSS dumps, keyed by version directory.
func recordings(t *testing.T, scenario string) map[string]string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(testdataDir, "scenarios", scenario, "envoy/kgateway-*/config_dump.json"))
	if len(paths) != 2 {
		t.Fatalf("%s: got %d recordings, want 2", scenario, len(paths))
	}
	out := map[string]string{}
	for _, p := range paths {
		out[filepath.Base(filepath.Dir(p))] = p
	}
	return out
}

// attributed is one route as krp's pipeline leaves it: origins on the route,
// its chain, and its contributions by filter name.
type attributed struct {
	route  model.Route
	chain  model.EffectiveChain
	byName map[string][]model.Contribution
}

// pipeline runs krp's steps on snap with m: origins, chains, contributions.
// It returns every route; hand-built snapshots get IDs first.
func pipeline(snap *model.Snapshot, m origin.Mapper) []attributed {
	snaptest.IDs(snap)
	if m != nil {
		snap = origin.Annotate(snap, m)
	}
	chains := chain.Effective(snap)
	contribs := attribute(snap, chains, m)
	var out []attributed
	for _, l := range snap.Listeners {
		if l.Static {
			continue // the corpus's bootstrap listeners
		}
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					a := attributed{route: r, chain: chains[r.ID], byName: map[string][]model.Contribution{}}
					for _, f := range fc.HCM.HTTPFilters {
						a.byName[f.Name] = contribs[r.ID][f.ID]
					}
					out = append(out, a)
				}
			}
		}
	}
	return out
}

// composed is pipeline's first route.
func composed(t *testing.T, snap *model.Snapshot, m origin.Mapper) attributed {
	t.Helper()
	rs := pipeline(snap, m)
	if len(rs) == 0 {
		t.Fatal("no routes")
	}
	return rs[0]
}

func tp(ns, name, field string) model.Contribution {
	return model.Contribution{Kind: model.ContributionPolicy, Field: field, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: ns, Name: name}}
}

func gatewayExtension(ns, name string) model.Contribution {
	return model.Contribution{Kind: model.ContributionExtension, Certainty: model.Inferred,
		Ref: model.Ref{Group: "gateway.kgateway.dev", Kind: "GatewayExtension", Namespace: ns, Name: name}}
}

func merge(fields map[string]string) map[string]any {
	m := map[string]any{}
	for field, name := range fields {
		m[field] = []any{"gateway.kgateway.dev/TrafficPolicy/ns/" + name}
	}
	return map[string]any{"merge.TrafficPolicy.gateway.kgateway.dev": m}
}

// mergeID is "merge.<kind>.<group>" metadata naming one policy for field.
func mergeID(group, kind, field string) map[string]any {
	return map[string]any{"merge." + kind + "." + group: map[string]any{field: []any{group + "/" + kind + "/ns/p"}}}
}

// oneHCM is a listener with one HCM over filters and one route r, with
// per-filter config rcPFC at route config level.
func oneHCM(filters []model.HTTPFilter, rcPFC map[string]any, r model.Route) *model.Snapshot {
	return &model.Snapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		HTTPFilters: filters,
		RouteConfig: &model.RouteConfig{TypedPerFilterConfig: rcPFC, VirtualHosts: []model.VirtualHost{{Routes: []model.Route{r}}}},
	}}}}}}
}

// oneFilter is one route that enables filter f per route and carries meta.
func oneFilter(f model.HTTPFilter, meta map[string]any) *model.Snapshot {
	f.Disabled = true
	return oneHCM([]model.HTTPFilter{f}, nil, model.Route{
		TypedPerFilterConfig: map[string]any{f.Name: map[string]any{"@type": filterConfigType, "config": map[string]any{}}},
		Metadata:             meta,
	})
}

func hasPolicy(r model.Route, want model.PolicyRef) bool {
	for _, p := range r.Policies {
		if p == want {
			return true
		}
	}
	return false
}

func policyCount(a attributed) int {
	n := 0
	for _, cts := range a.byName {
		for _, c := range cts {
			if c.Kind == model.ContributionPolicy {
				n++
			}
		}
	}
	return n
}

// pluginDump has a vendor filter enabled per route by a vendor policy field,
// and an OSS ext auth filter enabled by a TrafficPolicy. Two listeners share the route config.
const pluginDump = `{"configs": [
 {"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
  {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [` + pluginHCM + `]}]}}},
  {"name": "listener~81", "active_state": {"listener": {"name": "listener~81", "filter_chains": [{"name": "listener~81", "filters": [` + pluginHCM + `]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "rc",
  "virtual_hosts": [{"name": "vh", "domains": ["*"], "routes": [{"name": "r", "match": {"prefix": "/"}, "route": {"cluster": "c"},
   "typed_per_filter_config": {
    "acme_waf/ns": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}},
    "ext_auth/ns/authz": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}},
   "metadata": {"filter_metadata": {
    "merge.AcmePolicy.acme.io": {"acmeWaf": ["acme.io/AcmePolicy/ns/waf"]},
    "merge.TrafficPolicy.gateway.kgateway.dev": {"extAuth": ["gateway.kgateway.dev/TrafficPolicy/ns/auth"]}}}}]}]}}]}]}`

const pluginHCM = `{"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "rc"}, "http_filters": [
 {"name": "acme_waf/ns", "disabled": true, "typed_config": {"@type": "type.googleapis.com/acme.waf.v1.Waf"}},
 {"name": "ext_auth/ns/authz", "disabled": true, "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"}},
 {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}`

// acmeMapper is a vendor plugin. It maps nothing but its own policy field.
type acmeMapper struct{ origin.Mapper }

func (acmeMapper) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "acmeWaf" && f.Type == "type.googleapis.com/acme.waf.v1.Waf"
}

// fieldless implements Mapper but not PolicyFilters.
type fieldless struct{ origin.Mapper }

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

// unsureAcme is a plugin whose policy records carry no certainty.
type unsureAcme struct{ origin.Mapper }

func (unsureAcme) Policies(model.Route) []model.PolicyRef {
	return []model.PolicyRef{{Policy: model.Ref{Group: "acme.io", Kind: "AcmePolicy", Namespace: "ns", Name: "waf"}, Field: "acmeWaf"}}
}

func (unsureAcme) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "acmeWaf" && f.Type == "type.googleapis.com/acme.waf.v1.Waf"
}

// xformPlugin is a downstream plugin that claims the vendor transformation type.
type xformPlugin struct{ origin.Mapper }

func (xformPlugin) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "transformation" && f.Type == xformType
}

// TestAttribute_Corpus03 checks the contributions on the recorded
// 03-multi-policy dumps; without a mapper there are none.
func TestAttribute_Corpus03(t *testing.T) {
	ns := "kgateway-system"
	want := map[string][]model.Contribution{
		"global_disable/ext_auth":             nil,
		"envoy.filters.http.cors":             {tp(ns, "cors", "cors")},
		"ext_auth/kgateway-system/ext-authz":  {gatewayExtension(ns, "ext-authz"), tp(ns, "ext-auth", "extAuth")},
		"ratelimit/kgateway-system/ratelimit": {gatewayExtension(ns, "ratelimit"), tp(ns, "rate-limit", "rateLimit.global")},
		"envoy.filters.http.router":           nil,
	}
	for version, path := range recordings(t, "03-multi-policy") {
		if got := composed(t, parseFile(t, path), origin.Kgateway(origin.KgatewayOptions{})).byName; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: contributions =\n%+v\nwant\n%+v", version, got, want)
		}
		if n := policyCount(composed(t, parseFile(t, path), nil)); n != 0 {
			t.Errorf("%s: no mapper: %d policy contributions", version, n)
		}
	}
}

// TestAttribute_Contributions covers which mappers attribute which entries.
func TestAttribute_Contributions(t *testing.T) {
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	auth := tp("ns", "auth", "extAuth")
	ext := gatewayExtension("ns", "authz")
	waf := model.Contribution{Kind: model.ContributionPolicy, Field: "acmeWaf", Certainty: model.Inferred,
		Ref: model.Ref{Group: "acme.io", Kind: "AcmePolicy", Namespace: "ns", Name: "waf"}}

	tests := []struct {
		name string
		m    origin.Mapper
		want map[string][]model.Contribution
	}{
		{"kgateway: unmapped vendor field attributes nothing", kgw,
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
		{"plugin first in a chain adds its field", origin.Chain(acmeMapper{kgw}, kgw),
			map[string][]model.Contribution{"acme_waf/ns": {waf}, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
		{"mapper without PolicyFilters: extensions only", fieldless{kgw},
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": {ext}, "envoy.filters.http.router": nil}},
		{"nil mapper", nil,
			map[string][]model.Contribution{"acme_waf/ns": nil, "ext_auth/ns/authz": nil, "envoy.filters.http.router": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := pipeline(parseString(t, pluginDump), tt.m)
			if len(rs) != 2 {
				t.Fatalf("got %d routes, want 2 (one per HCM)", len(rs))
			}
			for _, r := range rs {
				if !reflect.DeepEqual(r.byName, tt.want) {
					t.Errorf("route %s: contributions = %+v\nwant %+v", r.route.ID, r.byName, tt.want)
				}
			}
		})
	}
}

// TestAttribute_PolicyNeedsPerRouteLevel checks that a policy isn't attributed to
// an entry that runs on the HCM default, even when the field matches its kind.
func TestAttribute_PolicyNeedsPerRouteLevel(t *testing.T) {
	dump := strings.Replace(pluginDump, `"ext_auth/ns/authz": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}`, `"unused": {}`, 1)
	dump = strings.ReplaceAll(dump, `{"name": "ext_auth/ns/authz", "disabled": true,`, `{"name": "ext_auth/ns/authz",`)
	got := composed(t, parseString(t, dump), origin.Kgateway(origin.KgatewayOptions{})).byName["ext_auth/ns/authz"]
	if want := []model.Contribution{gatewayExtension("ns", "authz")}; !reflect.DeepEqual(got, want) {
		t.Errorf("contributions = %+v, want the extension only %+v", got, want)
	}
}

// TestAttribute_PolicyNeedsOneMatch: with two ext_authz instances configured
// for the route, the extAuth policy can't be placed, so neither gets it.
func TestAttribute_PolicyNeedsOneMatch(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	snap := func(both bool) *model.Snapshot {
		pfc := map[string]any{"ext_auth/ns/a": enable}
		if both {
			pfc["ext_auth/ns/b"] = enable
		}
		return oneHCM([]model.HTTPFilter{
			{Name: "ext_auth/ns/a", Type: extAuthzType, Disabled: true},
			{Name: "ext_auth/ns/b", Type: extAuthzType, Disabled: true},
		}, nil, model.Route{TypedPerFilterConfig: pfc, Metadata: merge(map[string]string{"extAuth": "auth"})})
	}
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	if got := composed(t, snap(false), kgw).byName["ext_auth/ns/a"]; len(got) != 2 || got[1] != tp("ns", "auth", "extAuth") {
		t.Errorf("one instance: contributions = %+v, want the extension and the policy", got)
	}
	if n := policyCount(composed(t, snap(true), kgw)); n != 0 {
		t.Errorf("two instances: %d policy contributions, want none", n)
	}
}

// TestCertainty_Contributions: the OSS mapper's extension and policy
// contributions are inferred; a plugin's exact extension origin stays exact;
// Route.Policies refs read from metadata are exact.
func TestCertainty_Contributions(t *testing.T) {
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	auth := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "auth"}

	r := composed(t, parseString(t, pluginDump), kgw)
	want := []model.Contribution{gatewayExtension("ns", "authz"), tp("ns", "auth", "extAuth")}
	if got := r.byName["ext_auth/ns/authz"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ext_auth contributions =\n %+v\nwant\n %+v", got, want)
	}
	if !hasPolicy(r.route, model.PolicyRef{Policy: auth, Field: "extAuth", Certainty: model.Exact}) {
		t.Errorf("Route.Policies = %+v, want exact %v extAuth", r.route.Policies, auth)
	}

	r = composed(t, parseString(t, pluginDump), origin.Chain(exactWaf{kgw}, kgw))
	wantWaf := model.Contribution{Kind: model.ContributionExtension, Certainty: model.Exact,
		Ref: model.Ref{Group: "acme.io", Kind: "WafExtension", Namespace: "ns", Name: "waf"}}
	if got := r.byName["acme_waf/ns"]; len(got) == 0 || !reflect.DeepEqual(got[0], wantWaf) {
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
			snap := oneHCM([]model.HTTPFilter{
				{Name: c.helper, Type: setMetaType, Disabled: true},
				{Name: c.provider, Type: c.typ, Disabled: true},
			}, nil, model.Route{
				TypedPerFilterConfig: map[string]any{c.helper: enable},
				Metadata:             merge(map[string]string{c.field: "off"}),
			})
			rc := snap.Listeners[0].FilterChains[0].HCM.RouteConfig
			if level == "routeConfig" {
				rc.TypedPerFilterConfig = map[string]any{c.provider: enable}
			} else {
				rc.VirtualHosts[0].TypedPerFilterConfig = map[string]any{c.provider: enable}
			}
			r := composed(t, snap, origin.Kgateway(origin.KgatewayOptions{}))
			if n := policyCount(r); n != 0 {
				t.Errorf("%s, provider at %s: %d policy contributions, want none: %+v", c.field, level, n, r.byName)
			}
			off := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "off"}
			if !hasPolicy(r.route, model.PolicyRef{Policy: off, Field: c.field, Certainty: model.Exact}) {
				t.Errorf("%s, provider at %s: Route.Policies = %+v, want %v", c.field, level, r.route.Policies, off)
			}
		}
	}
}

// TestCertainty_PolicyLinkNoStrongerThanRef: a unique filter match adds no
// evidence for the policy's identity, so an unknown ref stays unknown (8.1 review #3).
func TestCertainty_PolicyLinkNoStrongerThanRef(t *testing.T) {
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	got := composed(t, parseString(t, pluginDump), origin.Chain(unsureAcme{kgw}, kgw)).byName["acme_waf/ns"]
	if len(got) != 1 || got[0].Certainty != model.Unknown {
		t.Errorf("acme_waf contributions = %+v, want one with unknown certainty", got)
	}
}

// TestCertainty_BackendOverrideIsACandidate: a matching filter that a weighted
// backend configures counts as a candidate, so the route-level one alone
// doesn't get the policy (8.1 review #4).
func TestCertainty_BackendOverrideIsACandidate(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	snap := func(backendFilter string) *model.Snapshot {
		return oneHCM([]model.HTTPFilter{
			{Name: "ext_auth/ns/a", Type: extAuthzType, Disabled: true},
			{Name: "ext_auth/ns/b", Type: extAuthzType, Disabled: true},
		}, nil, model.Route{
			TypedPerFilterConfig: map[string]any{"ext_auth/ns/a": enable},
			WeightedClusters:     []model.WeightedCluster{{Name: "c", Weight: 1, TypedPerFilterConfig: map[string]any{backendFilter: enable}}},
			Metadata:             merge(map[string]string{"extAuth": "auth"}),
		})
	}
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	if got := policyCount(composed(t, snap("ext_auth/ns/b"), kgw)); got != 0 {
		t.Errorf("backend configures another match: %d policy contributions, want 0", got)
	}
	if got := policyCount(composed(t, snap("ext_auth/ns/a"), kgw)); got != 1 {
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
	snap := origin.Annotate(parseString(t, dump), m)
	chains := chain.Effective(snap)
	contribs := attribute(snap, chains, m)

	var js bytes.Buffer
	if err := writeJSON(&js, snap, chains, contribs, nil); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, js.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"indexCertainty":"inferred"`, `"name":"waf"},"certainty":"exact"`} {
		if !strings.Contains(compact.String(), want) {
			t.Errorf("JSON lacks %s:\n%s", want, compact.String())
		}
	}
	var out bytes.Buffer
	if err := render.Text(&out, snap, chains, render.Options{Labels: contribs.labels}); err != nil {
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

// TestAttribute_RowsPerRoute: one slot under two routes with different
// per-filter config and policies prints each route's own labels on its row.
func TestAttribute_RowsPerRoute(t *testing.T) {
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	snap := oneHCM([]model.HTTPFilter{{Name: "ext_auth/ns/authz", Type: extAuthzType, Disabled: true}}, nil,
		model.Route{Match: model.RouteMatch{Prefix: "/a"}, TypedPerFilterConfig: map[string]any{"ext_auth/ns/authz": enable},
			Metadata: merge(map[string]string{"extAuth": "a"})})
	vh := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0]
	vh.Routes = append(vh.Routes, model.Route{Match: model.RouteMatch{Prefix: "/b"},
		TypedPerFilterConfig: map[string]any{"ext_auth/ns/authz": map[string]any{"@type": filterConfigType, "disabled": true}},
		Metadata:             merge(map[string]string{"extAuth": "b"})})
	snaptest.IDs(snap)
	m := origin.Kgateway(origin.KgatewayOptions{})
	chains := chain.Effective(snap)
	var out bytes.Buffer
	if err := render.Text(&out, snap, chains, render.Options{AllFilters: true, Labels: attribute(snap, chains, m).labels}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"1 ext_auth/ns/authz  enabled by route · GatewayExtension ns/authz (inferred) · TrafficPolicy ns/a (extAuth, inferred)\n",
		"1 ext_auth/ns/authz  off · GatewayExtension ns/authz (inferred) · TrafficPolicy ns/b (extAuth, inferred)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no row %q in\n%s", want, out.String())
		}
	}
}

// TestAttribute_Corpus09 is kgateway v2.5.1 from kgwctl's fixtures (#28): local
// rate limit, the OSS transformation filter, and an ext auth disable.
func TestAttribute_Corpus09(t *testing.T) {
	snap := parseFile(t, filepath.Join(testdataDir, "scenarios/09-kgwctl-policies/envoy/kgateway-v2.5.1/config_dump.json"))
	ext := gatewayExtension("kgwctl-proxy", "authz")
	want := map[string]map[string][]model.Contribution{
		"orders_kgwctl_example-route-0-httproute-orders-": {
			"global_disable/ext_auth":          nil,
			"ext_auth/kgwctl-proxy/authz":      {ext, tp("kgwctl-proxy", "orders-extauth", "extAuth")},
			"ratelimit/local":                  {tp("kgwctl-proxy", "orders-ratelimit", "rateLimit.local")},
			"dynamic_modules/simple_mutations": {tp("kgwctl-proxy", "orders-transformation", "transformation")},
			"envoy.filters.http.router":        nil,
		},
		"public_kgwctl_example-route-0-httproute-public-": {
			"global_disable/ext_auth":          {tp("kgwctl-proxy", "public-no-extauth", "extAuth")}, // the provider has no per-filter config here: one candidate
			"ext_auth/kgwctl-proxy/authz":      {ext},
			"ratelimit/local":                  nil,
			"dynamic_modules/simple_mutations": nil,
			"envoy.filters.http.router":        nil,
		},
	}
	seen := 0
	for _, r := range pipeline(snap, origin.Kgateway(origin.KgatewayOptions{})) {
		for key, w := range want {
			if !strings.Contains(r.route.Name, key) {
				continue
			}
			seen++
			if !reflect.DeepEqual(r.byName, w) {
				t.Errorf("%s:\n got %+v\nwant %+v", key, r.byName, w)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("matched %d routes, want %d", seen, len(want))
	}
}

// TestAttribute_Corpus02_1Transformation: OSS kgateway's transformation filter
// is the dynamic module, on v2.4.7 and v2.5.0 alike.
func TestAttribute_Corpus02_1Transformation(t *testing.T) {
	want := []model.Contribution{tp("kgateway-system", "transformation", "transformation")}
	for version, path := range recordings(t, "02_1-single-policy") {
		rs := pipeline(parseFile(t, path), origin.Kgateway(origin.KgatewayOptions{}))
		if len(rs) == 0 {
			t.Fatalf("%s: no routes", version)
		}
		for _, r := range rs {
			if got := r.byName["dynamic_modules/simple_mutations"]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s: transformation contributions = %+v, want %+v", version, r.route.Name, got, want)
			}
		}
	}
}

// TestAttribute_TransformationBesideAuthMetadata: kgateway's ext_auth_enabled
// helper is the same rustformation module; transformation still goes to
// dynamic_modules/simple_mutations (kgateway v2.5.1 trafficpolicy/utils.go).
func TestAttribute_TransformationBesideAuthMetadata(t *testing.T) {
	rust := rustformation("rust_module", "rustformation")
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	got := composed(t, oneHCM([]model.HTTPFilter{
		{Name: "ext_auth_enabled", Type: dynModType, TypedConfig: rust, Disabled: true},
		{Name: "dynamic_modules/simple_mutations", Type: dynModType, TypedConfig: rust, Disabled: true},
	}, nil, model.Route{
		TypedPerFilterConfig: map[string]any{"ext_auth_enabled": enable, "dynamic_modules/simple_mutations": enable},
		Metadata:             merge(map[string]string{"transformation": "tx"}),
	}), origin.Kgateway(origin.KgatewayOptions{})).byName
	if want := []model.Contribution{tp("ns", "tx", "transformation")}; !reflect.DeepEqual(got["dynamic_modules/simple_mutations"], want) || got["ext_auth_enabled"] != nil {
		t.Errorf("contributions = %+v, want transformation on simple_mutations only", got)
	}
}

// TestVendorNeutral_PolicyKind: krp links only kgateway's TrafficPolicy.
// Another group or kind stays listed on Route.Policies, unlinked.
func TestVendorNeutral_PolicyKind(t *testing.T) {
	authz := model.HTTPFilter{Name: "ext_auth/ns/authz", Type: extAuthzType}
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	for _, c := range []struct {
		group, kind string
		linked      bool
	}{
		{"gateway.kgateway.dev", "TrafficPolicy", true},
		{"example.com", "TrafficPolicy", false},
		{"gateway.kgateway.dev", "AcmePolicy", false},
	} {
		ref := model.Ref{Group: c.group, Kind: c.kind, Namespace: "ns", Name: "p"}
		r := composed(t, oneFilter(authz, mergeID(c.group, c.kind, "extAuth")), kgw)
		var got []model.Contribution
		for _, ct := range r.byName[authz.Name] {
			if ct.Kind == model.ContributionPolicy {
				got = append(got, ct)
			}
		}
		var want []model.Contribution
		if c.linked {
			want = []model.Contribution{{Kind: model.ContributionPolicy, Ref: ref, Field: "extAuth", Certainty: model.Inferred}}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s/%s: policy contributions = %+v, want %+v", c.group, c.kind, got, want)
		}
		if !hasPolicy(r.route, model.PolicyRef{Policy: ref, Field: "extAuth", Certainty: model.Exact}) {
			t.Errorf("%s/%s: Route.Policies = %+v, want %v listed", c.group, c.kind, r.route.Policies, ref)
		}
	}
}

// TestVendorNeutral_Transformation: the core does not know the vendor
// transformation type, so its sides are unknown and it gets no policy link.
// A plugin in origin.Chain that claims the type adds the link back.
func TestVendorNeutral_Transformation(t *testing.T) {
	xf := model.HTTPFilter{Name: "transformation", Type: xformType}
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	ref := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "p"}
	kgw := origin.Kgateway(origin.KgatewayOptions{})

	r := composed(t, oneFilter(xf, meta), kgw)
	if got := r.byName[xf.Name]; len(got) != 0 {
		t.Errorf("core: contributions = %+v, want none", got)
	}
	if e := r.chain.Filters[0]; e.Request != model.TriUnknown || e.Response != model.TriUnknown {
		t.Errorf("core: sides = %v/%v, want unknown/unknown", e.Request, e.Response)
	}

	r = composed(t, oneFilter(xf, meta), origin.Chain(xformPlugin{kgw}, kgw))
	want := []model.Contribution{{Kind: model.ContributionPolicy, Ref: ref, Field: "transformation", Certainty: model.Inferred}}
	if got := r.byName[xf.Name]; !reflect.DeepEqual(got, want) {
		t.Errorf("with plugin: contributions = %+v, want %+v", got, want)
	}
}

// rustformation is the typed config of OSS kgateway's transformation filter,
// with the module library and filter name given.
func rustformation(library, filter string) map[string]any {
	return map[string]any{"@type": dynModType, "dynamic_module_config": map[string]any{"name": library}, "filter_name": filter}
}

// TestVendorNeutral_TransformationEvidence: the OSS link needs the instance
// name, Envoy's DynamicModuleFilter type, and the rustformation module in
// rust_module, also behind a matcher wrapper (phase review #3, fix reviews 2 #4, 3 #3–#5).
func TestVendorNeutral_TransformationEvidence(t *testing.T) {
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	wrap := func(tc map[string]any) map[string]any {
		return map[string]any{"@type": matcherType, "extension_config": map[string]any{"name": "inner", "typed_config": tc}}
	}
	for _, c := range []struct {
		label, typ string
		tc         map[string]any
		linked     bool
	}{
		{"OSS", dynModType, rustformation("rust_module", "rustformation"), true},
		{"matcher-wrapped", dynModType, wrap(rustformation("rust_module", "rustformation")), true},
		{"lookalike type", "type.googleapis.com/example.filters.http.dynamic_modules.v1.Other", rustformation("rust_module", "rustformation"), false},
		{"downstream type", xformType, rustformation("rust_module", "rustformation"), false},
		{"other filter", dynModType, rustformation("rust_module", "other"), false},
		{"other library", dynModType, rustformation("custom_module", "rustformation"), false},
		{"wrapped other library", dynModType, wrap(rustformation("custom_module", "rustformation")), false},
		{"no config", dynModType, nil, false},
	} {
		f := model.HTTPFilter{Name: "dynamic_modules/simple_mutations", Type: c.typ, TypedConfig: c.tc}
		got := composed(t, oneFilter(f, meta), kgw).byName[f.Name]
		if (len(got) == 1) != c.linked {
			t.Errorf("%s: contributions %+v, want linked=%v", c.label, got, c.linked)
		}
	}
}

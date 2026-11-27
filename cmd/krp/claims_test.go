package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// These tests run krp's flow (build) and read what it prints on each filter
// row: the extension that defines the filter and the policy records linked to
// it on the route.

const (
	filterConfigType = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
	extAuthzType     = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"
	extProcType      = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"
	dynModType       = "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"
	setMetaType      = "type.googleapis.com/envoy.extensions.filters.http.set_metadata.v3.Config"
	matcherType      = "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	wafType          = "type.googleapis.com/acme.waf.v1.Waf"
	// xformType is a downstream transformation filter that the kgateway module does not know.
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

// attributed is one route as krp shows it: its chain, its filter-row labels
// by filter name, and the policy records that apply to it.
type attributed struct {
	route   model.Route
	chain   model.EffectiveChain
	byName  map[string][]string
	records []provenance.PolicyRecord
}

// pipeline runs build on snap with the plugin modules after kgateway's and
// returns every route of a dynamic listener; hand-built snapshots get IDs first.
func pipeline(t *testing.T, snap *model.Snapshot, plugins ...provenance.Module) []attributed {
	t.Helper()
	snaptest.IDs(snap)
	defer func(m []provenance.Module) { extraModules = m }(extraModules)
	extraModules = plugins
	res, err := build(snap, request{})
	if err != nil {
		t.Fatal(err)
	}
	labels := res.labels()
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
					a := attributed{route: r, chain: res.chains[r.ID], byName: map[string][]string{}}
					for _, f := range fc.HCM.HTTPFilters {
						a.byName[f.Name] = labels(render.Row{Subject: f.ID, Route: r.ID, Slot: f.ID})
					}
					for _, c := range provenance.Records(res.ix, snap, r.ID) {
						a.records = append(a.records, c.Assertion.(provenance.PolicyRecord))
					}
					out = append(out, a)
				}
			}
		}
	}
	return out
}

// composed is pipeline's first route.
func composed(t *testing.T, snap *model.Snapshot, plugins ...provenance.Module) attributed {
	t.Helper()
	rs := pipeline(t, snap, plugins...)
	if len(rs) == 0 {
		t.Fatal("no routes")
	}
	return rs[0]
}

func hasRecord(a attributed, kind, name, field string) bool {
	for _, p := range a.records {
		if p.Policy.Kind == kind && p.Policy.Name == name && p.Field == field {
			return true
		}
	}
	return false
}

// policyCount counts the policy labels on a route's filter rows.
func policyCount(a attributed) int {
	n := 0
	for _, ls := range a.byName {
		for _, l := range ls {
			if strings.Contains(l, "Policy ") {
				n++
			}
		}
	}
	return n
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

// linker is a test vendor module: on every route whose metadata key names a
// policy for field, it records that policy and links it to the one slot of
// filterType. Records carry certainty.
type linker struct {
	name, key, field, filterType string
	certainty                    model.Certainty
}

func (m linker) Name() string { return m.name }

func (m linker) Claims(snap *model.Snapshot) []provenance.Claim {
	var out []provenance.Claim
	n := 0
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			var slot model.ID
			for _, f := range fc.HCM.HTTPFilters {
				if f.Type == m.filterType {
					slot = f.ID
				}
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					fields, _ := r.Metadata[m.key].(map[string]any)
					ids, _ := fields[m.field].([]any)
					if len(ids) == 0 || slot == "" {
						continue
					}
					parts := strings.Split(ids[0].(string), "/")
					ref := model.Ref{Group: parts[0], Kind: parts[1], Namespace: parts[2], Name: parts[3]}
					n++
					rec := string(rune('a' + n))
					out = append(out,
						provenance.Claim{Ref: rec, Subject: r.ID, Certainty: m.certainty,
							Assertion: provenance.PolicyRecord{Policy: ref, Field: m.field, Level: provenance.LevelRoute},
							Evidence:  []provenance.Evidence{{Kind: provenance.EvidenceMetadata, Subject: r.ID, Field: "metadata." + m.key, Object: &ref}}},
						provenance.Claim{Subject: r.ID, Certainty: model.Inferred,
							Assertion: provenance.FilterLink{Record: rec, Slot: slot},
							Evidence:  []provenance.Evidence{{Kind: provenance.EvidenceTable, Subject: slot, Field: "type"}}})
				}
			}
		}
	}
	return out
}

// extension is a test vendor module that claims the slots of one filter
// type: the extension named by the slot's name suffix, at certainty.
type extension struct {
	name, filterType, kind string
	certainty              model.Certainty
}

func (m extension) Name() string { return m.name }

func (m extension) Claims(snap *model.Snapshot) []provenance.Claim {
	var out []provenance.Claim
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil {
				continue
			}
			for _, f := range fc.HCM.HTTPFilters {
				if f.Type != m.filterType {
					continue
				}
				ref := &model.Ref{Group: "acme.io", Kind: m.kind, Namespace: f.Name[strings.LastIndex(f.Name, "/")+1:], Name: "waf"}
				out = append(out, provenance.Claim{Subject: f.ID, Certainty: m.certainty,
					Assertion: provenance.ExtensionSource{Extension: ref},
					Evidence:  []provenance.Evidence{{Kind: provenance.EvidenceStructure, Subject: f.ID, Field: "typedConfig", Object: ref}}})
			}
		}
	}
	return out
}

var (
	acmeLinker = linker{name: "acme", key: "merge.AcmePolicy.acme.io", field: "acmeWaf", filterType: wafType, certainty: model.Exact}
	exactWaf   = extension{name: "acme-ext", filterType: wafType, kind: "WafExtension", certainty: model.Exact}
)

// TestClaims_Corpus03 checks the filter-row labels on the recorded
// 03-multi-policy dumps.
func TestClaims_Corpus03(t *testing.T) {
	want := map[string][]string{
		"global_disable/ext_auth":             nil,
		"envoy.filters.http.cors":             {"TrafficPolicy kgateway-system/cors (cors, inferred)"},
		"ext_auth/kgateway-system/ext-authz":  {"GatewayExtension kgateway-system/ext-authz (inferred)", "TrafficPolicy kgateway-system/ext-auth (extAuth, inferred)"},
		"ratelimit/kgateway-system/ratelimit": {"GatewayExtension kgateway-system/ratelimit (inferred)", "TrafficPolicy kgateway-system/rate-limit (rateLimit.global, inferred)"},
		"envoy.filters.http.router":           nil,
	}
	for version, path := range recordings(t, "03-multi-policy") {
		if got := composed(t, parseFile(t, path)).byName; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: labels =\n%+v\nwant\n%+v", version, got, want)
		}
	}
}

// TestClaims_Plugins covers which modules label which filter rows.
func TestClaims_Plugins(t *testing.T) {
	ext := "GatewayExtension ns/authz (inferred)"
	auth := "TrafficPolicy ns/auth (extAuth, inferred)"
	waf := "AcmePolicy ns/waf (acmeWaf, inferred)"
	tests := []struct {
		name    string
		plugins []provenance.Module
		want    map[string][]string
	}{
		{"kgateway: unmapped vendor field labels nothing", nil,
			map[string][]string{"acme_waf/ns": nil, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
		{"a vendor module adds its field", []provenance.Module{acmeLinker},
			map[string][]string{"acme_waf/ns": {waf}, "ext_auth/ns/authz": {ext, auth}, "envoy.filters.http.router": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := pipeline(t, parseString(t, pluginDump), tt.plugins...)
			if len(rs) != 2 {
				t.Fatalf("got %d routes, want 2 (one per HCM)", len(rs))
			}
			for _, r := range rs {
				if !reflect.DeepEqual(r.byName, tt.want) {
					t.Errorf("route %s: labels = %+v\nwant %+v", r.route.ID, r.byName, tt.want)
				}
			}
		})
	}
}

// TestClaims_PolicyNeedsPerRouteLevel checks that a policy isn't linked to
// an entry that runs on the HCM default, even when the field matches its kind.
func TestClaims_PolicyNeedsPerRouteLevel(t *testing.T) {
	dump := strings.Replace(pluginDump, `"ext_auth/ns/authz": {"@type": "type.googleapis.com/envoy.config.route.v3.FilterConfig", "config": {}}`, `"unused": {}`, 1)
	dump = strings.ReplaceAll(dump, `{"name": "ext_auth/ns/authz", "disabled": true,`, `{"name": "ext_auth/ns/authz",`)
	got := composed(t, parseString(t, dump)).byName["ext_auth/ns/authz"]
	if want := []string{"GatewayExtension ns/authz (inferred)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels = %+v, want the extension only %+v", got, want)
	}
}

// TestClaims_PolicyNeedsOneMatch: with two ext_authz instances configured
// for the route, the extAuth policy can't be placed, so neither gets it.
func TestClaims_PolicyNeedsOneMatch(t *testing.T) {
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
	if got := composed(t, snap(false)).byName["ext_auth/ns/a"]; len(got) != 2 || got[1] != "TrafficPolicy ns/auth (extAuth, inferred)" {
		t.Errorf("one instance: labels = %+v, want the extension and the policy", got)
	}
	if n := policyCount(composed(t, snap(true))); n != 0 {
		t.Errorf("two instances: %d policy labels, want none", n)
	}
}

// TestCertainty_Labels: the kgateway module's extension and link labels are
// inferred, its route records exact; a vendor module's exact extension prints bare.
func TestCertainty_Labels(t *testing.T) {
	r := composed(t, parseString(t, pluginDump))
	want := []string{"GatewayExtension ns/authz (inferred)", "TrafficPolicy ns/auth (extAuth, inferred)"}
	if got := r.byName["ext_auth/ns/authz"]; !reflect.DeepEqual(got, want) {
		t.Errorf("ext_auth labels = %+v, want %+v", got, want)
	}
	if !hasRecord(r, "TrafficPolicy", "auth", "extAuth") {
		t.Errorf("records = %+v, want TrafficPolicy auth extAuth", r.records)
	}
	r = composed(t, parseString(t, pluginDump), exactWaf)
	if got := r.byName["acme_waf/ns"]; len(got) == 0 || got[0] != "WafExtension ns/waf" {
		t.Errorf("acme_waf labels = %+v, want first the bare exact extension", got)
	}
}

// TestCertainty_OneCandidate: a policy field is linked only when exactly one
// entry with per-filter config matches it, at any level. A route-level disable
// helper beside a provider configured at a wider level is two candidates: no
// link, and the record stays. Covers ext_proc's helper too.
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
			r := composed(t, snap)
			if n := policyCount(r); n != 0 {
				t.Errorf("%s, provider at %s: %d policy labels, want none: %+v", c.field, level, n, r.byName)
			}
			if !hasRecord(r, "TrafficPolicy", "off", c.field) {
				t.Errorf("%s, provider at %s: records = %+v, want TrafficPolicy off", c.field, level, r.records)
			}
		}
	}
}

// TestCertainty_PolicyLinkNoStrongerThanRecord: a unique filter match adds no
// evidence for the policy's identity, so an unknown record stays unknown (8.1 review #3).
func TestCertainty_PolicyLinkNoStrongerThanRecord(t *testing.T) {
	unsure := acmeLinker
	unsure.certainty = model.Unknown
	got := composed(t, parseString(t, pluginDump), unsure).byName["acme_waf/ns"]
	if want := []string{"AcmePolicy ns/waf (acmeWaf, unknown)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("acme_waf labels = %+v, want %+v", got, want)
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
	if got := policyCount(composed(t, snap("ext_auth/ns/b"))); got != 0 {
		t.Errorf("backend configures another match: %d policy labels, want 0", got)
	}
	if got := policyCount(composed(t, snap("ext_auth/ns/a"))); got != 1 {
		t.Errorf("backend configures the same filter: %d policy labels, want 1", got)
	}
}

// TestCertainty_ComposedOutput: an exact vendor claim and name-derived route
// indices reach the JSON and text output (8.1 review #7).
func TestCertainty_ComposedOutput(t *testing.T) {
	dump := strings.Replace(pluginDump, `"routes": [{"name": "r",`,
		`"routes": [{"name": "listener~80~*-route-0-httproute-web-ns-1-0-api-matcher-0",`, 1)
	dump = strings.Replace(dump, `"metadata": {"filter_metadata": {`,
		`"metadata": {"filter_metadata": {"dev.kgateway.route_source": {"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "ns", "name": "web", "rule": "api", "match": "_match-0"},`, 1)
	extraModules = []provenance.Module{exactWaf}
	defer func() { extraModules = nil }()
	res, err := build(parseString(t, dump), request{})
	if err != nil {
		t.Fatal(err)
	}
	var js, compact bytes.Buffer
	if err := writeJSON(&js, res); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&compact, js.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"indexCertainty":"inferred"`, `"name":"waf"}},"certainty":"exact"`} {
		if !strings.Contains(compact.String(), want) {
			t.Errorf("JSON lacks %s:\n%s", want, compact.String())
		}
	}
	text := plainText(t, res)
	if !strings.Contains(text, "HTTPRoute ns/web · rule 1 (api) · match 0 (partly from name)") {
		t.Errorf("text lacks the name-derived indices:\n%s", text)
	}
	if !strings.Contains(text, "WafExtension ns/waf") || strings.Contains(text, "WafExtension ns/waf (") {
		t.Errorf("want the exact extension bare:\n%s", text)
	}
}

// TestClaims_Corpus09 is kgateway v2.5.1 from kgwctl's fixtures (#28): local
// rate limit, the OSS transformation filter, and an ext auth disable.
func TestClaims_Corpus09(t *testing.T) {
	snap := parseFile(t, filepath.Join(testdataDir, "scenarios/09-kgwctl-policies/envoy/kgateway-v2.5.1/config_dump.json"))
	ext := "GatewayExtension kgwctl-proxy/authz (inferred)"
	want := map[string]map[string][]string{
		"orders_kgwctl_example-route-0-httproute-orders-": {
			"global_disable/ext_auth":          nil,
			"ext_auth/kgwctl-proxy/authz":      {ext, "TrafficPolicy kgwctl-proxy/orders-extauth (extAuth, inferred)"},
			"ratelimit/local":                  {"TrafficPolicy kgwctl-proxy/orders-ratelimit (rateLimit.local, inferred)"},
			"dynamic_modules/simple_mutations": {"TrafficPolicy kgwctl-proxy/orders-transformation (transformation, inferred)"},
			"envoy.filters.http.router":        nil,
		},
		"public_kgwctl_example-route-0-httproute-public-": {
			"global_disable/ext_auth":          {"TrafficPolicy kgwctl-proxy/public-no-extauth (extAuth, inferred)"}, // the provider has no per-filter config here: one candidate
			"ext_auth/kgwctl-proxy/authz":      {ext},
			"ratelimit/local":                  nil,
			"dynamic_modules/simple_mutations": nil,
			"envoy.filters.http.router":        nil,
		},
	}
	seen := 0
	for _, r := range pipeline(t, snap) {
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

// TestClaims_Corpus02_1Transformation: OSS kgateway's transformation filter
// is the dynamic module, on v2.4.7 and v2.5.0 alike.
func TestClaims_Corpus02_1Transformation(t *testing.T) {
	want := []string{"TrafficPolicy kgateway-system/transformation (transformation, inferred)"}
	for version, path := range recordings(t, "02_1-single-policy") {
		rs := pipeline(t, parseFile(t, path))
		if len(rs) == 0 {
			t.Fatalf("%s: no routes", version)
		}
		for _, r := range rs {
			if got := r.byName["dynamic_modules/simple_mutations"]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s: transformation labels = %+v, want %+v", version, r.route.Name, got, want)
			}
		}
	}
}

// TestClaims_TransformationBesideAuthMetadata: kgateway's ext_auth_enabled
// helper is the same rustformation module; transformation still goes to
// dynamic_modules/simple_mutations (kgateway v2.5.1 trafficpolicy/utils.go).
func TestClaims_TransformationBesideAuthMetadata(t *testing.T) {
	rust := rustformation("rust_module", "rustformation")
	enable := map[string]any{"@type": filterConfigType, "config": map[string]any{}}
	got := composed(t, oneHCM([]model.HTTPFilter{
		{Name: "ext_auth_enabled", Type: dynModType, TypedConfig: rust, Disabled: true},
		{Name: "dynamic_modules/simple_mutations", Type: dynModType, TypedConfig: rust, Disabled: true},
	}, nil, model.Route{
		TypedPerFilterConfig: map[string]any{"ext_auth_enabled": enable, "dynamic_modules/simple_mutations": enable},
		Metadata:             merge(map[string]string{"transformation": "tx"}),
	})).byName
	if want := []string{"TrafficPolicy ns/tx (transformation, inferred)"}; !reflect.DeepEqual(got["dynamic_modules/simple_mutations"], want) || got["ext_auth_enabled"] != nil {
		t.Errorf("labels = %+v, want transformation on simple_mutations only", got)
	}
}

// TestVendorNeutral_PolicyKind: the kgateway module links only kgateway's
// TrafficPolicy. Another group or kind stays recorded, unlinked.
func TestVendorNeutral_PolicyKind(t *testing.T) {
	authz := model.HTTPFilter{Name: "ext_auth/ns/authz", Type: extAuthzType}
	for _, c := range []struct {
		group, kind string
		linked      bool
	}{
		{"gateway.kgateway.dev", "TrafficPolicy", true},
		{"example.com", "TrafficPolicy", false},
		{"gateway.kgateway.dev", "AcmePolicy", false},
	} {
		r := composed(t, oneFilter(authz, mergeID(c.group, c.kind, "extAuth")))
		if n := policyCount(r); (n == 1) != c.linked {
			t.Errorf("%s/%s: labels = %+v, want linked=%v", c.group, c.kind, r.byName, c.linked)
		}
		if !hasRecord(r, c.kind, "p", "extAuth") {
			t.Errorf("%s/%s: records = %+v, want %s p recorded", c.group, c.kind, r.records, c.kind)
		}
	}
}

// TestVendorNeutral_Transformation: the core does not know the vendor
// transformation type, so its sides are unknown and the kgateway module
// doesn't link it. A vendor module that claims the type adds the link.
func TestVendorNeutral_Transformation(t *testing.T) {
	xf := model.HTTPFilter{Name: "transformation", Type: xformType}
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	r := composed(t, oneFilter(xf, meta))
	if got := r.byName[xf.Name]; len(got) != 0 {
		t.Errorf("kgateway: labels = %+v, want none", got)
	}
	if e := r.chain.Filters[0]; e.Request != model.TriUnknown || e.Response != model.TriUnknown {
		t.Errorf("core: sides = %v/%v, want unknown/unknown", e.Request, e.Response)
	}
	xform := linker{name: "xform", key: "merge.TrafficPolicy.gateway.kgateway.dev", field: "transformation", filterType: xformType, certainty: model.Exact}
	r = composed(t, oneFilter(xf, meta), xform)
	if got, want := r.byName[xf.Name], []string{"TrafficPolicy ns/p (transformation, inferred)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("with a vendor module: labels = %+v, want %+v", got, want)
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
		got := composed(t, oneFilter(f, meta)).byName[f.Name]
		if (len(got) == 1) != c.linked {
			t.Errorf("%s: labels %+v, want linked=%v", c.label, got, c.linked)
		}
	}
}

// wrapper is a test downstream module that wraps the kgateway module: it
// forwards its claims and links the AcmePolicy records kgateway emits, but
// can't place, to the acme_waf slot of the route's HCM.
type wrapper struct{ inner provenance.Module }

func (wrapper) Name() string { return "acme-kgateway" }

func (m wrapper) Claims(snap *model.Snapshot) []provenance.Claim {
	out := m.inner.Claims(snap)
	slot := map[model.ID]model.ID{} // route -> acme_waf slot of the HCM serving it
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, f := range fc.HCM.HTTPFilters {
				if f.Type == wafType {
					for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
						for _, r := range vh.Routes {
							slot[r.ID] = f.ID
						}
					}
				}
			}
		}
	}
	for _, c := range slices.Clone(out) {
		rec, ok := c.Assertion.(provenance.PolicyRecord)
		if !ok || rec.Policy.Kind != "AcmePolicy" || rec.Field != "acmeWaf" || slot[c.Subject] == "" {
			continue
		}
		// Link to the wrapped record by its Ref, found by what it asserts.
		out = append(out, provenance.Claim{Subject: c.Subject, Certainty: model.Inferred,
			Assertion: provenance.FilterLink{Record: c.Ref, Slot: slot[c.Subject]},
			Evidence:  []provenance.Evidence{{Kind: provenance.EvidenceTable, Subject: slot[c.Subject], Field: "type"}}})
	}
	return out
}

// TestCompose_WrappedModule: a module that wraps the kgateway module links a
// record kgateway emitted, so one record serves the route, with no copy.
func TestCompose_WrappedModule(t *testing.T) {
	snap := parseString(t, pluginDump)
	snaptest.IDs(snap)
	ix, err := provenance.Compose(snap, wrapper{inner: kgateway.New()})
	if err != nil {
		t.Fatal(err)
	}
	if w := ix.Warnings(); len(w) != 0 {
		t.Errorf("warnings = %+v, want none", w)
	}
	// The records kgateway itself emits, per route, and the acme_waf slots.
	original := map[model.ID]string{}
	for _, c := range kgateway.New().Claims(snap) {
		if r, ok := c.Assertion.(provenance.PolicyRecord); ok && r.Policy.Kind == "AcmePolicy" {
			original[c.Subject] = c.Ref
		}
	}
	waf := map[model.ID]bool{}
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil {
				continue
			}
			for _, f := range fc.HCM.HTTPFilters {
				waf[f.ID] = f.Type == wafType
			}
		}
	}
	if len(original) != 2 {
		t.Fatalf("kgateway AcmePolicy records = %v, want one per route use", original)
	}
	linked, records := map[model.ID]string{}, map[model.ID]int{}
	for _, c := range ix.Claims() {
		if c.Module != "acme-kgateway" {
			t.Errorf("claim %s: module %q, want the wrapper's name", c.ID, c.Module)
		}
		if r, ok := c.Assertion.(provenance.PolicyRecord); ok && r.Policy.Kind == "AcmePolicy" {
			records[c.Subject]++
		}
		if l, ok := c.Assertion.(provenance.FilterLink); ok && waf[l.Slot] {
			rec, ok := ix.Record(c)
			if !ok || rec.Subject != c.Subject {
				t.Errorf("link %s: record = %+v, %v; want one on %s", c.ID, rec, ok, c.Subject)
			}
			linked[c.Subject] = rec.Ref
		}
	}
	// Each route's link resolves to the record kgateway emitted, not a copy.
	if !reflect.DeepEqual(linked, original) {
		t.Errorf("acme_waf links resolve to %v, want kgateway's records %v", linked, original)
	}
	for subject, n := range records {
		if n != 1 {
			t.Errorf("route %s: %d AcmePolicy records, want 1", subject, n)
		}
	}
}

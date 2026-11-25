package kgateway

import (
	"path"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// PolicyTableVersion is the kgateway version the policy field table was
// checked against (R14). Each entry cites its source; re-read them at every bump.
const PolicyTableVersion = "v2.5.1"

// policyFilterKinds maps a TrafficPolicy field to the Envoy filter kind it
// configures (pkg/kgateway/extensions2/plugins/trafficpolicy).
var policyFilterKinds = map[string]string{
	"extAuth":          "ext_authz",       // extauth_policy.go
	"extProc":          "ext_proc",        // extproc_policy.go
	"rateLimit.global": "ratelimit",       // global_rate_limit_policy.go
	"rateLimit.local":  "local_ratelimit", // local_rate_limit_policy.go
	"cors":             "cors",            // cors_policy.go
}

// globalDisable maps a field to the helper filter kgateway enables per route
// when a policy turns that feature off (extauth_policy.go, extproc_policy.go).
var globalDisable = map[string]string{
	"extAuth": "global_disable/ext_auth",
	"extProc": "global_disable/ext_proc",
}

// transformationField configures the rustformation filter instance
// (traffic_policy_plugin.go).
const transformationField = "transformation"

// PolicyFields lists every TrafficPolicy field the table links to a filter.
func PolicyFields() []string {
	out := []string{transformationField}
	for f := range policyFilterKinds {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

// rustformationFilter is OSS kgateway's transformation filter instance
// (traffic_policy_plugin.go). Its module also backs the ext_auth_enabled
// helper, so the instance name is checked as well as the module.
const rustformationFilter = "dynamic_modules/simple_mutations"

const (
	dynamicModuleType = "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"
	executeActionType = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction"
)

// candidate is a filter a slot may run: the slot's own, or a Composite delegate.
type candidate struct {
	name, typ string
	config    map[string]any
}

// candidates returns f itself, then every delegate its Composite config names
// in an ExecuteFilterAction (Phase 8 #5). f.Type is already the sole delegate's
// type when all delegates share one.
func candidates(f model.HTTPFilter) []candidate {
	out := []candidate{{f.Name, f.Type, f.TypedConfig}}
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if v["@type"] == executeActionType {
				ext, _ := v["typed_config"].(map[string]any)
				tc, _ := ext["typed_config"].(map[string]any)
				name, _ := ext["name"].(string)
				typ, _ := tc["@type"].(string)
				if typ != "" {
					out = append(out, candidate{name, typ, tc})
				}
				return
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(f.TypedConfig)
	return out
}

// configures reports whether a policy's field configures filter f, by the
// table. It knows only kgateway's TrafficPolicy; other kinds get no link.
func configures(p model.Ref, field string, f model.HTTPFilter) bool {
	if p.Group != kgatewayGroup || p.Kind != "TrafficPolicy" {
		return false
	}
	if name, ok := globalDisable[field]; ok && f.Name == name {
		return true
	}
	for i, c := range candidates(f) {
		switch {
		case field == transformationField:
			// A delegate counts when it, or the Composite holding it, is the instance.
			named := c.name == rustformationFilter || (i > 0 && f.Name == rustformationFilter)
			if named && c.typ == dynamicModuleType && runsRustformation(c.config) {
				return true
			}
		case policyFilterKinds[field] != "" && filterKind(c.typ) == policyFilterKinds[field]:
			return true
		}
	}
	return false
}

// filterKind returns the Envoy filter kind of an HTTP filter type, e.g. "ext_authz"
// for "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz".
func filterKind(typ string) string {
	_, rest, ok := strings.Cut(typ, ".filters.http.")
	if !ok {
		return ""
	}
	kind, _, _ := strings.Cut(rest, ".")
	return kind
}

// runsRustformation reports whether tc, or the filter an ExtensionWithMatcher
// holds, loads kgateway's rustformation module. Envoy loads a module from
// dynamic_module_config.module when set, else by name (dynamic_modules.cc,
// newDynamicModuleByConfig); a module that isn't librust_module.so, or a
// remote one, disagrees with the name and gets no link (Phase 8 #4).
func runsRustformation(tc map[string]any) bool {
	if inner, ok := tc["extension_config"].(map[string]any); ok {
		tc, _ = inner["typed_config"].(map[string]any)
	}
	lib, _ := tc["dynamic_module_config"].(map[string]any)
	if tc["filter_name"] != "rustformation" || lib["name"] != "rust_module" {
		return false
	}
	if mod, ok := lib["module"]; ok && mod != nil {
		m, _ := mod.(map[string]any)
		local, _ := m["local"].(map[string]any)
		file, _ := local["filename"].(string)
		return file != "" && path.Base(file) == "librust_module.so"
	}
	return true
}

// extensionFilters are the filter-name prefixes kgateway gives one filter per
// GatewayExtension, as "<prefix>/<ns>/<name>".
var extensionFilters = []string{"ext_auth", "ext_proc", "ratelimit", "jwt"}

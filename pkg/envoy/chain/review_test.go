package chain_test

import (
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const (
	matcherType   = "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	compositeType = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"
)

// matcher wraps typed config tc in an ExtensionWithMatcher.
func matcher(tc map[string]any) map[string]any {
	return map[string]any{"@type": matcherType, "extension_config": map[string]any{"name": "inner", "typed_config": tc}}
}

// TestEffective_Wrappers: a Composite runs on both sides whatever its
// delegate, whose type the parser puts in Type. An ExtensionWithMatcher keeps
// its inner filter's sides. Both are matcher-dependent.
func TestEffective_Wrappers(t *testing.T) {
	yes, no := model.TriYes, model.TriNo
	for _, c := range []struct {
		label   string
		f       model.HTTPFilter
		wrapper string
		sides   [2]model.Tri
	}{
		{"composite behind a matcher", model.HTTPFilter{Name: "c", Type: rbacType, TypedConfig: matcher(map[string]any{"@type": compositeType})},
			model.WrapperComposite, [2]model.Tri{yes, yes}},
		{"bare composite", model.HTTPFilter{Name: "c", Type: compositeType, TypedConfig: map[string]any{"@type": compositeType}},
			model.WrapperComposite, [2]model.Tri{yes, yes}},
		{"matcher around rbac", model.HTTPFilter{Name: "m", Type: rbacType, TypedConfig: matcher(map[string]any{"@type": rbacType})},
			model.WrapperExtensionWithMatcher, [2]model.Tri{yes, no}},
		{"plain filter", model.HTTPFilter{Name: "p", Type: rbacType, TypedConfig: map[string]any{"@type": rbacType}},
			"", [2]model.Tri{yes, no}},
	} {
		e := first(t, oneRoute([]model.HTTPFilter{c.f}, nil, nil, model.Route{})).Filters[0]
		if e.Type != c.f.Type || e.Wrapper != c.wrapper || e.Conditional != (c.wrapper != "") || [2]model.Tri{e.Request, e.Response} != c.sides {
			t.Errorf("%s: Type, Wrapper, Conditional, sides = %q, %q, %v, %v; want %q, %q, %v, %v",
				c.label, e.Type, e.Wrapper, e.Conditional, [2]model.Tri{e.Request, e.Response}, c.f.Type, c.wrapper, c.wrapper != "", c.sides)
		}
	}
}

// TestEffective_FilterConfigExactType: only Envoy's own FilterConfig is the
// wrapper, and a null config counts as unset.
func TestEffective_FilterConfigExactType(t *testing.T) {
	snap := setup(true, at{route: map[string]any{"@type": "type.googleapis.com/acme.envoy.config.route.v3.FilterConfig", "disabled": true}})
	if e := first(t, snap).Filters[0]; e.Runs != model.TriYes || e.State != model.FilterEnabled {
		t.Errorf("look-alike type: Runs, State = %q, %q; want yes, enabled (a filter-specific config)", e.Runs, e.State)
	}
	snap = setup(false, at{route: map[string]any{"@type": filterConfigType, "config": nil}})
	if e := first(t, snap).Filters[0]; e.Runs != model.TriUnknown || e.State != model.FilterUnknown {
		t.Errorf("null config: Runs, State = %q, %q; want unknown, unknown", e.Runs, e.State)
	}
}

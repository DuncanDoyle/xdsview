package chain_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const (
	filterConfigType = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
	extAuthzType     = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz"
	extAuthzPerRoute = "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute"
	routerType       = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	rbacType         = "type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"
)

func disabled() map[string]any { return map[string]any{"@type": filterConfigType, "disabled": true} }
func enabled() map[string]any {
	return map[string]any{"@type": filterConfigType, "config": map[string]any{}}
}
func invalid() map[string]any { return map[string]any{"@type": filterConfigType} }
func specific() map[string]any {
	return map[string]any{"@type": extAuthzPerRoute, "check_settings": map[string]any{}}
}

// at puts a per-filter config entry for the filter "f" at one level.
type at struct{ route, vhost, routeConfig map[string]any }

// hcmSnap is one listener with one HCM over filters and route config rc, with IDs.
func hcmSnap(filters []model.HTTPFilter, rc *model.RouteConfig) *model.Snapshot {
	return snaptest.IDs(&model.Snapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{
		HCM: &model.HCMConfig{HTTPFilters: filters, RouteConfig: rc}}}}}})
}

// oneRoute is hcmSnap with one route r, and per-filter config at route config and vhost level.
func oneRoute(filters []model.HTTPFilter, rcPFC, vhPFC map[string]any, r model.Route) *model.Snapshot {
	return hcmSnap(filters, &model.RouteConfig{Name: "rc", TypedPerFilterConfig: rcPFC,
		VirtualHosts: []model.VirtualHost{{Name: "vh", TypedPerFilterConfig: vhPFC, Routes: []model.Route{r}}}})
}

func setup(hcmDisabled bool, a at) *model.Snapshot {
	pfc := func(v map[string]any) map[string]any {
		if v == nil {
			return nil
		}
		return map[string]any{"f": v}
	}
	return oneRoute([]model.HTTPFilter{{Name: "f", Type: extAuthzType, Disabled: hcmDisabled}},
		pfc(a.routeConfig), pfc(a.vhost), model.Route{Name: "r", TypedPerFilterConfig: pfc(a.route)})
}

// first returns the chain of snap's first route.
func first(t *testing.T, snap *model.Snapshot) model.EffectiveChain {
	t.Helper()
	rs := routes(snap)
	if len(rs) == 0 {
		t.Fatal("no routes")
	}
	c, ok := chain.Effective(snap)[rs[0].ID]
	if !ok {
		t.Fatalf("no chain for route %s", rs[0].ID)
	}
	return c
}

// TestEffective_Semantics has one case per row of the design's semantics table,
// at each level, plus precedence between levels.
func TestEffective_Semantics(t *testing.T) {
	type want struct {
		runs  model.Tri
		state model.FilterState
		level model.ConfigLevel
		typ   string
	}
	yes, no, unknown := model.TriYes, model.TriNo, model.TriUnknown
	tests := []struct {
		name        string
		hcmDisabled bool
		at          at
		want        want
	}{
		{"on, no entry", false, at{}, want{yes, model.FilterOn, "", ""}},
		{"on, disabled by route", false, at{route: disabled()}, want{no, model.FilterDisabled, model.LevelRoute, filterConfigType}},
		{"on, disabled by vhost", false, at{vhost: disabled()}, want{no, model.FilterDisabled, model.LevelVirtualHost, filterConfigType}},
		{"on, disabled by route config", false, at{routeConfig: disabled()}, want{no, model.FilterDisabled, model.LevelRouteConfig, filterConfigType}},
		{"on, config at route", false, at{route: enabled()}, want{yes, model.FilterOn, model.LevelRoute, filterConfigType}},
		{"on, filter-specific at vhost", false, at{vhost: specific()}, want{yes, model.FilterOn, model.LevelVirtualHost, extAuthzPerRoute}},
		{"off, no entry", true, at{}, want{no, model.FilterOff, "", ""}},
		{"off, disabled by route", true, at{route: disabled()}, want{no, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"off, enabled by route", true, at{route: enabled()}, want{yes, model.FilterEnabled, model.LevelRoute, filterConfigType}},
		{"off, enabled by vhost", true, at{vhost: enabled()}, want{yes, model.FilterEnabled, model.LevelVirtualHost, filterConfigType}},
		{"off, enabled by route config", true, at{routeConfig: enabled()}, want{yes, model.FilterEnabled, model.LevelRouteConfig, filterConfigType}},
		{"off, filter-specific at route", true, at{route: specific()}, want{yes, model.FilterEnabled, model.LevelRoute, extAuthzPerRoute}},
		{"disabled wins over config in one FilterConfig", true,
			at{route: map[string]any{"@type": filterConfigType, "disabled": true, "config": map[string]any{}}},
			want{no, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"invalid FilterConfig, HCM on", false, at{route: invalid()}, want{unknown, model.FilterUnknown, model.LevelRoute, filterConfigType}},
		{"invalid FilterConfig, HCM off", true, at{vhost: invalid()}, want{unknown, model.FilterUnknown, model.LevelVirtualHost, filterConfigType}},
		{"route beats vhost", true, at{route: disabled(), vhost: enabled()}, want{no, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"vhost beats route config", true, at{vhost: enabled(), routeConfig: disabled()}, want{yes, model.FilterEnabled, model.LevelVirtualHost, filterConfigType}},
		{"route beats route config", false, at{route: enabled(), routeConfig: disabled()}, want{yes, model.FilterOn, model.LevelRoute, filterConfigType}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := first(t, setup(tt.hcmDisabled, tt.at))
			if len(c.Filters) != 1 {
				t.Fatalf("got %d entries, want 1", len(c.Filters))
			}
			e := c.Filters[0]
			got := want{e.Runs, e.State, e.Level, e.PerFilterType}
			if got != tt.want {
				t.Errorf("Runs, State, Level, PerFilterType = %+v, want %+v", got, tt.want)
			}
			if e.Name != "f" || e.Type != extAuthzType || e.Request != yes || e.Response != yes {
				t.Errorf("Name, Type, Request, Response = %q, %q, %q, %q", e.Name, e.Type, e.Request, e.Response)
			}
			if e.Wrapper != "" || e.Conditional || e.ConfigSource != model.ConfigInline {
				t.Errorf("Wrapper, Conditional, ConfigSource = %q, %v, %q; want none, false, inline", e.Wrapper, e.Conditional, e.ConfigSource)
			}
			if c.Conditional {
				t.Error("Conditional = true, want false")
			}
		})
	}
}

// TestEffective_KeyedByRoute: one chain per route ID, each entry carrying its
// slot ID; an HCM without a joined route config has its default under the use ID.
func TestEffective_KeyedByRoute(t *testing.T) {
	filters := []model.HTTPFilter{{Name: "a", Type: extAuthzType}, {Name: "router", Type: routerType}}
	snap := hcmSnap(filters, &model.RouteConfig{VirtualHosts: []model.VirtualHost{
		{Routes: []model.Route{{Name: "r0"}, {Name: "r1"}}}, {Routes: []model.Route{{Name: "r2"}}}}})
	snap.Listeners = append(snap.Listeners, model.Listener{ID: "L1", FilterChains: []model.NetworkFilterChain{{ID: "L1/C0",
		HCM: &model.HCMConfig{ID: "L1/C0/N0", RouteConfigUse: model.RouteConfigUse{ID: "L1/C0/N0/RC", State: model.RouteConfigUnknown},
			HTTPFilters: []model.HTTPFilter{{ID: "L1/C0/N0/F0", Name: "router", Type: routerType}}}}}})

	got := chain.Effective(snap)
	l0 := []model.ID{"L0/C0/N0/F0", "L0/C0/N0/F1"}
	want := map[model.ID][]model.ID{
		"L0/C0/N0/RC/V0/R0": l0,
		"L0/C0/N0/RC/V0/R1": l0,
		"L0/C0/N0/RC/V1/R0": l0,
		"L1/C0/N0/RC":       {"L1/C0/N0/F0"},
	}
	slots := map[model.ID][]model.ID{}
	for k, c := range got {
		slots[k] = []model.ID{}
		for _, e := range c.Filters {
			slots[k] = append(slots[k], e.Slot)
		}
	}
	if !reflect.DeepEqual(slots, want) {
		t.Errorf("slots by key = %v\nwant %v", slots, want)
	}
}

// TestEffective_ReadsOnly: Effective leaves the snapshot as it was.
func TestEffective_ReadsOnly(t *testing.T) {
	snap := setup(true, at{route: enabled()})
	before := *snap.Listeners[0].FilterChains[0].HCM.RouteConfig
	chain.Effective(snap)
	if after := *snap.Listeners[0].FilterChains[0].HCM.RouteConfig; !reflect.DeepEqual(before, after) {
		t.Errorf("route config changed:\n%+v\n%+v", before, after)
	}
}

// TestEffective_NoRouteConfigLevel uses route and vhost entries only.
func TestEffective_NoRouteConfigLevel(t *testing.T) {
	e := first(t, setup(true, at{vhost: enabled()})).Filters[0]
	if e.Runs != model.TriYes || e.State != model.FilterEnabled || e.Level != model.LevelVirtualHost {
		t.Errorf("Runs, State, Level = %q, %q, %q; want yes, enabled, virtualHost", e.Runs, e.State, e.Level)
	}
}

// TestEffective_Conditional marks a chain whose route has a backend with per-filter config.
func TestEffective_Conditional(t *testing.T) {
	snap := setup(true, at{route: enabled()})
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	r.WeightedClusters = []model.WeightedCluster{
		{Name: "v1", Weight: 80, TypedPerFilterConfig: map[string]any{"f": disabled()}},
		{Name: "v2", Weight: 20},
	}
	c := first(t, snap)
	if !c.Conditional {
		t.Error("Conditional = false, want true")
	}
	// The baseline ignores the backend's entry.
	if e := c.Filters[0]; e.Runs != model.TriYes || e.State != model.FilterEnabled || e.Level != model.LevelRoute {
		t.Errorf("baseline Runs, State, Level = %q, %q, %q; want yes, enabled, route", e.Runs, e.State, e.Level)
	}

	r.WeightedClusters[0].TypedPerFilterConfig = nil
	if first(t, snap).Conditional {
		t.Error("Conditional = true without backend per-filter config, want false")
	}
}

// TestEffective_SidesAndOrders checks the side table and both orders on a
// chain with a decoder-only filter, an unknown type, and a filter that doesn't run.
func TestEffective_SidesAndOrders(t *testing.T) {
	c := first(t, oneRoute([]model.HTTPFilter{
		{Name: "rbac", Type: rbacType},
		{Name: "authz", Type: extAuthzType},
		{Name: "off", Type: extAuthzType, Disabled: true},
		{Name: "vendor", Type: "type.googleapis.com/example.Vendor"},
		{Name: "router", Type: routerType},
	}, nil, nil, model.Route{}))

	sides := map[string][2]model.Tri{}
	for _, e := range c.Filters {
		sides[e.Name] = [2]model.Tri{e.Request, e.Response}
	}
	yes, no, unknown := model.TriYes, model.TriNo, model.TriUnknown
	wantSides := map[string][2]model.Tri{
		"rbac":   {yes, no},
		"authz":  {yes, yes},
		"off":    {yes, yes},
		"vendor": {unknown, unknown},
		"router": {yes, no},
	}
	if !reflect.DeepEqual(sides, wantSides) {
		t.Errorf("sides = %v, want %v", sides, wantSides)
	}

	if got, want := names(c.RequestOrder()), []string{"rbac", "authz", "vendor", "router"}; !reflect.DeepEqual(got, want) {
		t.Errorf("request order = %v, want %v", got, want)
	}
	if got, want := names(c.ResponseOrder()), []string{"vendor", "authz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("response order = %v, want %v", got, want)
	}
}

package chain_test

import (
	"reflect"
	"testing"

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

func setup(hcmDisabled bool, a at) (model.HCMConfig, model.VirtualHost, model.Route) {
	pfc := func(v map[string]any) map[string]any {
		if v == nil {
			return nil
		}
		return map[string]any{"f": v}
	}
	r := model.Route{Name: "r", TypedPerFilterConfig: pfc(a.route)}
	vh := model.VirtualHost{Name: "vh", Routes: []model.Route{r}, TypedPerFilterConfig: pfc(a.vhost)}
	hcm := model.HCMConfig{
		HTTPFilters: []model.HTTPFilter{{Name: "f", Type: extAuthzType, Disabled: hcmDisabled}},
		RouteConfig: &model.RouteConfig{Name: "rc", VirtualHosts: []model.VirtualHost{vh}, TypedPerFilterConfig: pfc(a.routeConfig)},
	}
	return hcm, vh, r
}

// TestEffective_Semantics has one case per row of the design's semantics table,
// at each level, plus precedence between levels.
func TestEffective_Semantics(t *testing.T) {
	type want struct {
		runs  bool
		state model.FilterState
		level model.ConfigLevel
		typ   string
	}
	tests := []struct {
		name        string
		hcmDisabled bool
		at          at
		want        want
	}{
		{"on, no entry", false, at{}, want{true, model.FilterOn, "", ""}},
		{"on, disabled by route", false, at{route: disabled()}, want{false, model.FilterDisabled, model.LevelRoute, filterConfigType}},
		{"on, disabled by vhost", false, at{vhost: disabled()}, want{false, model.FilterDisabled, model.LevelVirtualHost, filterConfigType}},
		{"on, disabled by route config", false, at{routeConfig: disabled()}, want{false, model.FilterDisabled, model.LevelRouteConfig, filterConfigType}},
		{"on, config at route", false, at{route: enabled()}, want{true, model.FilterOn, model.LevelRoute, filterConfigType}},
		{"on, filter-specific at vhost", false, at{vhost: specific()}, want{true, model.FilterOn, model.LevelVirtualHost, extAuthzPerRoute}},
		{"off, no entry", true, at{}, want{false, model.FilterOff, "", ""}},
		{"off, disabled by route", true, at{route: disabled()}, want{false, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"off, enabled by route", true, at{route: enabled()}, want{true, model.FilterEnabled, model.LevelRoute, filterConfigType}},
		{"off, enabled by vhost", true, at{vhost: enabled()}, want{true, model.FilterEnabled, model.LevelVirtualHost, filterConfigType}},
		{"off, enabled by route config", true, at{routeConfig: enabled()}, want{true, model.FilterEnabled, model.LevelRouteConfig, filterConfigType}},
		{"off, filter-specific at route", true, at{route: specific()}, want{true, model.FilterEnabled, model.LevelRoute, extAuthzPerRoute}},
		{"disabled wins over config in one FilterConfig", true,
			at{route: map[string]any{"@type": filterConfigType, "disabled": true, "config": map[string]any{}}},
			want{false, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"invalid FilterConfig, HCM on", false, at{route: invalid()}, want{false, model.FilterUnknown, model.LevelRoute, filterConfigType}},
		{"invalid FilterConfig, HCM off", true, at{vhost: invalid()}, want{false, model.FilterUnknown, model.LevelVirtualHost, filterConfigType}},
		{"route beats vhost", true, at{route: disabled(), vhost: enabled()}, want{false, model.FilterOff, model.LevelRoute, filterConfigType}},
		{"vhost beats route config", true, at{vhost: enabled(), routeConfig: disabled()}, want{true, model.FilterEnabled, model.LevelVirtualHost, filterConfigType}},
		{"route beats route config", false, at{route: enabled(), routeConfig: disabled()}, want{true, model.FilterOn, model.LevelRoute, filterConfigType}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcm, vh, r := setup(tt.hcmDisabled, tt.at)
			c := chain.Effective(hcm, vh, r)
			if len(c.Filters) != 1 {
				t.Fatalf("got %d entries, want 1", len(c.Filters))
			}
			e := c.Filters[0]
			got := want{e.Runs, e.State, e.Level, e.PerFilterType}
			if got != tt.want {
				t.Errorf("Runs, State, Level, PerFilterType = %+v, want %+v", got, tt.want)
			}
			if e.Name != "f" || e.Type != extAuthzType || e.Request != model.SideYes || e.Response != model.SideYes {
				t.Errorf("Name, Type, Request, Response = %q, %q, %q, %q", e.Name, e.Type, e.Request, e.Response)
			}
			if c.Conditional {
				t.Error("Conditional = true, want false")
			}
		})
	}
}

// TestEffective_NoRouteConfig uses route and vhost entries only.
func TestEffective_NoRouteConfig(t *testing.T) {
	hcm, vh, r := setup(true, at{vhost: enabled()})
	hcm.RouteConfig = nil
	e := chain.Effective(hcm, vh, r).Filters[0]
	if !e.Runs || e.State != model.FilterEnabled || e.Level != model.LevelVirtualHost {
		t.Errorf("Runs, State, Level = %v, %q, %q; want true, enabled, virtualHost", e.Runs, e.State, e.Level)
	}
}

// TestEffective_Conditional marks a chain whose route has a backend with per-filter config.
func TestEffective_Conditional(t *testing.T) {
	hcm, vh, r := setup(true, at{route: enabled()})
	r.WeightedClusters = []model.WeightedCluster{
		{Name: "v1", Weight: 80, TypedPerFilterConfig: map[string]any{"f": disabled()}},
		{Name: "v2", Weight: 20},
	}
	c := chain.Effective(hcm, vh, r)
	if !c.Conditional {
		t.Error("Conditional = false, want true")
	}
	// The baseline ignores the backend's entry.
	if e := c.Filters[0]; !e.Runs || e.State != model.FilterEnabled || e.Level != model.LevelRoute {
		t.Errorf("baseline Runs, State, Level = %v, %q, %q; want true, enabled, route", e.Runs, e.State, e.Level)
	}

	r.WeightedClusters[0].TypedPerFilterConfig = nil
	if chain.Effective(hcm, vh, r).Conditional {
		t.Error("Conditional = true without backend per-filter config, want false")
	}
}

// TestEffective_SidesAndOrders checks the side table and both orders on a
// chain with a decoder-only filter, an unknown type, and a filter that doesn't run.
func TestEffective_SidesAndOrders(t *testing.T) {
	hcm := model.HCMConfig{HTTPFilters: []model.HTTPFilter{
		{Name: "rbac", Type: rbacType},
		{Name: "authz", Type: extAuthzType},
		{Name: "off", Type: extAuthzType, Disabled: true},
		{Name: "vendor", Type: "type.googleapis.com/example.Vendor"},
		{Name: "router", Type: routerType},
	}}
	c := chain.Effective(hcm, model.VirtualHost{}, model.Route{})

	sides := map[string][2]model.Side{}
	for _, e := range c.Filters {
		sides[e.Name] = [2]model.Side{e.Request, e.Response}
	}
	wantSides := map[string][2]model.Side{
		"rbac":   {model.SideYes, model.SideNo},
		"authz":  {model.SideYes, model.SideYes},
		"off":    {model.SideYes, model.SideYes},
		"vendor": {model.SideUnknown, model.SideUnknown},
		"router": {model.SideYes, model.SideNo},
	}
	if !reflect.DeepEqual(sides, wantSides) {
		t.Errorf("sides = %v, want %v", sides, wantSides)
	}

	var req, resp []string
	for _, e := range c.RequestOrder() {
		req = append(req, e.Name)
	}
	for _, e := range c.ResponseOrder() {
		resp = append(resp, e.Name)
	}
	if want := []string{"rbac", "authz", "vendor", "router"}; !reflect.DeepEqual(req, want) {
		t.Errorf("request order = %v, want %v", req, want)
	}
	if want := []string{"vendor", "authz"}; !reflect.DeepEqual(resp, want) {
		t.Errorf("response order = %v, want %v", resp, want)
	}
}

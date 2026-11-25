// Tests for the unexported resolveFilterConfig helper. This file uses
// package renderer (not renderer_test) to access the unexported function directly.
// TestMain is declared in renderer_test.go and applies to all test files in
// this directory, so the ANSI color profile is already forced there.
package renderer

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// oneRoute is a snapshot with one filter and one route, with per-filter config
// at route, vhost and route config level.
func oneRoute(f model.HTTPFilter, route, vhost, routeConfig map[string]any) *model.Snapshot {
	return snaptest.IDs(&model.Snapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		HTTPFilters: []model.HTTPFilter{f},
		RouteConfig: &model.RouteConfig{TypedPerFilterConfig: routeConfig, VirtualHosts: []model.VirtualHost{{
			TypedPerFilterConfig: vhost, Routes: []model.Route{{TypedPerFilterConfig: route}},
		}}},
	}}}}}})
}

// row is the row of oneRoute's i-th filter under its route.
func row(i int) render.Row {
	slot := model.ID("L0/C0/N0/F" + string(rune('0'+i)))
	return render.Row{Subject: slot, Route: "L0/C0/N0/RC/V0/R0", Slot: slot}
}

// TestResolveFilterConfig_Levels: the most specific level's config is shown.
func TestResolveFilterConfig_Levels(t *testing.T) {
	f := model.HTTPFilter{Name: "f", TypedConfig: map[string]any{"level": "hcm"}}
	at := func(level string) map[string]any { return map[string]any{"f": map[string]any{"level": level}} }
	tests := []struct {
		name                      string
		route, vhost, routeConfig map[string]any
		want                      string
	}{
		{"route beats vhost", at("route"), at("vhost"), at("rc"), "route"},
		{"vhost beats route config", nil, at("vhost"), at("rc"), "vhost"},
		{"route config beats HCM", nil, nil, at("rc"), "rc"},
		{"HCM when none", nil, nil, nil, "hcm"},
	}
	for _, tt := range tests {
		got := resolveFilterConfig(oneRoute(f, tt.route, tt.vhost, tt.routeConfig), row(0))
		if want := map[string]any{"level": tt.want}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, want)
		}
	}
}

// TestResolveFilterConfig covers the config-resolution priority rules for
// [resolveFilterConfig]:
//  1. Per-route config (TypedPerFilterConfig[filter.Name]) takes precedence
//     over HCM-level config (HTTPFilter.TypedConfig) when the value is non-nil.
//  2. Per-route config key present but value nil → falls through to HCM config.
//  3. Neither source set → returns nil.
//  4. HCM-level config set, no per-route config → returns HCM config.
//  5. Empty map per-route value → treated as "has config" (non-nil in Go) and returned as-is.
func TestResolveFilterConfig(t *testing.T) {
	hmcConfig := map[string]any{"@type": "type.googleapis.com/hcm.Config", "level": "hcm"}
	perRouteConfig := map[string]any{"@type": "type.googleapis.com/route.Config", "level": "route"}

	tests := []struct {
		name                 string
		filterTypedConfig    map[string]any
		typedPerFilterConfig map[string]any
		filterName           string
		wantNil              bool
		wantValue            any
	}{
		{
			name:                 "per-route config takes precedence over HCM config",
			filterName:           "example.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"example.transformation": perRouteConfig},
			wantValue:            perRouteConfig,
		},
		{
			name:                 "per-route key present but value nil falls through to HCM config",
			filterName:           "example.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"example.transformation": nil},
			wantValue:            hmcConfig,
		},
		{
			name:                 "neither per-route nor HCM config set returns nil",
			filterName:           "envoy.filters.http.router",
			filterTypedConfig:    nil,
			typedPerFilterConfig: nil,
			wantNil:              true,
		},
		{
			name:                 "HCM config set, no per-route config returns HCM config",
			filterName:           "example.transformation",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: nil,
			wantValue:            hmcConfig,
		},
		{
			name:              "empty map per-route value is treated as has-config and returned",
			filterName:        "envoy.filters.http.cors",
			filterTypedConfig: hmcConfig,
			typedPerFilterConfig: map[string]any{
				"envoy.filters.http.cors": map[string]any{}, // empty map — non-nil in Go
			},
			wantValue: map[string]any{},
		},
		{
			name:                 "per-route config for different filter name does not apply",
			filterName:           "envoy.filters.http.router",
			filterTypedConfig:    hmcConfig,
			typedPerFilterConfig: map[string]any{"example.transformation": perRouteConfig},
			wantValue:            hmcConfig, // falls through to HCM config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := model.HTTPFilter{
				Name:        tt.filterName,
				TypedConfig: tt.filterTypedConfig,
			}
			got := resolveFilterConfig(oneRoute(f, tt.typedPerFilterConfig, nil, nil), row(0))
			if tt.wantNil {
				if got != nil {
					t.Errorf("resolveFilterConfig() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("resolveFilterConfig() = nil, want %v", tt.wantValue)
				return
			}
			// Compare by pointer identity for the map cases — the function must
			// return exactly the map it received, not a copy.
			gotMap, gotOk := got.(map[string]any)
			wantMap, wantOk := tt.wantValue.(map[string]any)
			if gotOk && wantOk {
				// Verify same backing map via length and key presence (pointer equality
				// is not reliable across interface conversions in all Go versions).
				if len(gotMap) != len(wantMap) {
					t.Errorf("resolveFilterConfig() returned map with len %d, want %d", len(gotMap), len(wantMap))
				}
				for k, wv := range wantMap {
					if gv, ok := gotMap[k]; !ok || gv != wv {
						t.Errorf("resolveFilterConfig() map[%q] = %v, want %v", k, gv, wv)
					}
				}
			}
		})
	}
}

// TestRenderInteractive_DuplicateNamesOwnConfig: two HCM filters share a name.
// Expanded, each row shows its own config directly under it.
func TestRenderInteractive_DuplicateNamesOwnConfig(t *testing.T) {
	snap := oneRoute(model.HTTPFilter{Name: "dup", TypedConfig: map[string]any{"which": "first"}}, nil, nil, nil)
	hcm := snap.Listeners[0].FilterChains[0].HCM
	hcm.HTTPFilters = append(hcm.HTTPFilters, model.HTTPFilter{Name: "dup", TypedConfig: map[string]any{"which": "second"}})
	snaptest.IDs(snap)
	v := View{Snapshot: snap, Chains: chain.Effective(snap)}
	out := RenderInteractive(v, RenderOpts{Expanded: map[render.Row]bool{row(0): true, row(1): true}})

	lines := strings.Split(ansiCodes.ReplaceAllString(out, ""), "\n")
	var got []string
	for i, l := range lines {
		if strings.Contains(l, " dup ") && i+2 < len(lines) {
			got = append(got, strings.TrimSpace(lines[i+1])+strings.TrimSpace(lines[i+2]))
		}
	}
	want := []string{`{"which": "first"`, `{"which": "second"`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config under each dup row = %q, want %q\n%s", got, want, out)
	}
}

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

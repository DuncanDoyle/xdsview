package renderer

import (
	"encoding/json"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
	"github.com/charmbracelet/lipgloss"
)

// cursorStyle highlights the filter row that holds the cursor. Reverse emits
// \x1b[7m, which the TUI uses to find the cursor line.
var cursorStyle = lipgloss.NewStyle().Reverse(true)

// RenderOpts carries the cursor position and expanded filter rows for
// [RenderInteractive]. A nil Cursor highlights nothing.
type RenderOpts struct {
	Cursor   *render.Row
	Expanded map[render.Row]bool
}

// RenderInteractive renders every HCM filter of every route as a row, so each
// row the TUI can select is visible. The cursor row's filter name is
// reversed, and expanded rows show their deciding config as indented JSON.
func RenderInteractive(v View, opts RenderOpts) string {
	return text(v, render.Options{
		AllFilters: true,
		Style: func(c render.Class, r render.Row, s string) string {
			isFilter := c == render.ClassFilter || c == render.ClassFilterOff
			if isFilter && opts.Cursor != nil && r == *opts.Cursor {
				return cursorStyle.Render(s)
			}
			return style(c, r, s)
		},
		Notes: func(r render.Row) []string {
			if r.Slot == "" || !opts.Expanded[r] {
				return nil
			}
			cfg := resolveFilterConfig(v.Snapshot, r)
			if cfg == nil {
				return []string{"(no typed config)"}
			}
			out, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return []string{"(no typed config)"}
			}
			return strings.Split(string(out), "\n")
		},
	})
}

// resolveFilterConfig returns the config that decides r's filter on its
// route: the most specific per-filter config (route, vhost, route config),
// else the HCM filter's own config, else nil.
func resolveFilterConfig(snapshot *model.Snapshot, r render.Row) any {
	for _, l := range snapshot.Listeners {
		for _, fc := range l.FilterChains {
			hcm := fc.HCM
			if hcm == nil || !r.Slot.Within(hcm.ID) {
				continue
			}
			for _, f := range hcm.HTTPFilters {
				if f.ID != r.Slot {
					continue
				}
				for _, level := range perFilterLevels(hcm.RouteConfig, r.Route) {
					if v, ok := level[f.Name]; ok && v != nil {
						return v
					}
				}
				if f.TypedConfig != nil {
					return f.TypedConfig
				}
				return nil
			}
		}
	}
	return nil
}

// perFilterLevels returns the per-filter config of route's route, vhost and
// route config, most specific first, or nil when rc has no such route.
func perFilterLevels(rc *model.RouteConfig, route model.ID) []map[string]any {
	if rc == nil {
		return nil
	}
	for _, vh := range rc.VirtualHosts {
		for _, rt := range vh.Routes {
			if rt.ID == route {
				return []map[string]any{rt.TypedPerFilterConfig, vh.TypedPerFilterConfig, rc.TypedPerFilterConfig}
			}
		}
	}
	return nil
}

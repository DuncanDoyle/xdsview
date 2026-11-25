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

// RenderOpts carries the cursor position and expanded filters for
// [RenderInteractive]. A nil Cursor highlights nothing.
type RenderOpts struct {
	Cursor   *render.FilterRef
	Expanded map[render.FilterRef]bool
}

// RenderInteractive renders every HCM filter of every route as a row, so each
// row the TUI can select is visible. The cursor row is reversed, and expanded
// rows show their deciding config as indented JSON.
func RenderInteractive(snapshot *model.Snapshot, opts RenderOpts) string {
	return text(snapshot, render.Options{
		AllFilters: true,
		Style: func(c render.Class, ref *render.FilterRef, s string) string {
			if ref != nil && opts.Cursor != nil && *ref == *opts.Cursor {
				return cursorStyle.Render(s)
			}
			return style(c, ref, s)
		},
		Extra: func(ref render.FilterRef) []string {
			if !opts.Expanded[ref] {
				return nil
			}
			cfg := resolveFilterConfig(snapshot, ref)
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

// resolveFilterConfig returns the config that decides ref's filter on its
// route: the most specific per-filter config (route, vhost, route config),
// else the HCM filter's own config, else nil.
func resolveFilterConfig(snapshot *model.Snapshot, ref render.FilterRef) any {
	hcm := snapshot.Listeners[ref.ListenerIdx].FilterChains[ref.FilterChainIdx].HCM
	f := hcm.HTTPFilters[ref.FilterIdx]
	if rc := hcm.RouteConfig; rc != nil {
		vh := rc.VirtualHosts[ref.VirtualHostIdx]
		for _, pfc := range []map[string]any{vh.Routes[ref.RouteIdx].TypedPerFilterConfig, vh.TypedPerFilterConfig, rc.TypedPerFilterConfig} {
			if v, ok := pfc[f.Name]; ok && v != nil {
				return v
			}
		}
	}
	if f.TypedConfig != nil {
		return f.TypedConfig
	}
	return nil
}

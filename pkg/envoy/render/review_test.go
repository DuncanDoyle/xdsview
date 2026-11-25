package render_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// oneHCM is a listener with one route over the given HCM filters.
func oneHCM(filters []model.HTTPFilter, r model.Route) *model.Snapshot {
	return &model.Snapshot{Listeners: []model.Listener{{Name: "l", Address: ":::80",
		State: model.ListenerState{Status: model.ListenerActive},
		FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{HTTPFilters: filters,
			RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: "vh", Routes: []model.Route{r}}}}}}}}}}
}

// TestText_ConditionalAlwaysLabelled: a conditional chain is labelled as the
// baseline even when no filter runs on it, and in response order too.
func TestText_ConditionalAlwaysLabelled(t *testing.T) {
	r := model.Route{WeightedClusters: []model.WeightedCluster{
		{Name: "v1", Weight: 1, TypedPerFilterConfig: map[string]any{"authz": map[string]any{"@type": fcType, "config": map[string]any{}}}},
	}}
	snap := oneHCM([]model.HTTPFilter{{Name: "authz", Type: httpFilters + "ext_authz.v3.ExtAuthz", Disabled: true}}, r)
	out := text(t, snap, render.Options{ResponseOrder: true})
	hasLine(t, out, "      filters (request order, baseline before backend selection):")
	hasLine(t, out, "      filters (response order, baseline before backend selection):")
	hasLine(t, out, "      not run: authz (off)")
	hasLine(t, out, "      backend overrides: v1 (authz)")
}

// TestText_DuplicateNames: each row gets its own slot ID.
func TestText_DuplicateNames(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "dup"}, {Name: "dup"}}, model.Route{})
	var slots []model.ID
	opts := render.Options{AllFilters: true,
		Style: func(c render.Class, r render.Row, s string) string {
			if c == render.ClassFilter || c == render.ClassFilterOff {
				slots = append(slots, r.Slot)
			}
			return s
		},
		Notes: func(r render.Row) []string {
			if r.Slot == "" {
				return nil
			}
			return []string{"config " + string(r.Slot)}
		},
	}
	out := text(t, snap, opts)
	if want := []model.ID{"L0/C0/N0/F0", "L0/C0/N0/F1"}; !reflect.DeepEqual(slots, want) {
		t.Errorf("slots = %v, want %v", slots, want)
	}
	if !strings.Contains(out, "config L0/C0/N0/F0\n") || !strings.Contains(out, "config L0/C0/N0/F1\n") {
		t.Errorf("notes not per row:\n%s", out)
	}
}

// TestText_UntrustedStrings: control bytes from the dump are escaped and no
// line ends in whitespace.
func TestText_UntrustedStrings(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "router"}}, model.Route{
		RequestHeadersToAdd: []model.HeaderOperation{{Key: "x-a", Value: "v\t"}}})
	snap.Listeners = append(snap.Listeners, model.Listener{Name: "bad",
		State: model.ListenerState{Status: model.ListenerRejected, Error: &model.ListenerError{Details: "oops \x1b[2J"}}})
	out := text(t, snap, render.Options{})
	if strings.Contains(out, "\x1b") {
		t.Errorf("output contains an ESC byte:\n%q", out)
	}
	if !strings.Contains(out, `error: oops \x1b[2J`) {
		t.Errorf("escaped ESC missing:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimRight(l, " \t") != l {
			t.Errorf("trailing whitespace: %q", l)
		}
	}
}

// TestText_InnerSpacesKept: escaping keeps spaces inside a line; only the
// line's end is trimmed.
func TestText_InnerSpacesKept(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "router"}}, model.Route{
		Rewrite:             &model.RouteRewrite{RegexPattern: "^x ", Substitution: "/y"},
		RequestHeadersToAdd: []model.HeaderOperation{{Key: "a", Value: "1 "}, {Key: "b", Value: "2"}}})
	out := text(t, snap, render.Options{})
	hasLine(t, out, "      rewrite: ^x  → /y")
	hasLine(t, out, "      request headers: + a: 1 , + b: 2")
}

// TestText_UTF8Alignment: padding counts characters, not bytes.
func TestText_UTF8Alignment(t *testing.T) {
	out := text(t, oneHCM([]model.HTTPFilter{{Name: "é", Type: httpFilters + "cors.v3.Cors"}, {Name: "a", Type: httpFilters + "cors.v3.Cors"}}, model.Route{}), render.Options{})
	hasLine(t, out, "        1 é  on")
	hasLine(t, out, "        2 a  on")
}

// routeNotes records the route rows Notes is called for, and prints note for each.
func routeNotes(rows *[]render.Row, note func(render.Row) string) func(render.Row) []string {
	return func(r render.Row) []string {
		if r.Slot != "" {
			return nil
		}
		*rows = append(*rows, r)
		if n := note(r); n != "" {
			return []string{n}
		}
		return nil
	}
}

// TestText_RouteNotes: route-row notes print under each route line, before
// its filters, with that route's row (D-010).
func TestText_RouteNotes(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "router"}}, model.Route{Match: model.RouteMatch{Prefix: "/a"}})
	vh := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0]
	vh.Routes = append(vh.Routes, model.Route{Match: model.RouteMatch{Prefix: "/b"}})
	var rows []render.Row
	out := text(t, snap, render.Options{Notes: routeNotes(&rows, func(r render.Row) string {
		if r.Route == "L0/C0/N0/RC/V0/R1" {
			return "note: one candidate exists in the cluster"
		}
		return ""
	})})
	want := []render.Row{{Subject: "L0/C0/N0/RC/V0/R0", Route: "L0/C0/N0/RC/V0/R0"}, {Subject: "L0/C0/N0/RC/V0/R1", Route: "L0/C0/N0/RC/V0/R1"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}
	block := "   └─ route /b (prefix)\n" +
		"      note: one candidate exists in the cluster\n" +
		"      filters (request order):\n"
	if !strings.Contains(out, block) {
		t.Errorf("no note under the /b route line:\n%s", out)
	}
}

// TestText_RouteNoteRows: route rows follow the traversal, and each note
// lands under its own route.
func TestText_RouteNoteRows(t *testing.T) {
	vh := func(name string, routes ...string) model.VirtualHost {
		v := model.VirtualHost{Name: name}
		for _, p := range routes {
			v.Routes = append(v.Routes, model.Route{Match: model.RouteMatch{Prefix: p}})
		}
		return v
	}
	chainOf := func(name string, vhs ...model.VirtualHost) model.NetworkFilterChain {
		return model.NetworkFilterChain{Name: name, TLS: &model.TLSContext{SNIHosts: []string{name}},
			HCM: &model.HCMConfig{RouteConfig: &model.RouteConfig{VirtualHosts: vhs}}}
	}
	snap := &model.Snapshot{Listeners: []model.Listener{
		{Name: "l0", FilterChains: []model.NetworkFilterChain{chainOf("c0", vh("v0", "/a"))}},
		{Name: "l1", FilterChains: []model.NetworkFilterChain{chainOf("c0", vh("v0", "/b")), chainOf("c1", vh("v0", "/c"), vh("v1", "/d", "/e"))}},
	}}
	var rows []render.Row
	out := text(t, snap, render.Options{Notes: routeNotes(&rows, func(r render.Row) string { return "ref " + string(r.Route) })})
	var got []model.ID
	for _, r := range rows {
		got = append(got, r.Route)
	}
	want := []model.ID{"L0/C0/N0/RC/V0/R0", "L1/C0/N0/RC/V0/R0", "L1/C1/N0/RC/V0/R0", "L1/C1/N0/RC/V1/R0", "L1/C1/N0/RC/V1/R1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("routes = %v, want %v", got, want)
	}
	for _, pair := range [][2]string{{"/d", "ref L1/C1/N0/RC/V1/R0"}, {"/e", "ref L1/C1/N0/RC/V1/R1"}, {"/c", "ref L1/C1/N0/RC/V0/R0"}} {
		lines := strings.Split(out, "\n")
		i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "route "+pair[0]+" ") })
		if i < 0 || i+1 >= len(lines) || strings.TrimLeft(lines[i+1], " │") != pair[1] {
			t.Errorf("line under route %s isn't %q:\n%s", pair[0], pair[1], out)
		}
	}
}

// TestText_AddressesAndAlignment: IPv6 hosts are bracketed in the text, and
// the state column lines up across one- and two-digit and "?" rows.
func TestText_AddressesAndAlignment(t *testing.T) {
	var filters []model.HTTPFilter
	for i := range 10 {
		filters = append(filters, model.HTTPFilter{Name: fmt.Sprintf("f%d", i), Type: httpFilters + "cors.v3.Cors"})
	}
	filters[3].Type = typePrefix + "acme.Unknown"
	out := text(t, oneHCM(filters, model.Route{}), render.Options{})
	if !strings.HasPrefix(out, "l [::]:80 · active\n") {
		t.Errorf("listener line = %q", strings.SplitN(out, "\n", 2)[0])
	}
	col, rows := -1, 0
	for _, l := range strings.Split(out, "\n") {
		i := strings.Index(l, "  on")
		if !strings.Contains(l, " f") || i < 0 {
			continue
		}
		rows++
		if col >= 0 && i != col {
			t.Errorf("state column at %d, want %d: %q", i, col, l)
		}
		col = i
	}
	if rows != 10 {
		t.Fatalf("got %d filter rows, want 10:\n%s", rows, out)
	}
	hasLine(t, out, "        4? f3  on")
	hasLine(t, out, "        10 f9  on")

	for addr, want := range map[string]string{"0.0.0.0:80": "0.0.0.0:80", "[::]:443": "[::]:443", "": ""} {
		snap := oneHCM(nil, model.Route{})
		snap.Listeners[0].Address = addr
		if got := strings.SplitN(text(t, snap, render.Options{}), " · ", 2)[0]; got != strings.TrimSpace("l "+want) {
			t.Errorf("address %q printed as %q, want %q", addr, got, "l "+want)
		}
	}
}

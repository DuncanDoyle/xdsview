package render_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

// oneHCM is a listener with one route over the given HCM filters.
func oneHCM(filters []model.HTTPFilter, r model.Route) *model.EnvoySnapshot {
	return &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", Address: ":::80",
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

// TestText_DuplicateNames: each row gets its own HCM index.
func TestText_DuplicateNames(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "dup"}, {Name: "dup"}}, model.Route{})
	var idx []int
	opts := render.Options{AllFilters: true,
		Style: func(c render.Class, ref *render.FilterRef, s string) string {
			if ref != nil {
				idx = append(idx, ref.FilterIdx)
			}
			return s
		},
		Extra: func(ref render.FilterRef) []string { return []string{fmt.Sprintf("config %d", ref.FilterIdx)} },
	}
	out := text(t, snap, opts)
	if !reflect.DeepEqual(idx, []int{0, 1}) {
		t.Errorf("FilterIdx = %v, want [0 1]", idx)
	}
	if !strings.Contains(out, "config 0\n") || !strings.Contains(out, "config 1\n") {
		t.Errorf("Extra lines not per row:\n%s", out)
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

func TestText_KnownWording(t *testing.T) {
	snap := oneHCM([]model.HTTPFilter{{Name: "router"}}, model.Route{Origin: &model.RouteAnnotation{
		Certainty: model.Inferred, NarrowedByKnown: true,
		Candidates: []model.RouteOrigin{{Route: model.Ref{Kind: "HTTPRoute", Namespace: "a", Name: "b"}, RuleIndex: 0, MatchIndex: 0}}}})
	hasLine(t, text(t, snap, render.Options{}), "   └─ route / · HTTPRoute a/b · rule 0 · match 0 (chosen from known HTTPRoutes)")
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

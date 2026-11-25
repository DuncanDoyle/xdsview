package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

const (
	typePrefix  = "type.googleapis.com/"
	httpFilters = typePrefix + "envoy.extensions.filters.http."
	fcType      = typePrefix + "envoy.config.route.v3.FilterConfig"
	routeName   = "listener~80~orders_example-route-0-httproute-orders-shop-0-0-matcher-0"
)

// sample is one Gateway with an active listener whose route enables ext_auth
// per route, and a warming listener with an error. [sampleLabels] and
// [sampleTrailer] stand in for krp's provenance hooks.
func sample() *model.Snapshot {
	hcm := &model.HCMConfig{
		RouteConfigName: "listener~80",
		HTTPFilters: []model.HTTPFilter{
			{Name: "example.transformation", Type: httpFilters + "dynamic_modules.v3.DynamicModuleFilter"},
			{Name: "ext_auth/shop/auth", Type: httpFilters + "ext_authz.v3.ExtAuthz", Disabled: true},
			{Name: "ratelimit/shop/rl", Type: httpFilters + "ratelimit.v3.RateLimit", Disabled: true},
			{Name: "envoy.filters.http.cors", Type: httpFilters + "cors.v3.Cors", Disabled: true},
			{Name: "envoy.filters.http.router", Type: httpFilters + "router.v3.Router"},
		},
		UpstreamFilters: []model.HTTPFilter{{Name: "envoy.filters.http.upstream_codec"}},
		RouteConfig: &model.RouteConfig{Name: "listener~80", VirtualHosts: []model.VirtualHost{{
			Name: "listener~80~orders_example", Domains: []string{"orders.example"},
			Routes: []model.Route{{
				Name:                 routeName,
				Match:                model.RouteMatch{Prefix: "/orders"},
				Cluster:              "kube_shop_orders_8080",
				RequestHeadersToAdd:  []model.HeaderOperation{{Key: "x-a", Value: "1"}},
				TypedPerFilterConfig: map[string]any{"ext_auth/shop/auth": map[string]any{"@type": fcType, "config": map[string]any{}}},
				Metadata: map[string]any{"merge.TrafficPolicy.gateway.kgateway.dev": map[string]any{
					"extAuth": []any{"gateway.kgateway.dev/TrafficPolicy/shop/auth"}}},
			}},
		}}},
	}
	snap := &model.Snapshot{Listeners: []model.Listener{
		{Name: "listener~80", Address: "[::]:80", State: model.ListenerState{Status: model.ListenerActive},
			FilterChains: []model.NetworkFilterChain{{Name: "listener~80", HCM: hcm}}},
		{Name: "listener~8443", Address: "[::]:8443", State: model.ListenerState{Status: model.ListenerWarming,
			Error: &model.ListenerError{Details: "bad cert", LastAttempt: "2026-11-20T19:39:58Z"}}},
	}}
	return snaptest.IDs(snap)
}

// sampleLabels are labels like krp's on sample's listeners, vhost, route,
// and ext_auth and ratelimit rows.
func sampleLabels(r render.Row) []string {
	switch {
	case r.Slot == "L0/C0/N0/F1":
		return []string{"GatewayExtension shop/auth (inferred)", "TrafficPolicy shop/auth (extAuth, inferred)"}
	case r.Slot == "L0/C0/N0/F2":
		return []string{"GatewayExtension shop/rl (inferred)"}
	case r.Slot != "":
		return nil
	}
	return map[model.ID][]string{
		"L0":                {"Gateway shop/gw port 80 (inferred)"},
		"L1":                {"Gateway shop/gw port 8443 (inferred)"},
		"L0/C0/N0/RC/V0":    {"port 80 (inferred)"},
		"L0/C0/N0/RC/V0/R0": {"HTTPRoute shop/orders · rule 0 · match 0 (inferred)"},
	}[r.Subject]
}

// sampleTrailer is a policies line like krp's under sample's route.
func sampleTrailer(r render.Row) []render.Line {
	if r.Route == "" || r.Slot != "" {
		return nil
	}
	return []render.Line{{Label: "policies", Text: "TrafficPolicy shop/auth (extAuth)"}}
}

// text renders snap with the chains chain.Effective gives it. Without IDs, it adds them.
func text(t *testing.T, snap *model.Snapshot, opts render.Options) string {
	t.Helper()
	snaptest.IDs(snap)
	var b bytes.Buffer
	if err := render.Text(&b, snap, chain.Effective(snap), opts); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestText_Default is the design's text shape, literally.
func TestText_Default(t *testing.T) {
	want := `Gateway shop/gw · file dump.json
listener~80 [::]:80 · active · Gateway shop/gw port 80 (inferred)
└─ vhost listener~80~orders_example [orders.example] · port 80 (inferred)
   └─ route /orders (prefix) · HTTPRoute shop/orders · rule 0 · match 0 (inferred)
      filters (request order):
        1 example.transformation     on
        2 ext_auth/shop/auth         enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)
        3 envoy.filters.http.router  on
      not run: ratelimit/shop/rl (off), envoy.filters.http.cors (off)
      router upstream filters (as configured): envoy.filters.http.upstream_codec
      policies: TrafficPolicy shop/auth (extAuth)
      request headers: + x-a: 1
      backend: kube_shop_orders_8080
listener~8443 [::]:8443 · warming · Gateway shop/gw port 8443 (inferred) · error: bad cert (2026-11-20T19:39:58Z)
`
	if got := text(t, sample(), render.Options{Header: "Gateway shop/gw · file dump.json", Labels: sampleLabels, Trailer: sampleTrailer}); got != want {
		t.Errorf("Text =\n%s\nwant\n%s", got, want)
	}
}

func lines(s string) []string { return strings.Split(strings.TrimSuffix(s, "\n"), "\n") }

func hasLine(t *testing.T, out, line string) {
	t.Helper()
	for _, l := range lines(out) {
		if l == line {
			return
		}
	}
	t.Errorf("no line %q in\n%s", line, out)
}

func TestText_AllFilters(t *testing.T) {
	out := text(t, sample(), render.Options{AllFilters: true, Labels: sampleLabels})
	for _, l := range []string{
		"        1 example.transformation     on",
		"        2 ext_auth/shop/auth         enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)",
		"        3 ratelimit/shop/rl          off · GatewayExtension shop/rl (inferred)",
		"        4 envoy.filters.http.cors    off",
		"        5 envoy.filters.http.router  on",
	} {
		hasLine(t, out, l)
	}
	if strings.Contains(out, "not run:") {
		t.Error("AllFilters output has a not run: line")
	}
}

func TestText_ResponseOrder(t *testing.T) {
	out := text(t, sample(), render.Options{ResponseOrder: true, Labels: sampleLabels})
	want := "      filters (response order):\n" +
		"        1 ext_auth/shop/auth      enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)\n" +
		"        2 example.transformation  on\n"
	if !strings.Contains(out, want) {
		t.Errorf("no response order block\n%s\nin\n%s", want, out)
	}
	if strings.Contains(text(t, sample(), render.Options{}), "response order") {
		t.Error("default output has a response order block")
	}
}

// TestText_UnknownSide marks a filter type outside the side table in both orders.
func TestText_UnknownSide(t *testing.T) {
	snap := sample()
	hcm := snap.Listeners[0].FilterChains[0].HCM
	hcm.HTTPFilters = append([]model.HTTPFilter{{ID: "waf", Name: "acme_waf", Type: typePrefix + "acme.Waf"}}, hcm.HTTPFilters...)
	out := text(t, snap, render.Options{ResponseOrder: true})
	hasLine(t, out, "        1? acme_waf                   on")
	hasLine(t, out, "        3? acme_waf                on")
}

func TestText_Conditional(t *testing.T) {
	snap := sample()
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	r.Cluster = ""
	r.WeightedClusters = []model.WeightedCluster{
		{Name: "v1", Weight: 80, TypedPerFilterConfig: map[string]any{"ext_auth/shop/auth": map[string]any{"@type": fcType, "disabled": true}}},
		{Name: "v2", Weight: 20},
	}
	out := text(t, snap, render.Options{})
	hasLine(t, out, "      filters (request order, baseline before backend selection):")
	hasLine(t, out, "      backend overrides: v1 (ext_auth/shop/auth)")
	hasLine(t, out, "      backends: v1 (80), v2 (20)")
}

func TestText_ListenerStates(t *testing.T) {
	errState := &model.ListenerError{Details: "dup", LastAttempt: "t1"}
	snap := &model.Snapshot{Listeners: []model.Listener{
		{Name: "a", State: model.ListenerState{Status: model.ListenerActive, Updating: true, Draining: true, Error: errState}},
		{Name: "b", State: model.ListenerState{Status: model.ListenerDraining}},
		{Name: "c", State: model.ListenerState{Status: model.ListenerRejected, Error: &model.ListenerError{Details: "bad"}}},
		{State: model.ListenerState{Status: model.ListenerRejected, Error: &model.ListenerError{Details: "lds", LastAttempt: "t2"}}},
	}}
	want := "a · active · updating · draining · error: dup (t1)\n" +
		"b · draining\n" +
		"c · rejected · error: bad\n" +
		"LDS error · rejected · error: lds (t2)\n"
	if got := text(t, snap, render.Options{}); got != want {
		t.Errorf("Text =\n%s\nwant\n%s", got, want)
	}
}

// TestText_Unannotated prints no origins or labels without hooks, and no
// filters for a route without a chain.
func TestText_Unannotated(t *testing.T) {
	snap := sample()
	out := text(t, snap, render.Options{})
	hasLine(t, out, "   └─ route /orders (prefix)")
	hasLine(t, out, "        2 ext_auth/shop/auth         enabled by route")

	var b bytes.Buffer
	if err := render.Text(&b, snap, nil, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "filters (request") || strings.Contains(b.String(), "not run:") {
		t.Errorf("no chains, but filters printed:\n%s", b.String())
	}
}

// TestText_UnknownRunState: an entry whose run state is unknown stays in
// position with a "?" and its reason, and is not listed as not run.
func TestText_UnknownRunState(t *testing.T) {
	snap := sample()
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	r.TypedPerFilterConfig = map[string]any{"ratelimit/shop/rl": map[string]any{"@type": fcType}}
	out := text(t, snap, render.Options{})
	hasLine(t, out, "        2? ratelimit/shop/rl          unknown: invalid FilterConfig at route")
	hasLine(t, out, "      not run: ext_auth/shop/auth (off), envoy.filters.http.cors (off)")
}

// TestText_SubjectWarnings (Codex #7): each subject's warnings print under
// its row as ClassWarning; a dump-wide warning doesn't print. A filter
// chain heading passes the chain's ID to Style.
func TestText_SubjectWarnings(t *testing.T) {
	snap := sample()
	snap.Listeners = append(snap.Listeners, model.Listener{Name: "two", FilterChains: []model.NetworkFilterChain{
		{Name: "a", HCM: &model.HCMConfig{RouteConfigName: "x", RouteConfigUse: model.RouteConfigUse{State: model.RouteConfigMissing}}},
		{Name: "b"},
	}})
	snaptest.IDs(snap)
	snap.Warnings = []model.Warning{
		{Code: model.WarnSectionSkipped, Message: "dump-wide"},
		{Subject: "L0", Code: "c", Message: "on listener"},
		{Subject: "L0/C0/N0/RC/V0", Code: "c", Message: "on vhost"},
		{Subject: "L0/C0/N0/RC/V0/R0", Code: "c", Message: "on route"},
		{Subject: "L0/C0/N0/F1", Code: model.WarnECDSAmbiguous, Message: "on ext_auth"},
		{Subject: "L0/C0/N0/F2", Code: model.WarnECDSAmbiguous, Message: "on ratelimit"},
		{Subject: "L0/C0/N0/U0", Code: model.WarnECDSAmbiguous, Message: "on upstream"},
		{Subject: "L2/C0/N0/RC", Code: model.WarnRDSMissing, Message: "on use"},
		{Subject: "L2/C1", Code: model.WarnHCMSkipped, Message: "on chain"},
	}
	var chains []render.Row
	opts := render.Options{Style: func(c render.Class, r render.Row, s string) string {
		if c == render.ClassFilterChain {
			chains = append(chains, r)
		}
		if c == render.ClassWarning {
			return "!" + s
		}
		return s
	}}
	want := `listener~80 [::]:80 · active
   !warning: on listener
└─ vhost listener~80~orders_example [orders.example]
   !warning: on vhost
   └─ route /orders (prefix)
      !warning: on route
      filters (request order):
        1 example.transformation     on
        2 ext_auth/shop/auth         enabled by route
            !warning: on ext_auth
        3 envoy.filters.http.router  on
      not run: ratelimit/shop/rl (off), envoy.filters.http.cors (off)
      !warning: on ratelimit
      router upstream filters (as configured): envoy.filters.http.upstream_codec
      !warning: on upstream
      request headers: + x-a: 1
      backend: kube_shop_orders_8080
listener~8443 [::]:8443 · !warming · !error: bad cert (2026-11-20T19:39:58Z)
two
├─ chain a
│  └─ !route config x missing: no route config has this name in a complete dump
│     !warning: on use
└─ chain b
   └─ !no HTTP connection manager
      !warning: on chain
`
	if got := text(t, snap, opts); got != want {
		t.Errorf("Text =\n%s\nwant\n%s", got, want)
	}
	if len(chains) != 2 || chains[0].Subject != "L2/C0" || chains[1].Subject != "L2/C1" {
		t.Errorf("chain rows = %+v, want L2/C0 and L2/C1", chains)
	}
}

func TestText_NoListeners(t *testing.T) {
	if got := text(t, &model.Snapshot{}, render.Options{}); got != "no listeners\n" {
		t.Errorf("Text = %q", got)
	}
}

// TestText_Hooks checks that Style sees each filter row, Labels print on the
// row and Notes add lines under it.
func TestText_Hooks(t *testing.T) {
	const route = "L0/C0/N0/RC/V0/R0"
	var rows []render.Row
	opts := render.Options{
		AllFilters: true,
		Labels:     sampleLabels,
		Style: func(c render.Class, r render.Row, s string) string {
			if c == render.ClassFilter || c == render.ClassFilterOff {
				rows = append(rows, r)
				return "<" + s + ">"
			}
			return s
		},
		Notes: func(r render.Row) []string {
			if r.Slot == "L0/C0/N0/F1" {
				return []string{"{", `  "config": {}`, "}"}
			}
			return nil
		},
	}
	out := text(t, sample(), opts)
	if want := (render.Row{Subject: "L0/C0/N0/F4", Route: route, Slot: "L0/C0/N0/F4"}); len(rows) != 5 || rows[4] != want {
		t.Errorf("rows = %+v, want 5, the last %+v", rows, want)
	}
	want := "        2 <ext_auth/shop/auth>         enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)\n" +
		"            {\n" +
		"              \"config\": {}\n" +
		"            }\n"
	if !strings.Contains(out, want) {
		t.Errorf("no styled row with labels and notes\n%s\nin\n%s", want, out)
	}
}

// TestText_RowsPerRoute: one slot under two routes with different per-filter
// config is two rows, and each gets its own labels and notes.
func TestText_RowsPerRoute(t *testing.T) {
	enable := map[string]any{"@type": fcType, "config": map[string]any{"a": 1}}
	snap := oneHCM([]model.HTTPFilter{{Name: "authz", Type: httpFilters + "ext_authz.v3.ExtAuthz", Disabled: true}},
		model.Route{Match: model.RouteMatch{Prefix: "/a"}, TypedPerFilterConfig: map[string]any{"authz": enable}})
	vh := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0]
	vh.Routes = append(vh.Routes, model.Route{Match: model.RouteMatch{Prefix: "/b"},
		TypedPerFilterConfig: map[string]any{"authz": map[string]any{"@type": fcType, "config": map[string]any{"b": 2}}}})
	out := text(t, snap, render.Options{
		Labels: func(r render.Row) []string { return []string{"label " + string(r.Route)} },
		Notes: func(r render.Row) []string {
			if r.Slot == "" {
				return nil
			}
			return []string{"note " + string(r.Route) + " " + string(r.Slot)}
		},
	})
	for _, r := range []string{"L0/C0/N0/RC/V0/R0", "L0/C0/N0/RC/V0/R1"} {
		row := "  1 authz  enabled by route · label " + r + "\n"
		if !strings.Contains(out, row) {
			t.Errorf("no row %q in\n%s", row, out)
		}
		if !strings.Contains(out, "      note "+r+" L0/C0/N0/F0\n") {
			t.Errorf("no note for %s in\n%s", r, out)
		}
	}
}

// TestText_Trailer: trailer lines go after the filters and the upstream
// filters line, before the actions and backend (review 9.1-9.2 #4). Styling
// sees the route row.
func TestText_Trailer(t *testing.T) {
	snap := sample()
	route := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	var styled []render.Row
	out := text(t, snap, render.Options{
		Trailer: func(r render.Row) []render.Line {
			if r.Route != route.ID {
				return nil
			}
			return []render.Line{{Label: "policies", Text: "TrafficPolicy shop/auth (extAuth)"}}
		},
		Style: func(c render.Class, r render.Row, s string) string {
			if c == render.ClassOrigin && strings.HasPrefix(s, "TrafficPolicy") {
				styled = append(styled, r)
			}
			return s
		},
	})
	want := "      router upstream filters (as configured): envoy.filters.http.upstream_codec\n" +
		"      policies: TrafficPolicy shop/auth (extAuth)\n" +
		"      request headers: + x-a: 1\n"
	if !strings.Contains(out, want) {
		t.Errorf("trailer not between filters and actions:\n%s", out)
	}
	if len(styled) != 1 || styled[0].Route != route.ID {
		t.Errorf("styled rows = %+v", styled)
	}
}

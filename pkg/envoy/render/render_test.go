package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

const (
	typePrefix  = "type.googleapis.com/"
	httpFilters = typePrefix + "envoy.extensions.filters.http."
	fcType      = typePrefix + "envoy.config.route.v3.FilterConfig"
	routeName   = "listener~80~orders_example-route-0-httproute-orders-shop-0-0-matcher-0"
)

// sample is one Gateway with an active listener whose route enables ext_auth
// per route, and a warming listener with an error. It is annotated the way
// krp does it: origins and chains from the kgateway mapper.
func sample() *model.EnvoySnapshot {
	hcm := &model.HCMConfig{
		RouteConfigName: "listener~80",
		HTTPFilters: []model.HTTPFilter{
			{Name: "io.solo.transformation", Type: typePrefix + "envoy.api.v2.filter.http.FilterTransformations"},
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
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{
		{Name: "listener~80", Address: "[::]:80", State: model.ListenerState{Status: model.ListenerActive},
			FilterChains: []model.NetworkFilterChain{{Name: "listener~80", HCM: hcm}}},
		{Name: "listener~8443", Address: "[::]:8443", State: model.ListenerState{Status: model.ListenerWarming,
			Error: &model.ListenerError{Details: "bad cert", LastAttempt: "2026-11-20T19:39:58Z"}}},
	}}
	gw := &model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "shop", Name: "gw"}
	m := origin.Kgateway(origin.KgatewayOptions{Gateway: gw})
	return chain.Annotate(origin.Annotate(snap, m), m)
}

func text(t *testing.T, snap *model.EnvoySnapshot, opts render.Options) string {
	t.Helper()
	var b bytes.Buffer
	if err := render.Text(&b, snap, opts); err != nil {
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
        1 io.solo.transformation     on
        2 ext_auth/shop/auth         enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)
        3 envoy.filters.http.router  on
      not run: ratelimit/shop/rl (off), envoy.filters.http.cors (off)
      router upstream filters (as configured): envoy.filters.http.upstream_codec
      policies: TrafficPolicy shop/auth (extAuth)
      request headers: + x-a: 1
      backend: kube_shop_orders_8080
listener~8443 [::]:8443 · warming · Gateway shop/gw port 8443 (inferred) · error: bad cert (2026-11-20T19:39:58Z)
`
	if got := text(t, sample(), render.Options{Header: "Gateway shop/gw · file dump.json"}); got != want {
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
	out := text(t, sample(), render.Options{AllFilters: true})
	for _, l := range []string{
		"        1 io.solo.transformation     on",
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
	out := text(t, sample(), render.Options{ResponseOrder: true})
	want := "      filters (response order):\n" +
		"        1 ext_auth/shop/auth      enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)\n" +
		"        2 io.solo.transformation  on\n"
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
	hcm.HTTPFilters = append([]model.HTTPFilter{{Name: "acme_waf", Type: typePrefix + "acme.Waf"}}, hcm.HTTPFilters...)
	snap = chain.Annotate(snap, nil)
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
	r.Chain = nil
	out := text(t, snap, render.Options{})
	hasLine(t, out, "      filters (request order, baseline before backend selection):")
	hasLine(t, out, "      backend overrides: v1 (ext_auth/shop/auth)")
	hasLine(t, out, "      backends: v1 (80), v2 (20)")
}

func TestText_ListenerStates(t *testing.T) {
	errState := &model.ListenerError{Details: "dup", LastAttempt: "t1"}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{
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

func TestText_RouteOrigins(t *testing.T) {
	snap := sample()
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	r.Origin.NarrowedByKnown = true
	hasLine(t, text(t, snap, render.Options{}), "   └─ route /orders (prefix) · HTTPRoute shop/orders · rule 0 · match 0 (chosen from known HTTPRoutes)")

	r.Origin = &model.RouteAnnotation{Certainty: model.Ambiguous, Candidates: []model.RouteOrigin{
		{Route: model.Ref{Kind: "HTTPRoute", Namespace: "a", Name: "b"}, RuleIndex: 0, MatchIndex: 0},
		{Route: model.Ref{Kind: "HTTPRoute", Namespace: "c", Name: "d"}, RuleIndex: 1, MatchIndex: 0},
	}}
	hasLine(t, text(t, snap, render.Options{}), "   └─ route /orders (prefix) · ambiguous: HTTPRoute a/b, HTTPRoute c/d")
}

// TestText_Certainty: exact origins print bare, inferred ones say so, and
// route indices parsed from the name say where they came from.
func TestText_Certainty(t *testing.T) {
	snap := sample()
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	web := model.Ref{Kind: "HTTPRoute", Namespace: "shop", Name: "web"}
	cases := []struct {
		label string
		o     model.RouteOrigin
		want  string
	}{
		{"metadata indices", model.RouteOrigin{Route: web, RuleIndex: 1, MatchIndex: 0},
			"   └─ route /orders (prefix) · HTTPRoute shop/web · rule 1 · match 0"},
		{"indices from the name", model.RouteOrigin{Route: web, RuleIndex: 1, RuleName: "api", MatchIndex: 0, IndexCertainty: model.Inferred},
			"   └─ route /orders (prefix) · HTTPRoute shop/web · rule 1 (api) · match 0 (from name)"},
	}
	for _, c := range cases {
		r.Origin = &model.RouteAnnotation{Certainty: model.Exact, Candidates: []model.RouteOrigin{c.o}}
		hasLine(t, text(t, snap, render.Options{}), c.want)
	}

	r.Policies = []model.PolicyRef{{Policy: model.Ref{Kind: "AcmePolicy", Namespace: "shop", Name: "p"}, Field: "x", Certainty: model.Inferred}}
	hasLine(t, text(t, snap, render.Options{}), "      policies: AcmePolicy shop/p (x, inferred)")
	for _, c := range []model.Certainty{model.Unknown, model.Ambiguous} {
		r.Policies[0].Certainty = c
		hasLine(t, text(t, snap, render.Options{}), "      policies: AcmePolicy shop/p (x, "+c.String()+")")
	}
}

// TestText_Unannotated computes chains when the snapshot has none, and prints no origins.
func TestText_Unannotated(t *testing.T) {
	snap := sample()
	r := &snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	r.Chain, r.Origin, r.Policies = nil, nil, nil
	out := text(t, snap, render.Options{})
	hasLine(t, out, "   └─ route /orders (prefix)")
	hasLine(t, out, "        2 ext_auth/shop/auth         enabled by route")
}

func TestText_NoListeners(t *testing.T) {
	if got := text(t, &model.EnvoySnapshot{}, render.Options{}); got != "no listeners\n" {
		t.Errorf("Text = %q", got)
	}
}

// TestText_Hooks checks that Style sees each filter row's ref and Extra adds lines under it.
func TestText_Hooks(t *testing.T) {
	var refs []render.FilterRef
	opts := render.Options{
		AllFilters: true,
		Style: func(c render.Class, ref *render.FilterRef, s string) string {
			if ref != nil && (c == render.ClassFilter || c == render.ClassFilterOff) {
				refs = append(refs, *ref)
				return "<" + s + ">"
			}
			return s
		},
		Extra: func(ref render.FilterRef) []string {
			if ref.FilterIdx == 1 {
				return []string{"{", `  "config": {}`, "}"}
			}
			return nil
		},
	}
	out := text(t, sample(), opts)
	if len(refs) != 5 || refs[4] != (render.FilterRef{FilterIdx: 4}) {
		t.Errorf("refs = %+v, want 5, the last at FilterIdx 4", refs)
	}
	want := "        2 <ext_auth/shop/auth>         enabled by route · GatewayExtension shop/auth (inferred) · TrafficPolicy shop/auth (extAuth, inferred)\n" +
		"            {\n" +
		"              \"config\": {}\n" +
		"            }\n"
	if !strings.Contains(out, want) {
		t.Errorf("no styled row with extra lines\n%s\nin\n%s", want, out)
	}
}

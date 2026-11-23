package filter_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/filter"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// collidingRoute reads as HTTPRoute orders in team-a and as orders-team in a.
const collidingRoute = "listener~80~shop-route-0-httproute-orders-team-a-0-0-matcher-0"

func routeRef(ns, name string) model.Ref {
	return model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: ns, Name: name}
}

// listener returns a listener with one HCM holding the given routes.
func listener(name string, state model.ListenerState, routes ...string) model.Listener {
	vh := model.VirtualHost{Name: "listener~80~shop"}
	for _, r := range routes {
		vh.Routes = append(vh.Routes, model.Route{Name: r})
	}
	l := model.Listener{Name: name, State: state}
	if state.Status != model.ListenerRejected {
		l.FilterChains = []model.NetworkFilterChain{{Name: name, HCM: &model.HCMConfig{
			RouteConfig: &model.RouteConfig{Name: name, VirtualHosts: []model.VirtualHost{vh}},
		}}}
	}
	return l
}

func opts(ns, name string) filter.Options {
	return filter.Options{HTTPRouteName: name, HTTPRouteNamespace: ns, RuleIndex: filter.AllRules}
}

// TestFilter_AnnotatedOrigins: annotated routes are matched on their origin,
// and an Ambiguous route is never selected but reported.
func TestFilter_AnnotatedOrigins(t *testing.T) {
	raw := &model.EnvoySnapshot{Listeners: []model.Listener{
		listener("listener~80", model.ListenerState{Status: model.ListenerActive}, collidingRoute),
	}}
	unknown := origin.Annotate(raw, origin.Kgateway(origin.KgatewayOptions{}))
	known := origin.Annotate(raw, origin.Kgateway(origin.KgatewayOptions{Known: []model.Ref{routeRef("team-a", "orders")}}))

	tests := []struct {
		name          string
		snap          *model.EnvoySnapshot
		opts          filter.Options
		wantRoutes    []string
		wantAmbiguous []string
	}{
		{"not annotated: name match selects orders/team-a", raw, opts("team-a", "orders"), []string{collidingRoute}, nil},
		{"not annotated: name match selects orders-team/a", raw, opts("a", "orders-team"), []string{collidingRoute}, nil},
		{"ambiguous: orders/team-a not selected", unknown, opts("team-a", "orders"), nil, []string{collidingRoute}},
		{"ambiguous: orders-team/a not selected", unknown, opts("a", "orders-team"), nil, []string{collidingRoute}},
		{"ambiguous: unrelated HTTPRoute not reported", unknown, opts("x", "y"), nil, nil},
		{"known orders/team-a: selected", known, opts("team-a", "orders"), []string{collidingRoute}, nil},
		// Known set the orders-team/a reading aside, but Envoy may still serve a deleted orders-team/a (#29).
		{"known orders/team-a: orders-team/a not selected, listed", known, opts("a", "orders-team"), nil, []string{collidingRoute}},
		{"known: rule 0 selected", known, filter.Options{HTTPRouteName: "orders", HTTPRouteNamespace: "team-a"}, []string{collidingRoute}, nil},
		{"known: rule 1 not selected", known, filter.Options{HTTPRouteName: "orders", HTTPRouteNamespace: "team-a", RuleIndex: 1}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeNames(filter.Filter(tt.snap, tt.opts)); !reflect.DeepEqual(got, tt.wantRoutes) {
				t.Errorf("Filter routes = %v, want %v", got, tt.wantRoutes)
			}
			if got := filter.Ambiguous(tt.snap, tt.opts); !reflect.DeepEqual(got, tt.wantAmbiguous) {
				t.Errorf("Ambiguous = %v, want %v", got, tt.wantAmbiguous)
			}
		})
	}

	if a := known.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0].Origin; a == nil || !a.NarrowedByKnown {
		t.Errorf("known origin = %+v, want NarrowedByKnown", a)
	}
}

// TestFilter_ListenersThatExplain: listeners that aren't active, are mid-update,
// or have an error, survive narrowing with their state even without a matching route.
func TestFilter_ListenersThatExplain(t *testing.T) {
	other := "listener~80~shop-route-0-httproute-other-ns-0-0-matcher-0"
	errState := &model.ListenerError{Details: "bad"}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{
		listener("active", model.ListenerState{Status: model.ListenerActive}, other),
		listener("active-error", model.ListenerState{Status: model.ListenerActive, Error: errState}, other),
		listener("active-updating", model.ListenerState{Status: model.ListenerActive, Updating: true}, other),
		listener("active-draining", model.ListenerState{Status: model.ListenerActive, Draining: true}, other),
		listener("warming", model.ListenerState{Status: model.ListenerWarming}, other),
		listener("draining", model.ListenerState{Status: model.ListenerDraining}),
		listener("rejected", model.ListenerState{Status: model.ListenerRejected, Error: errState}),
		listener("", model.ListenerState{Status: model.ListenerRejected, Error: errState}),
	}}
	out := filter.Filter(snap, opts("team-a", "orders"))

	var got []string
	for _, l := range out.Listeners {
		got = append(got, l.Name)
		if !reflect.DeepEqual(l.State, stateOf(snap, l.Name)) {
			t.Errorf("listener %q state = %+v, want it unchanged", l.Name, l.State)
		}
	}
	if want := []string{"active-error", "active-updating", "active-draining", "warming", "draining", "rejected", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("listeners = %q, want %q", got, want)
	}
	if rs := routeNames(out); rs != nil {
		t.Errorf("routes = %v, want none", rs)
	}
}

func stateOf(snap *model.EnvoySnapshot, name string) model.ListenerState {
	for _, l := range snap.Listeners {
		if l.Name == name {
			return l.State
		}
	}
	return model.ListenerState{}
}

// readings4 reads as a/b-c-d, a-b/c-d and a-b-c/d.
const readings4 = "listener~80~shop-route-0-httproute-a-b-c-d-0-0-matcher-0"

// TestAmbiguous_AllNameReadings: a route Known narrowed may still serve a
// deleted HTTPRoute, so Ambiguous judges it on all its name's readings (#29).
func TestAmbiguous_AllNameReadings(t *testing.T) {
	raw := &model.EnvoySnapshot{Listeners: []model.Listener{
		listener("listener~80", model.ListenerState{Status: model.ListenerActive}, readings4),
	}}
	annotate := func(known ...model.Ref) *model.EnvoySnapshot {
		return origin.Annotate(raw, origin.Kgateway(origin.KgatewayOptions{Known: known}))
	}
	tests := []struct {
		name string
		snap *model.EnvoySnapshot
	}{
		{"Known keeps two other readings", annotate(routeRef("b-c-d", "a"), routeRef("c-d", "a-b"))},
		{"Known keeps one other reading", annotate(routeRef("b-c-d", "a"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := opts("d", "a-b-c")
			if got := routeNames(filter.Filter(tt.snap, o)); got != nil {
				t.Errorf("Filter = %v, want nothing", got)
			}
			if got, want := filter.Ambiguous(tt.snap, o), []string{readings4}; !reflect.DeepEqual(got, want) {
				t.Errorf("Ambiguous = %v, want %v", got, want)
			}
		})
	}

	// Selected routes and exact origins are never listed.
	known := annotate(routeRef("b-c-d", "a"))
	if got := filter.Ambiguous(known, opts("b-c-d", "a")); got != nil {
		t.Errorf("selected route listed: %v", got)
	}
	exact := annotate()
	exact.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0].Origin = &model.RouteAnnotation{
		Certainty: model.Exact, Candidates: []model.RouteOrigin{{Route: routeRef("x", "y")}}}
	if got := filter.Ambiguous(exact, opts("d", "a-b-c")); got != nil {
		t.Errorf("exact origin listed: %v", got)
	}
}

// TestFilter_NameMatchIgnoresVhost: an unannotated route matches only on the
// part after "<vhost>-route-<n>-", not inside the vhost name (#29).
func TestFilter_NameMatchIgnoresVhost(t *testing.T) {
	const vhost = "listener~80~httproute-orders-team-a-example"
	name := vhost + "-route-0-httproute-web-shop-0-0-matcher-0"
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: vhost, Routes: []model.Route{{Name: name}}}}}}}}}}}
	if got := routeNames(filter.Filter(snap, opts("team-a", "orders"))); got != nil {
		t.Errorf("team-a/orders selected %v; the marker is in the vhost name", got)
	}
	if got := routeNames(filter.Filter(snap, opts("shop", "web"))); !reflect.DeepEqual(got, []string{name}) {
		t.Errorf("shop/web selected %v, want the route", got)
	}
}

// TestAmbiguous_ReviewCases: readings come from the route part only, a custom
// mapper's candidates still count, and route_source metadata is authoritative.
func TestAmbiguous_ReviewCases(t *testing.T) {
	route := func(vhost, name string, o *model.RouteAnnotation, meta map[string]any) *model.EnvoySnapshot {
		return &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
			RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: vhost, Routes: []model.Route{{Name: name, Origin: o, Metadata: meta}}}}}}}}}}}
	}
	ambiguous := &model.RouteAnnotation{Certainty: model.Ambiguous, Candidates: []model.RouteOrigin{{Route: routeRef("x", "y")}, {Route: routeRef("z", "w")}}}

	// 1: a vhost carrying its own marker; the route part reads ns/foo only.
	const vh = "listener~80~a-route-0-httproute-web"
	name := vh + "-route-0-httproute-foo-ns-0-0-matcher-0"
	if got := filter.Ambiguous(route(vh, name, ambiguous, nil), opts("ns", "foo")); !reflect.DeepEqual(got, []string{name}) {
		t.Errorf("route part: Ambiguous = %v, want the route", got)
	}

	// 2: a custom mapper's candidates, on a name in another format.
	custom := &model.RouteAnnotation{Certainty: model.Ambiguous, Candidates: []model.RouteOrigin{{Route: routeRef("shop", "web")}, {Route: routeRef("x", "y")}}}
	if got := filter.Ambiguous(route("vh", "acme-route-7", custom, nil), opts("shop", "web")); !reflect.DeepEqual(got, []string{"acme-route-7"}) {
		t.Errorf("custom candidates: Ambiguous = %v, want the route", got)
	}

	// 3: route_source names another HTTPRoute, under a non-Exact annotation.
	meta := map[string]any{"dev.kgateway.route_source": map[string]any{
		"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "x", "name": "y", "rule": "_rule-0", "match": "_match-0"}}
	if got := filter.Ambiguous(route("listener~80~shop", readings4, ambiguous, meta), opts("d", "a-b-c")); got != nil {
		t.Errorf("route_source: Ambiguous = %v, want nothing", got)
	}
}

// TestFilter_NameMatchIgnoresVhostRules: rule index and rule name still apply
// to the route part.
func TestFilter_NameMatchIgnoresVhostRules(t *testing.T) {
	const vhost = "listener~80~httproute-web-shop-1-0-api-matcher-0"
	name := vhost + "-route-0-httproute-web-shop-0-0-matcher-0"
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: vhost, Routes: []model.Route{{Name: name}}}}}}}}}}}
	for _, c := range []struct {
		o    filter.Options
		want []string
	}{
		{filter.Options{HTTPRouteName: "web", HTTPRouteNamespace: "shop", RuleIndex: 0}, []string{name}},
		{filter.Options{HTTPRouteName: "web", HTTPRouteNamespace: "shop", RuleIndex: 1}, nil},
		{filter.Options{HTTPRouteName: "web", HTTPRouteNamespace: "shop", RuleIndex: filter.AllRules, RuleName: "api"}, nil},
	} {
		if got := routeNames(filter.Filter(snap, c.o)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: Filter = %v, want %v", c.o, got, c.want)
		}
	}
}

// TestFilter_RuleIndexFromName: narrowing by rule index selects a route whose
// route_source names the rule, with the index only in the Envoy name, as it
// does one whose metadata carries the index (8.1 brief case 10).
func TestFilter_RuleIndexFromName(t *testing.T) {
	const name = "listener~80~*-route-0-httproute-web-shop-1-0-api-matcher-0"
	for _, rule := range []string{"api", "_rule-1"} {
		meta := map[string]any{"dev.kgateway.route_source": map[string]any{
			"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "shop", "name": "web", "rule": rule, "match": "_match-0"}}
		snap := &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
			RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: "listener~80~*", Routes: []model.Route{{Name: name, Metadata: meta}}}}}}}}}}}
		o := opts("shop", "web")
		o.RuleIndex = 1
		if got := routeNames(filter.Filter(snap, o)); !reflect.DeepEqual(got, []string{name}) {
			t.Errorf("rule %q, index 1: selected %v, want the route", rule, got)
		}
		o.RuleIndex = 0
		if got := routeNames(filter.Filter(snap, o)); got != nil {
			t.Errorf("rule %q, index 0: selected %v, want nothing", rule, got)
		}
	}
}

// TestFilter_NameConflictsWithMetadata: a route name whose rule disagrees with
// route_source is not evidence for that rule (phase review #1).
func TestFilter_NameConflictsWithMetadata(t *testing.T) {
	for _, name := range []string{
		"listener~80~*-route-0-httproute-web-shop-2-0-admin-matcher-0",
		"listener~80~*-route-0-httproute-web-shop-2-0-matcher-0", // unnamed rule 2
	} {
		nameConflict(t, name)
	}
}

func nameConflict(t *testing.T, name string) {
	t.Helper()
	meta := map[string]any{"dev.kgateway.route_source": map[string]any{
		"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "shop", "name": "web", "rule": "api", "match": "_match-0"}}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: "listener~80~*", Routes: []model.Route{{Name: name, Metadata: meta}}}}}}}}}}}
	o := opts("shop", "web")
	o.RuleIndex = 2
	if got := routeNames(filter.Filter(snap, o)); got != nil {
		t.Errorf("%s, index 2: selected %v, want nothing", name, got)
	}
	o.RuleIndex, o.RuleName = -1, "api"
	if got := routeNames(filter.Filter(snap, o)); !reflect.DeepEqual(got, []string{name}) {
		t.Errorf("%s, rule name api: selected %v, want the route", name, got)
	}
}

// TestFilter_UnnamedRuleNotSelectedByName: "_rule-<n>" is an unnamed rule, so
// a rule name read from the Envoy name doesn't select it (phase fix review 2 #1).
func TestFilter_UnnamedRuleNotSelectedByName(t *testing.T) {
	const name = "listener~80~*-route-0-httproute-web-shop-2-0-api-matcher-0"
	meta := map[string]any{"dev.kgateway.route_source": map[string]any{
		"kind": "HTTPRoute", "group": "gateway.networking.k8s.io", "namespace": "shop", "name": "web", "rule": "_rule-2", "match": "_match-0"}}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{{Name: "l", FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Name: "listener~80~*", Routes: []model.Route{{Name: name, Metadata: meta}}}}}}}}}}}
	o := opts("shop", "web")
	o.RuleName = "api"
	if got := routeNames(filter.Filter(snap, o)); got != nil {
		t.Errorf("rule name api: selected %v, want nothing", got)
	}
	o.RuleName, o.RuleIndex = "", 2
	if got := routeNames(filter.Filter(snap, o)); !reflect.DeepEqual(got, []string{name}) {
		t.Errorf("index 2: selected %v, want the route", got)
	}
}

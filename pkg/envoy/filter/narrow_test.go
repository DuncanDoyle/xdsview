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
		{"known orders/team-a: orders-team/a not selected", known, opts("a", "orders-team"), nil, nil},
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

// TestFilter_ListenersThatExplain: listeners that aren't active, or that have
// an error, survive narrowing with their state even without a matching route.
func TestFilter_ListenersThatExplain(t *testing.T) {
	other := "listener~80~shop-route-0-httproute-other-ns-0-0-matcher-0"
	errState := &model.ListenerError{Details: "bad"}
	snap := &model.EnvoySnapshot{Listeners: []model.Listener{
		listener("active", model.ListenerState{Status: model.ListenerActive}, other),
		listener("active-error", model.ListenerState{Status: model.ListenerActive, Error: errState}, other),
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
	if want := []string{"active-error", "warming", "draining", "rejected", ""}; !reflect.DeepEqual(got, want) {
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

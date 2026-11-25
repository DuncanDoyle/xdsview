package model_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// listenerWith builds listener L<i> with one chain per vhost list; each chain's
// HCM uses source RD<i> and each route is named by its ID's last parts.
func listenerWith(i int, state model.ListenerState, chains ...[][]string) model.Listener {
	l := model.Listener{ID: model.ID(fmt.Sprintf("L%d", i)), Name: fmt.Sprintf("l%d", i), State: state}
	for c, vhosts := range chains {
		fc := model.NetworkFilterChain{ID: model.ID(fmt.Sprintf("%s/C%d", l.ID, c))}
		hcm := &model.HCMConfig{ID: fc.ID + "/N0"}
		hcm.HTTPFilters = []model.HTTPFilter{{ID: hcm.ID + "/F0", Name: "router"}}
		hcm.RouteConfigUse = model.RouteConfigUse{ID: hcm.ID + "/RC", Source: model.ID(fmt.Sprintf("RD%d", i)), State: model.RouteConfigFound}
		rc := &model.RouteConfig{Name: "rc"}
		for v, routes := range vhosts {
			vh := model.VirtualHost{ID: model.ID(fmt.Sprintf("%s/V%d", hcm.RouteConfigUse.ID, v)), Name: fmt.Sprintf("vh%d", v)}
			for r, name := range routes {
				vh.Routes = append(vh.Routes, model.Route{ID: model.ID(fmt.Sprintf("%s/R%d", vh.ID, r)), Name: name})
			}
			rc.VirtualHosts = append(rc.VirtualHosts, vh)
		}
		hcm.RouteConfig = rc
		fc.HCM = hcm
		l.FilterChains = append(l.FilterChains, fc)
	}
	return l
}

var active = model.ListenerState{Status: model.ListenerActive}

// keepNames keeps routes whose name is in names.
func keepNames(snap *model.Snapshot, names ...string) func(model.ID) bool {
	ids := map[model.ID]bool{}
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					for _, n := range names {
						if r.Name == n {
							ids[r.ID] = true
						}
					}
				}
			}
		}
	}
	return func(id model.ID) bool { return ids[id] }
}

// shape prints listeners, chains, vhosts and routes by ID.
func shape(snap *model.Snapshot) []string {
	var out []string
	for _, l := range snap.Listeners {
		out = append(out, string(l.ID))
		for _, fc := range l.FilterChains {
			out = append(out, string(fc.ID))
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				out = append(out, string(vh.ID))
				for _, r := range vh.Routes {
					out = append(out, string(r.ID))
				}
			}
		}
	}
	return out
}

// TestPrune_RoutesAndAncestors: selected routes keep their vhost, chain and
// listener; empty vhosts go; a chain without selected routes keeps its HCM
// with no vhosts; an active listener without selected routes goes.
func TestPrune_RoutesAndAncestors(t *testing.T) {
	snap := &model.Snapshot{
		Capture: model.CaptureComplete,
		Listeners: []model.Listener{
			listenerWith(0, active, [][]string{{"a", "b"}, {"c"}}, [][]string{{"d"}}),
			listenerWith(1, active, [][]string{{"e"}}),
		},
		RouteConfigs: []model.RouteConfigSource{{ID: "RD0", Kind: model.SourceDynamic, Name: "rc"}, {ID: "RD1", Kind: model.SourceDynamic, Name: "rc1"}, {ID: "RS0", Kind: model.SourceStatic, Name: "s"}},
		Warnings: []model.Warning{
			{Code: model.WarnSectionSkipped, Message: "dump-wide"},
			{Subject: "L0/C1/N0/RC", Code: model.WarnRDSAmbiguous, Message: "kept listener"},
			{Subject: "L1/C0", Code: model.WarnHCMSkipped, Message: "dropped listener"},
		},
	}
	before := shape(snap)
	got := model.Prune(snap, keepNames(snap, "b"))

	want := []string{"L0", "L0/C0", "L0/C0/N0/RC/V0", "L0/C0/N0/RC/V0/R1", "L0/C1"}
	if s := shape(got); !reflect.DeepEqual(s, want) {
		t.Errorf("pruned = %v\nwant %v", s, want)
	}
	if hcm := got.Listeners[0].FilterChains[1].HCM; hcm == nil || hcm.RouteConfig == nil || len(hcm.RouteConfig.VirtualHosts) != 0 || len(hcm.HTTPFilters) != 1 {
		t.Errorf("chain without selected routes = %+v, want its HCM with no vhosts", hcm)
	}
	if got.Capture != model.CaptureComplete {
		t.Errorf("capture = %q", got.Capture)
	}
	if want := []model.RouteConfigSource{snap.RouteConfigs[0]}; !reflect.DeepEqual(got.RouteConfigs, want) {
		t.Errorf("sources = %+v, want only the retained uses' %+v", got.RouteConfigs, want)
	}
	if want := snap.Warnings[:2]; !reflect.DeepEqual(got.Warnings, want) {
		t.Errorf("warnings = %+v, want %+v", got.Warnings, want)
	}
	if !reflect.DeepEqual(shape(snap), before) {
		t.Error("Prune changed its input")
	}
}

// TestPrune_WarningSubjects: a warning stays only when its own subject is
// retained, not merely its listener (design §7).
func TestPrune_WarningSubjects(t *testing.T) {
	snap := &model.Snapshot{
		Listeners: []model.Listener{listenerWith(0, active, [][]string{{"a", "b"}, {"c"}})},
		Warnings: []model.Warning{
			{Subject: "L0/C0/N0/RC/V0/R1", Message: "kept route"},
			{Subject: "L0/C0/N0/RC/V0", Message: "kept vhost"},
			{Subject: "L0/C0/N0/F0", Message: "kept filter"},
			{Subject: "L0/C0/N0/RC/V0/R0", Message: "pruned route"},
			{Subject: "L0/C0/N0/RC/V1", Message: "pruned vhost"},
			{Subject: "L0/C0/N0/RC/V1/R0", Message: "route of pruned vhost"},
		},
	}
	got := model.Prune(snap, keepNames(snap, "b"))
	if want := snap.Warnings[:3]; !reflect.DeepEqual(got.Warnings, want) {
		t.Errorf("warnings = %+v\nwant %+v", got.Warnings, want)
	}
}

// TestPrune_ListenersThatExplain: listeners that may explain a missing route
// stay, with their state, when no route of theirs is selected (ported from filter).
func TestPrune_ListenersThatExplain(t *testing.T) {
	errState := &model.ListenerError{Details: "bad"}
	states := []model.ListenerState{
		active,
		{Status: model.ListenerActive, Error: errState},
		{Status: model.ListenerActive, Updating: true},
		{Status: model.ListenerActive, Draining: true},
		{Status: model.ListenerWarming},
		{Status: model.ListenerDraining},
		{Status: model.ListenerRejected, Error: errState},
		{}, // no status: treated as active
	}
	snap := &model.Snapshot{}
	for i, s := range states {
		snap.Listeners = append(snap.Listeners, listenerWith(i, s, [][]string{{"other"}}))
	}
	got := model.Prune(snap, func(model.ID) bool { return false })

	var ids []model.ID
	for _, l := range got.Listeners {
		ids = append(ids, l.ID)
		i := 0
		fmt.Sscanf(string(l.ID), "L%d", &i)
		if !reflect.DeepEqual(l.State, states[i]) {
			t.Errorf("%s state = %+v, want it unchanged", l.ID, l.State)
		}
		if vhs := l.FilterChains[0].HCM.RouteConfig.VirtualHosts; len(vhs) != 0 {
			t.Errorf("%s kept vhosts %+v", l.ID, vhs)
		}
	}
	if want := []model.ID{"L1", "L2", "L3", "L4", "L5", "L6"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("listeners = %v, want %v", ids, want)
	}
}

// TestPrune_NoRouteConfig: chains without an HCM or a joined route config are
// kept as they are in a retained listener.
func TestPrune_NoRouteConfig(t *testing.T) {
	l := listenerWith(0, active, [][]string{{"a"}})
	l.FilterChains = append(l.FilterChains,
		model.NetworkFilterChain{ID: "L0/C1"},
		model.NetworkFilterChain{ID: "L0/C2", HCM: &model.HCMConfig{ID: "L0/C2/N0", RouteConfigUse: model.RouteConfigUse{ID: "L0/C2/N0/RC", State: model.RouteConfigAmbiguous}}})
	got := model.Prune(&model.Snapshot{Listeners: []model.Listener{l}}, func(id model.ID) bool { return id == "L0/C0/N0/RC/V0/R0" })
	if want := []string{"L0", "L0/C0", "L0/C0/N0/RC/V0", "L0/C0/N0/RC/V0/R0", "L0/C1", "L0/C2"}; !reflect.DeepEqual(shape(got), want) {
		t.Errorf("pruned = %v, want %v", shape(got), want)
	}
}

package model_test

import (
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

func TestCompareID(t *testing.T) {
	ids := []model.ID{"RS0", "L10", "L2/C0/N0/RC/V0/R10", "L2/C0/N0/RC/V0/R2", "L2/C0/N0/RCI", "L2/C0/N0/RC", "L2/C0", "L2/Cd", "L2", "RD3", "RD12"}
	slices.SortFunc(ids, model.CompareID)
	want := []model.ID{"L2", "L2/C0", "L2/C0/N0/RC", "L2/C0/N0/RC/V0/R2", "L2/C0/N0/RC/V0/R10", "L2/C0/N0/RCI", "L2/Cd", "L10", "RD3", "RD12", "RS0"}
	if !slices.Equal(ids, want) {
		t.Errorf("sorted = %v\nwant     %v", ids, want)
	}
	if model.CompareID("L1", "L1") != 0 {
		t.Error("L1 != L1")
	}
}

func TestSnapshot_Lookups(t *testing.T) {
	snap := &model.Snapshot{
		Listeners: []model.Listener{
			listenerWith(0, model.ListenerState{Status: model.ListenerActive}, [][]string{{"a", "b"}}),
			listenerWith(1, model.ListenerState{Status: model.ListenerActive}, [][]string{{"c"}}),
		},
		RouteConfigs: []model.RouteConfigSource{{ID: "RD0", Name: "rc"}},
	}
	route := snap.Listeners[1].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0]
	if r, ok := snap.Route(route.ID); !ok || r.Name != route.Name {
		t.Errorf("Route(%s) = %v, %v", route.ID, r.Name, ok)
	}
	vh := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0]
	if got, ok := snap.VirtualHost(vh.ID); !ok || got.ID != vh.ID {
		t.Errorf("VirtualHost(%s) = %v, %v", vh.ID, got.ID, ok)
	}
	if l, ok := snap.Listener("L1"); !ok || l.ID != "L1" {
		t.Errorf("Listener(L1) = %v, %v", l.ID, ok)
	}
	slot := snap.Listeners[0].FilterChains[0].HCM.HTTPFilters[0]
	if f, ok := snap.Slot(slot.ID); !ok || f.Name != slot.Name {
		t.Errorf("Slot(%s) = %v, %v", slot.ID, f.Name, ok)
	}
	if s, ok := snap.RouteConfigSource("RD0"); !ok || s.Name != "rc" {
		t.Errorf("RouteConfigSource(RD0) = %v, %v", s, ok)
	}
	for _, id := range []model.ID{"L2", "L0/C0/N0/RC/V9", "L1/C0/N0/F9", "RD9"} {
		_, a := snap.Listener(id)
		_, b := snap.VirtualHost(id)
		_, c := snap.Slot(id)
		_, d := snap.RouteConfigSource(id)
		if a || b || c || d {
			t.Errorf("%s found", id)
		}
	}
}

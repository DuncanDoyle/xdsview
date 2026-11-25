package parser_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const fixtures = "../../../testdata/fixtures/"

// TestStaticListeners: static listeners are parsed after the dynamic ones,
// marked Static and active. An inline route config records the static entry
// Envoy reports for it; a static entry no HCM uses stays a source.
func TestStaticListeners(t *testing.T) {
	snap := parseFile(t, fixtures+"static-bootstrap/envoy/config_dump.json")
	if len(snap.Listeners) != 1 {
		t.Fatalf("listeners = %d, want 1", len(snap.Listeners))
	}
	l := snap.Listeners[0]
	if l.ID != "L0" || l.Name != "health_listener" || !l.Static || l.State.Status != model.ListenerActive || l.Address != ":::9902" {
		t.Errorf("listener = %+v, want static active L0 health_listener on :::9902", l)
	}
	hcm := l.FilterChains[0].HCM
	if hcm == nil || hcm.RouteConfig == nil || hcm.RouteConfig.VirtualHosts[0].Routes[0].Cluster != "local_service" {
		t.Fatalf("hcm = %+v, want the inline route config joined", hcm)
	}
	want := []model.RouteConfigSource{
		{ID: "L0/C0/N0/RCI", Kind: model.SourceInline, Name: "local_route", Reported: "RS0"},
		{ID: "RS1", Kind: model.SourceStatic, Name: "spare_route"},
	}
	if !reflect.DeepEqual(snap.RouteConfigs, want) {
		t.Errorf("sources = %+v\nwant %+v", snap.RouteConfigs, want)
	}

	// Scenario 00: the dynamic listener keeps L0; the static ones follow.
	snap = parseFile(t, "../../../testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json")
	var got []string
	for _, l := range snap.Listeners {
		got = append(got, string(l.ID)+" "+l.Name)
		if l.Static != (l.Name != "listener~80") {
			t.Errorf("%s: Static = %v", l.Name, l.Static)
		}
	}
	if want := []string{"L0 listener~80", "L1 readiness_listener", "L2 prometheus_listener"}; !reflect.DeepEqual(got, want) {
		t.Errorf("listeners = %v, want %v", got, want)
	}
}

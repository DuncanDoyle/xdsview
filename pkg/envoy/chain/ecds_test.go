package chain_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/internal/snaptest"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// TestEffective_ECDS: an ECDS filter runs its ECDS or default config; one
// with neither stays in position with Runs unknown and State unresolved.
func TestEffective_ECDS(t *testing.T) {
	snap := parseFile(t, "../../../testdata/fixtures/ecds/envoy/config_dump.json")
	route := snap.Listeners[0].FilterChains[0].HCM.RouteConfig.VirtualHosts[0].Routes[0].ID
	c := chain.Effective(snap)[route]
	type row struct {
		Name, Source, Wrapper string
		Runs                  model.Tri
		State                 model.FilterState
	}
	var got []row
	for _, e := range c.Filters {
		got = append(got, row{e.Name, e.ConfigSource, e.Wrapper, e.Runs, e.State})
	}
	want := []row{
		{"ecds.found", model.ConfigECDS, "", model.TriYes, model.FilterOn},
		{"ecds.wrong_type", "", "", model.TriUnknown, model.FilterUnresolved},
		{"ecds.default", model.ConfigECDSDefault, "", model.TriYes, model.FilterOn},
		{"ecds.default_no_warming", model.ConfigECDSDefault, "", model.TriYes, model.FilterOn},
		{"ecds.none", "", "", model.TriUnknown, model.FilterUnresolved},
		{"ecds.matched", model.ConfigECDS, model.WrapperExtensionWithMatcher, model.TriYes, model.FilterOn},
		{"envoy.filters.http.router", model.ConfigInline, "", model.TriYes, model.FilterOn},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chain =\n%+v\nwant\n%+v", got, want)
	}
	if n := len(c.RequestOrder()); n != len(want) {
		t.Errorf("request order has %d entries, want all %d (unresolved ones stay in position)", n, len(want))
	}

	// Disabled in the HCM and not enabled for the route: it doesn't run,
	// whatever its config.
	off := snaptest.IDs(&model.Snapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		HTTPFilters: []model.HTTPFilter{{Name: "x", Disabled: true, Discovery: &model.Discovery{}}},
		RouteConfig: &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Routes: []model.Route{{Name: "r"}}}}},
	}}}}}})
	e := chain.Effective(off)["L0/C0/N0/RC/V0/R0"].Filters[0]
	if e.Runs != model.TriNo || e.State != model.FilterOff || e.ConfigSource != "" {
		t.Errorf("disabled unresolved entry = %+v, want off", e)
	}
}

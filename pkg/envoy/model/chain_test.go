package model_test

import (
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

func names(es []model.ChainEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

// TestChainOrders covers which entries each order keeps and in what order:
// non-running entries and entries without that side drop out; unknown sides stay.
func TestChainOrders(t *testing.T) {
	c := model.EffectiveChain{Filters: []model.ChainEntry{
		{Name: "both", Runs: true, Request: model.SideYes, Response: model.SideYes},
		{Name: "off", Runs: false, Request: model.SideYes, Response: model.SideYes},
		{Name: "decoder-only", Runs: true, Request: model.SideYes, Response: model.SideNo},
		{Name: "encoder-only", Runs: true, Request: model.SideNo, Response: model.SideYes},
		{Name: "unknown", Runs: true, Request: model.SideUnknown, Response: model.SideUnknown},
		{Name: "router", Runs: true, Request: model.SideYes, Response: model.SideNo},
	}}

	if got, want := names(c.RequestOrder()), []string{"both", "decoder-only", "unknown", "router"}; !slices.Equal(got, want) {
		t.Errorf("RequestOrder = %v, want %v", got, want)
	}
	if got, want := names(c.ResponseOrder()), []string{"unknown", "encoder-only", "both"}; !slices.Equal(got, want) {
		t.Errorf("ResponseOrder = %v, want %v", got, want)
	}
	if got := (model.EffectiveChain{}).RequestOrder(); got != nil {
		t.Errorf("empty chain RequestOrder = %v, want nil", got)
	}
}

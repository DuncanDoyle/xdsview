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
// non-running entries and entries without that side drop out; an unknown side
// or run state stays in position.
func TestChainOrders(t *testing.T) {
	yes, no, unknown := model.TriYes, model.TriNo, model.TriUnknown
	c := model.EffectiveChain{Filters: []model.ChainEntry{
		{Name: "both", Runs: yes, Request: yes, Response: yes},
		{Name: "off", Runs: no, Request: yes, Response: yes},
		{Name: "decoder-only", Runs: yes, Request: yes, Response: no},
		{Name: "encoder-only", Runs: yes, Request: no, Response: yes},
		{Name: "unknown", Runs: yes, Request: unknown, Response: unknown},
		{Name: "maybe", Runs: unknown, Request: yes, Response: no},
		{Name: "router", Runs: yes, Request: yes, Response: no},
	}}

	if got, want := names(c.RequestOrder()), []string{"both", "decoder-only", "unknown", "maybe", "router"}; !slices.Equal(got, want) {
		t.Errorf("RequestOrder = %v, want %v", got, want)
	}
	if got, want := names(c.ResponseOrder()), []string{"unknown", "encoder-only", "both"}; !slices.Equal(got, want) {
		t.Errorf("ResponseOrder = %v, want %v", got, want)
	}
	if got := (model.EffectiveChain{}).RequestOrder(); got != nil {
		t.Errorf("empty chain RequestOrder = %v, want nil", got)
	}
}

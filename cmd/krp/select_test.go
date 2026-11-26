package main

import (
	"slices"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// echo repeats the kgateway module's RouteSource readings with Exhaustive
// flipped: the same readings by Resolve's identity, from a second module.
type echo struct{}

func (echo) Name() string { return "echo" }

func (echo) Claims(snap *model.Snapshot) []provenance.Claim {
	var out []provenance.Claim
	for _, c := range kgateway.New().Claims(snap) {
		if rs, ok := c.Assertion.(provenance.RouteSource); ok {
			rs.Exhaustive = !rs.Exhaustive
			c.Assertion, c.Ref = rs, ""
			out = append(out, c)
		}
	}
	return out
}

// TestSelection_D022CountsReadings: D-022 selects a narrowed route when one
// reading is kept, counting readings as Resolve merges them, not claims.
func TestSelection_D022CountsReadings(t *testing.T) {
	route := model.Ref{Group: gatewayGroup, Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com"}
	q := kgateway.Query{Route: route, Known: []model.Ref{route}}
	var byD022 int
	for _, dump := range allDumps(t) {
		snap := parseFile(t, dump)
		base, err := build(snap, request{query: &q})
		if err != nil {
			t.Fatal(err)
		}
		extraModules = []provenance.Module{echo{}}
		dup, err := build(snap, request{query: &q})
		extraModules = nil
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(base.selection.Routes, dup.selection.Routes) {
			t.Errorf("%s: routes %v with a repeated reading, want %v", dump, dup.selection.Routes, base.selection.Routes)
		}
		for _, n := range dup.selection.Narrowed {
			if n.Selected {
				byD022++
			}
		}
	}
	if byD022 == 0 {
		t.Error("no route selected by D-022 with a repeated reading: the test checks nothing")
	}
}

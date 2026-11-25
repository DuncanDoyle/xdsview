package provenance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

func dumps(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, g := range []string{"scenarios/*/envoy/*/config_dump.json", "fixtures/*/envoy/config_dump.json"} {
		m, _ := filepath.Glob(filepath.Join("../../../testdata", g))
		out = append(out, m...)
	}
	if len(out) == 0 {
		t.Fatal("no dumps")
	}
	return out
}

func parseFile(t *testing.T, path string) *model.Snapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	snap, err := parser.Parse(f, parser.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// vandal is a module that changes every map, slice and pointer it can reach
// in its copy of the snapshot, and claims nothing. It uses exported API only.
type vandal struct{}

func (vandal) Name() string { return "vandal" }

func (vandal) Claims(snap *model.Snapshot) []provenance.Claim {
	scribble := func(m map[string]any) {
		for k, v := range m {
			if inner, ok := v.(map[string]any); ok {
				inner["vandal"] = true
			}
			if list, ok := v.([]any); ok && len(list) > 0 {
				list[0] = "vandal"
			}
			m[k] = v
		}
		if m != nil {
			m["vandal"] = []any{"x"}
		}
	}
	for i := range snap.Listeners {
		l := &snap.Listeners[i]
		l.Name += "-vandal"
		scribble(l.Metadata)
		if l.State.Error != nil {
			l.State.Error.Details = "vandal"
		}
		for j := range l.FilterChains {
			fc := &l.FilterChains[j]
			if fc.TLS != nil {
				fc.TLS.SNIHosts[0] = "vandal"
			}
			for k := range fc.NetworkFilters {
				fc.NetworkFilters[k].Name = "vandal"
			}
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			for k := range hcm.HTTPFilters {
				f := &hcm.HTTPFilters[k]
				f.Name, f.Disabled = "vandal", !f.Disabled
				scribble(f.TypedConfig)
				if f.Discovery != nil {
					f.Discovery.Source = "vandal"
				}
			}
			hcm.UpstreamFilters = append(hcm.UpstreamFilters, model.HTTPFilter{Name: "vandal"})
			rc := hcm.RouteConfig
			if rc == nil {
				continue
			}
			scribble(rc.TypedPerFilterConfig)
			for v := range rc.VirtualHosts {
				vh := &rc.VirtualHosts[v]
				scribble(vh.TypedPerFilterConfig)
				scribble(vh.Metadata)
				if len(vh.Domains) > 0 {
					vh.Domains[0] = "vandal"
				}
				for r := range vh.Routes {
					rt := &vh.Routes[r]
					scribble(rt.Metadata)
					scribble(rt.TypedPerFilterConfig)
					rt.Cluster = "vandal"
					if rt.Rewrite != nil {
						rt.Rewrite.Prefix = "vandal"
					}
					if rt.Match.CaseSensitive != nil {
						*rt.Match.CaseSensitive = !*rt.Match.CaseSensitive
					}
					for w := range rt.WeightedClusters {
						rt.WeightedClusters[w].Weight++
						scribble(rt.WeightedClusters[w].TypedPerFilterConfig)
					}
					if rt.Action.DirectResponse != nil && rt.Action.DirectResponse.BodySize != nil {
						*rt.Action.DirectResponse.BodySize++
					}
				}
				vh.Routes = append(vh.Routes, model.Route{Name: "vandal"})
			}
		}
	}
	for i := range snap.RouteConfigs {
		scribble(snap.RouteConfigs[i].Metadata)
		scribble(snap.RouteConfigs[i].TypedPerFilterConfig)
	}
	snap.Warnings = append(snap.Warnings, model.Warning{Code: "vandal"})
	return nil
}

// TestCompose_Invariance (R2): composing the kgateway module with a module
// that mutates its copy leaves the snapshot and its execution results equal
// to an independent parse's, and the kgateway claims equal to composing it alone.
func TestCompose_Invariance(t *testing.T) {
	for _, dump := range dumps(t) {
		t.Run(dump, func(t *testing.T) {
			a, b := parseFile(t, dump), parseFile(t, dump)
			alone, err := provenance.Compose(parseFile(t, dump), kgateway.New())
			if err != nil {
				t.Fatal(err)
			}
			ix, err := provenance.Compose(a, vandal{}, kgateway.New())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Error("Compose changed the snapshot")
			}
			if !reflect.DeepEqual(chain.Effective(a), chain.Effective(b)) {
				t.Error("Compose changed the effective chains")
			}
			if !reflect.DeepEqual(ix.Claims(), alone.Claims()) {
				t.Error("the vandal changed the kgateway claims")
			}
		})
	}
}

package provenance_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// TestDependencies_FailClosed checks, for every corpus dump and fixture and
// every listener, virtual host, route and source block, that removing any
// single claim covering an input from the kgateway module's output adds that
// input's own gap. An input that several claims cover redundantly loses all of
// them at once. TestDependencies_Oracle checks the inputs are complete.
func TestDependencies_FailClosed(t *testing.T) {
	type check struct {
		block model.ID
		gap   provenance.Gap
	}
	for _, path := range dumps(t) {
		name, _ := filepath.Rel("../../../testdata", filepath.Dir(path))
		snap := parseFile(t, path)
		claims := kgateway.New().Claims(parseFile(t, path))
		ix := compose(t, snap, fixed{"m", claims})
		kind := map[string]string{}
		for _, c := range ix.Claims() {
			kind[c.ID] = c.Assertion.Kind()
		}
		removals := map[string][]check{} // claim IDs to remove → the gaps each block must gain
		for _, b := range blockIDs(snap) {
			for _, c := range provenance.Covers(ix, b) {
				ids := slices.Compact(slices.Sorted(slices.Values(c.IDs)))
				kinds := map[string]bool{}
				for _, id := range ids {
					kinds[kind[id]] = true
				}
				if len(kinds) < len(ids) {
					// Redundant: remove the whole cover.
					ids = []string{strings.Join(ids, " ")}
				}
				for _, id := range ids {
					removals[id] = append(removals[id], check{b, c.Gap})
				}
			}
		}
		for key, checks := range removals {
			drop := map[int]bool{}
			for _, id := range strings.Fields(key) {
				n, err := strconv.Atoi(strings.TrimPrefix(id, "m/"))
				if err != nil {
					t.Fatal(err)
				}
				drop[n] = true
			}
			var kept []provenance.Claim
			for i, c := range claims {
				if !drop[i] {
					kept = append(kept, c)
				}
			}
			without := compose(t, snap, fixed{"m", kept})
			for _, c := range checks {
				if !slices.Contains(without.Dependencies(c.block).Gaps, c.gap) {
					t.Errorf("%s: %s: removing %s (%s) adds no gap %+v", name, c.block, key, kind[strings.Fields(key)[0]], c.gap)
				}
			}
		}
		t.Logf("%s: %d removals checked", name, len(removals))
	}
}

// blockIDs returns every listener, virtual host, route and route config
// source in snap.
func blockIDs(snap *model.Snapshot) []model.ID {
	var out []model.ID
	for _, s := range snap.RouteConfigs {
		out = append(out, s.ID)
	}
	for _, l := range snap.Listeners {
		out = append(out, l.ID)
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				out = append(out, vh.ID)
				for _, r := range vh.Routes {
					out = append(out, r.ID)
				}
			}
		}
	}
	return out
}

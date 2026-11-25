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
// every route block, that removing any single claim covering an input from
// the kgateway module's output adds a gap to the block. An input that several
// claims cover redundantly loses all of them at once.
func TestDependencies_FailClosed(t *testing.T) {
	for _, path := range dumps(t) {
		name, _ := filepath.Rel("../../../testdata", filepath.Dir(path))
		snap := parseFile(t, path)
		claims := kgateway.New().Claims(parseFile(t, path))
		ix := compose(t, snap, fixed{"m", claims})
		kind := map[string]string{}
		for _, c := range ix.Claims() {
			kind[c.ID] = c.Assertion.Kind()
		}
		removals := map[string][]model.ID{} // claim IDs to remove → route blocks to check
		for _, r := range routeIDs(snap) {
			for _, c := range provenance.Covers(ix, r) {
				ids := c.IDs
				slices.Sort(ids)
				ids = slices.Compact(ids)
				kinds := map[string]bool{}
				for _, id := range ids {
					kinds[kind[id]] = true
				}
				if len(kinds) < len(ids) {
					// Redundant: remove the whole cover.
					removals[strings.Join(ids, " ")] = append(removals[strings.Join(ids, " ")], r)
					continue
				}
				for _, id := range ids {
					removals[id] = append(removals[id], r)
				}
			}
		}
		for key, routes := range removals {
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
			for _, r := range slices.Compact(routes) {
				before, after := ix.Dependencies(r).Gaps, without.Dependencies(r).Gaps
				if !slices.ContainsFunc(after, func(g provenance.Gap) bool { return !slices.Contains(before, g) }) {
					t.Errorf("%s: %s: removing %s (%s) adds no gap", name, r, key, kind[strings.Fields(key)[0]])
				}
			}
		}
		t.Logf("%s: %d removals checked", name, len(removals))
	}
}

// routeIDs returns every route in snap.
func routeIDs(snap *model.Snapshot) []model.ID {
	var out []model.ID
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					out = append(out, r.ID)
				}
			}
		}
	}
	return out
}

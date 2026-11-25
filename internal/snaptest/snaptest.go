// Package snaptest helps tests that build a [model.Snapshot] by hand.
package snaptest

import (
	"fmt"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// IDs sets every empty occurrence ID in s the way parser.Parse does, so a
// hand-built snapshot can be keyed by ID. It changes s in place and returns it.
func IDs(s *model.Snapshot) *model.Snapshot {
	set := func(id *model.ID, format string, args ...any) {
		if *id == "" {
			*id = model.ID(fmt.Sprintf(format, args...))
		}
	}
	for i := range s.Listeners {
		l := &s.Listeners[i]
		set(&l.ID, "L%d", i)
		for j := range l.FilterChains {
			fc := &l.FilterChains[j]
			set(&fc.ID, "%s/C%d", l.ID, j)
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			set(&hcm.ID, "%s/N0", fc.ID)
			set(&hcm.RouteConfigUse.ID, "%s/RC", hcm.ID)
			for k := range hcm.HTTPFilters {
				set(&hcm.HTTPFilters[k].ID, "%s/F%d", hcm.ID, k)
			}
			for k := range hcm.UpstreamFilters {
				set(&hcm.UpstreamFilters[k].ID, "%s/U%d", hcm.ID, k)
			}
			if hcm.RouteConfig == nil {
				continue
			}
			for v := range hcm.RouteConfig.VirtualHosts {
				vh := &hcm.RouteConfig.VirtualHosts[v]
				set(&vh.ID, "%s/V%d", hcm.RouteConfigUse.ID, v)
				for r := range vh.Routes {
					rt := &vh.Routes[r]
					set(&rt.ID, "%s/R%d", vh.ID, r)
					for w := range rt.WeightedClusters {
						set(&rt.WeightedClusters[w].ID, "%s/W%d", rt.ID, w)
					}
				}
			}
		}
	}
	return s
}

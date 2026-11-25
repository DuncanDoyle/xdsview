package model

import "slices"

// Prune returns a copy of snap that keeps the routes for which keep returns
// true, and their ancestors. IDs are unchanged, so results computed on snap
// still join.
//
// Pruning works per listener:
//   - A virtual host with no kept route is removed.
//   - Every filter chain of a retained listener stays; a chain with no kept
//     route keeps its HCM with no virtual hosts, so its filters still show.
//   - A listener with no kept route is removed unless it may explain a missing
//     route: it is not active, or it is active with a version warming or
//     draining beside it, or Envoy rejected its last update.
//
// Route config sources stay when a retained HCM uses them. Warnings stay when
// they are dump-wide or their subject is itself retained.
// The copy shares maps and unpruned slices with snap; treat both as read-only.
func Prune(snap *Snapshot, keep func(ID) bool) *Snapshot {
	out := *snap
	out.Listeners = nil
	out.RouteConfigs = nil
	out.Warnings = nil
	used := map[ID]bool{}
	for _, l := range snap.Listeners {
		pl, kept := pruneListener(l, keep)
		if !kept && !explains(l) {
			continue
		}
		out.Listeners = append(out.Listeners, pl)
		for _, fc := range pl.FilterChains {
			if fc.HCM != nil && fc.HCM.RouteConfigUse.Source != "" {
				used[fc.HCM.RouteConfigUse.Source] = true
			}
		}
	}
	for _, s := range snap.RouteConfigs {
		if used[s.ID] {
			out.RouteConfigs = append(out.RouteConfigs, s)
		}
	}
	dropped := prunedBlocks(snap, &out)
	for _, w := range snap.Warnings {
		if w.Subject == "" || retained(&out, w.Subject) && !slices.ContainsFunc(dropped, w.Subject.Within) {
			out.Warnings = append(out.Warnings, w)
		}
	}
	return &out
}

// prunedBlocks returns the vhosts and routes of snap that out no longer holds.
func prunedBlocks(snap, out *Snapshot) []ID {
	kept := map[ID]bool{}
	walkRoutes(out, func(id ID) { kept[id] = true })
	var dropped []ID
	walkRoutes(snap, func(id ID) {
		if !kept[id] {
			dropped = append(dropped, id)
		}
	})
	return dropped
}

// walkRoutes calls fn with the ID of every virtual host and route in snap.
func walkRoutes(snap *Snapshot, fn func(ID)) {
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				fn(vh.ID)
				for _, r := range vh.Routes {
					fn(r.ID)
				}
			}
		}
	}
}

// pruneListener returns l with only the kept routes, and whether any was kept.
func pruneListener(l Listener, keep func(ID) bool) (Listener, bool) {
	kept := false
	chains := make([]NetworkFilterChain, len(l.FilterChains))
	for i, fc := range l.FilterChains {
		if fc.HCM != nil && fc.HCM.RouteConfig != nil {
			hcm := *fc.HCM
			rc := *hcm.RouteConfig
			rc.VirtualHosts = nil
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				var routes []Route
				for _, r := range vh.Routes {
					if keep(r.ID) {
						routes = append(routes, r)
					}
				}
				if len(routes) > 0 {
					vh.Routes = routes
					rc.VirtualHosts = append(rc.VirtualHosts, vh)
					kept = true
				}
			}
			hcm.RouteConfig = &rc
			fc.HCM = &hcm
		}
		chains[i] = fc
	}
	l.FilterChains = chains
	return l, kept
}

// retained reports whether id lies in a listener or source of snap.
func retained(snap *Snapshot, id ID) bool {
	for _, l := range snap.Listeners {
		if id.Within(l.ID) {
			return true
		}
	}
	for _, s := range snap.RouteConfigs {
		if id.Within(s.ID) {
			return true
		}
	}
	return false
}

// explains reports whether l may explain a missing route: it is warming,
// draining or rejected, has a version warming or draining beside the active
// one, or Envoy rejected its last update. A listener without a status (built
// by hand, or from older JSON) is treated as active.
func explains(l Listener) bool {
	switch l.State.Status {
	case ListenerWarming, ListenerDraining, ListenerRejected:
		return true
	}
	return l.State.Updating || l.State.Draining || l.State.Error != nil
}

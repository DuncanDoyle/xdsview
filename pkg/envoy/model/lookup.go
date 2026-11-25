package model

import (
	"strconv"
	"strings"
)

// CompareID orders IDs segment by segment, comparing a segment's trailing
// index numerically, so "L2" sorts before "L10" and a block before what lies
// inside it. It returns -1, 0 or +1.
func CompareID(a, b ID) int {
	as, bs := strings.Split(string(a), "/"), strings.Split(string(b), "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := compareSegment(as[i], bs[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// compareSegment compares the letter prefix, then the index: a segment
// without an index ("RCI") sorts before one with an index.
func compareSegment(a, b string) int {
	ap, an := splitSegment(a)
	bp, bn := splitSegment(b)
	if c := strings.Compare(ap, bp); c != 0 {
		return c
	}
	switch {
	case an < bn:
		return -1
	case an > bn:
		return 1
	}
	return strings.Compare(a, b)
}

// splitSegment splits "RD12" into "RD" and 12; the index is -1 when absent.
func splitSegment(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return s, -1
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, -1
	}
	return s[:i], n
}

// Listener returns the listener with id.
func (s *Snapshot) Listener(id ID) (Listener, bool) {
	for _, l := range s.Listeners {
		if l.ID == id {
			return l, true
		}
	}
	return Listener{}, false
}

// FilterChain returns the filter chain with id.
func (s *Snapshot) FilterChain(id ID) (NetworkFilterChain, bool) {
	for _, l := range s.Listeners {
		if !id.Within(l.ID) {
			continue
		}
		for _, fc := range l.FilterChains {
			if fc.ID == id {
				return fc, true
			}
		}
	}
	return NetworkFilterChain{}, false
}

// HCM returns the HTTP connection manager whose ID, or whose route config
// use, virtual host, route, slot or weighted backend ID, is id or contains id.
func (s *Snapshot) HCM(id ID) (HCMConfig, bool) {
	for _, l := range s.Listeners {
		if !id.Within(l.ID) {
			continue
		}
		for _, fc := range l.FilterChains {
			if fc.HCM != nil && id.Within(fc.HCM.ID) {
				return *fc.HCM, true
			}
		}
	}
	return HCMConfig{}, false
}

// VirtualHost returns the virtual host with id.
func (s *Snapshot) VirtualHost(id ID) (VirtualHost, bool) {
	hcm, ok := s.HCM(id)
	if !ok || hcm.RouteConfig == nil {
		return VirtualHost{}, false
	}
	for _, vh := range hcm.RouteConfig.VirtualHosts {
		if vh.ID == id {
			return vh, true
		}
	}
	return VirtualHost{}, false
}

// Route returns the route with id.
func (s *Snapshot) Route(id ID) (Route, bool) {
	hcm, ok := s.HCM(id)
	if !ok || hcm.RouteConfig == nil {
		return Route{}, false
	}
	for _, vh := range hcm.RouteConfig.VirtualHosts {
		if !id.Within(vh.ID) {
			continue
		}
		for _, r := range vh.Routes {
			if r.ID == id {
				return r, true
			}
		}
	}
	return Route{}, false
}

// WeightedCluster returns the weighted backend with id.
func (s *Snapshot) WeightedCluster(id ID) (WeightedCluster, bool) {
	hcm, ok := s.HCM(id)
	if !ok || hcm.RouteConfig == nil {
		return WeightedCluster{}, false
	}
	for _, vh := range hcm.RouteConfig.VirtualHosts {
		for _, r := range vh.Routes {
			if !id.Within(r.ID) {
				continue
			}
			for _, wc := range r.WeightedClusters {
				if wc.ID == id {
					return wc, true
				}
			}
		}
	}
	return WeightedCluster{}, false
}

// Slot returns the HTTP filter slot with id: an HCM filter or a router
// upstream filter.
func (s *Snapshot) Slot(id ID) (HTTPFilter, bool) {
	hcm, ok := s.HCM(id)
	if !ok {
		return HTTPFilter{}, false
	}
	for _, list := range [][]HTTPFilter{hcm.HTTPFilters, hcm.UpstreamFilters} {
		for _, f := range list {
			if f.ID == id {
				return f, true
			}
		}
	}
	return HTTPFilter{}, false
}

// RouteConfigSource returns the route config source with id.
func (s *Snapshot) RouteConfigSource(id ID) (RouteConfigSource, bool) {
	for _, rc := range s.RouteConfigs {
		if rc.ID == id {
			return rc, true
		}
	}
	return RouteConfigSource{}, false
}

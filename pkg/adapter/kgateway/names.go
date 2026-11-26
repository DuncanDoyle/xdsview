package kgateway

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

const (
	gatewayGroup   = "gateway.networking.k8s.io"
	kgatewayGroup  = "gateway.kgateway.dev"
	routeSourceKey = "dev.kgateway.route_source"
	mergeKeyPrefix = "merge."
)

// CheckedMinors lists the kgateway minors whose name formats and metadata
// shapes this module was checked against. A consumer that supports other
// minors checks its minor against this list.
func CheckedMinors() []string { return []string{"v2.4", "v2.5"} }

// Names returns the [provenance.NameMapper] for kgateway names:
//   - a cluster "kube_<ns>_<name>_<port>" is a Service, inferred;
//   - a route name reads as one or more HTTPRoutes (see [New]);
//   - route config and listener names carry a port or listener name but no
//     object, so they map to nothing.
func Names() provenance.NameMapper { return names{} }

type names struct{}

func (names) Name(kind provenance.NameKind, name string) provenance.NameAnswer {
	switch kind {
	case provenance.NameCluster:
		if b, _, ok := clusterBackend(name); ok {
			return provenance.NameAnswer{Certainty: model.Inferred, Candidates: []model.Ref{b}}
		}
	case provenance.NameRoute:
		var refs []model.Ref
		for _, r := range parseAnyRouteName(name) {
			if !slices.Contains(refs, r.Route) {
				refs = append(refs, r.Route)
			}
		}
		switch len(refs) {
		case 0:
		case 1:
			return provenance.NameAnswer{Certainty: model.Inferred, Candidates: refs}
		default:
			return provenance.NameAnswer{Certainty: model.Ambiguous, Candidates: refs}
		}
	}
	return provenance.NameAnswer{}
}

// clusterBackend reads "kube_<ns>_<name>_<port>" (kgateway's Service cluster
// name, pluginsdk/ir/backend.go buildClusterName with prefix "kube").
func clusterBackend(name string) (model.Ref, int, bool) {
	parts := strings.Split(name, "_")
	if len(parts) != 4 || parts[0] != "kube" {
		return model.Ref{}, 0, false
	}
	port, ok := index(parts[3])
	if !ok || !validDNSLabel(parts[1]) || !validDNSLabel(parts[2]) {
		return model.Ref{}, 0, false
	}
	return model.Ref{Kind: "Service", Namespace: parts[1], Name: parts[2]}, port, true
}

// listenerFromName reads "listener~<port>" (a port) or a bare listener
// (section) name, which is a DNS label; that rules out cluster names such as
// "xds_cluster". ListenerSet names ("<ns>/<name>/<section>") aren't read.
func listenerFromName(name string) (provenance.ListenerSource, bool) {
	if port, ok := strings.CutPrefix(name, "listener~"); ok {
		if p, ok := index(port); ok {
			return provenance.ListenerSource{Port: p}, true
		}
		return provenance.ListenerSource{}, false
	}
	if !validDNSLabel(name) {
		return provenance.ListenerSource{}, false
	}
	return provenance.ListenerSource{Section: name}, true
}

// routeRest captures the HTTPRoute part and final match index of a route name
// after its "<vhost>-route-<N>-" prefix (kgateway irtranslator/route.go):
// httproute-<name>-<ns>-<rule>-<match>[-<ruleName>]-matcher-<match>.
var routeRest = regexp.MustCompile(`^httproute-(.+)-matcher-(\d+)$`)

// parseRouteName returns every reading of a route name under virtual host
// vhost, parsed after the "<vhost>-route-<N>-" prefix. anchored is false when
// the name lacks that prefix; the readings then come from [parseAnyRouteName].
func parseRouteName(vhost, name string) (readings []provenance.RouteSource, anchored bool) {
	if rest, ok := cutRoutePrefix(vhost, name); ok {
		return parseRouteRest(rest), true
	}
	return parseAnyRouteName(name), false
}

// parseAnyRouteName reads a name without its virtual host: every
// "-route-<N>-" in it may end the prefix, so it returns the readings of each.
func parseAnyRouteName(name string) []provenance.RouteSource {
	var out []provenance.RouteSource
	for i := 0; ; i++ {
		j := strings.Index(name[i:], "-route-")
		if j < 0 {
			return out
		}
		i += j
		if rest, ok := cutRoutePrefix(name[:i], name); ok {
			for _, r := range parseRouteRest(rest) {
				if !slices.Contains(out, r) {
					out = append(out, r)
				}
			}
		}
	}
}

// parseRouteRest returns every reading of a route name's HTTPRoute part.
// Names, namespaces and rule names may all contain "-", so a name can have
// several readings. A rule name is a Gateway API SectionName, which may
// contain dots (Phase 8 #2).
func parseRouteRest(rest string) []provenance.RouteSource {
	m := routeRest.FindStringSubmatch(rest)
	if m == nil {
		return nil
	}
	match, _ := strconv.Atoi(m[2])
	tokens := strings.Split(m[1], "-")
	var out []provenance.RouteSource
	// <name>-<ns> needs at least two tokens before "<rule>-<match>".
	for i := 2; i+1 < len(tokens); i++ {
		rule, ok1 := index(tokens[i])
		mi, ok2 := index(tokens[i+1])
		if !ok1 || !ok2 || mi != match {
			continue
		}
		ruleName := strings.Join(tokens[i+2:], "-")
		if ruleName != "" && !validSectionName(ruleName) {
			continue
		}
		for j := 1; j < i; j++ {
			n, ns := strings.Join(tokens[:j], "-"), strings.Join(tokens[j:i], "-")
			if !validDNSSubdomain(n) || !validDNSLabel(ns) {
				continue
			}
			out = append(out, provenance.RouteSource{
				Route:     model.Ref{Group: gatewayGroup, Kind: "HTTPRoute", Namespace: ns, Name: n},
				RuleIndex: rule, RuleName: ruleName, MatchIndex: match,
			})
		}
	}
	return out
}

var (
	dnsLabel     = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	dnsSubdomain = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

func validDNSLabel(s string) bool     { return len(s) <= 63 && dnsLabel.MatchString(s) }
func validDNSSubdomain(s string) bool { return len(s) <= 253 && dnsSubdomain.MatchString(s) }

// validSectionName checks a Gateway API SectionName: a DNS subdomain of at
// most 253 characters (apis/v1/shared_types.go).
func validSectionName(s string) bool { return validDNSSubdomain(s) }

// validGroupKind reports whether group and kind are valid: the group empty (core)
// or a DNS subdomain, and the kind a DNS-1035 label once lowercased.
func validGroupKind(group, kind string) bool {
	k := strings.ToLower(kind)
	return (group == "" || validDNSSubdomain(group)) && validDNSLabel(k) && k[0] >= 'a' && k[0] <= 'z'
}

// index parses an unsigned decimal index. strconv.Atoi alone accepts a sign.
func index(s string) (int, bool) {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

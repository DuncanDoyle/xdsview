package kgateway

import (
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// routeFromMetadata reads dev.kgateway.route_source. Its shape differs by
// version:
//   - v2.5: rule is "_rule-<idx>" or the rule name, and match is "_match-<idx>".
//   - v2.4: rule is "httproute-<name>-<ns>-<rule>-<match>", and there is no match.
//
// Indices the metadata lacks come from the route name, read with the
// metadata's name and namespace, so that read is unambiguous; fromName says so.
// vhost is the name of the route's virtual host.
func routeFromMetadata(vhost string, r model.Route) (o provenance.RouteSource, fromName, ok bool) {
	md, ok := r.Metadata[routeSourceKey].(map[string]any)
	if !ok {
		return o, false, false
	}
	str := func(k string) string { s, _ := md[k].(string); return s }
	ref := model.Ref{Group: str("group"), Kind: str("kind"), Namespace: str("namespace"), Name: str("name")}
	if ref.Kind == "" || ref.Name == "" {
		return o, false, false
	}
	o = provenance.RouteSource{Route: ref, RuleIndex: -1, MatchIndex: -1}
	rule, match := str("rule"), str("match")
	legacy := strings.ToLower(ref.Kind) + "-" + ref.Name + "-" + ref.Namespace + "-"
	unnamed := strings.HasPrefix(rule, "_rule-") // v2.5 writes this only for an unnamed rule
	if idx, ok := strings.CutPrefix(rule, "_rule-"); ok {
		if ri, ok := index(idx); ok {
			o.RuleIndex = ri
		}
	} else if rest, ok := strings.CutPrefix(rule, legacy); ok && match == "" {
		// v2.4: "<kind>-<name>-<ns>-<rule>-<match>[-<rule name>]" (query/route.go),
		// with no "match"; v2.5 always writes "match".
		if p := strings.SplitN(rest, "-", 3); len(p) >= 2 {
			if ri, ok := index(p[0]); ok {
				o.RuleIndex = ri
			}
			if mi, ok := index(p[1]); ok {
				o.MatchIndex = mi
			}
			if len(p) == 3 {
				o.RuleName = p[2]
			} else {
				unnamed = true
			}
		}
	} else {
		o.RuleName = rule
	}
	if idx, ok := strings.CutPrefix(match, "_match-"); ok {
		if mi, ok := index(idx); ok {
			o.MatchIndex = mi
		}
	}
	// Fill the rest from the name, restricted to this route's identity.
	readings, _ := parseRouteName(vhost, r.Name)
	for _, c := range readings {
		if c.Route != ref {
			continue
		}
		if conflicts(o, c) || (unnamed && c.RuleName != "") {
			break
		}
		if o.RuleIndex < 0 && c.RuleIndex >= 0 {
			o.RuleIndex, fromName = c.RuleIndex, true
		}
		if o.MatchIndex < 0 && c.MatchIndex >= 0 {
			o.MatchIndex, fromName = c.MatchIndex, true
		}
		if o.RuleName == "" && c.RuleName != "" {
			o.RuleName, fromName = c.RuleName, true
		}
		break
	}
	if fromName {
		o.IndexCertainty = model.Inferred
	}
	return o, fromName, true
}

// conflicts reports whether a name reading c disagrees with a field that the
// metadata set in o. Such a name is not evidence for the missing fields.
func conflicts(o, c provenance.RouteSource) bool {
	return (o.RuleIndex >= 0 && c.RuleIndex >= 0 && o.RuleIndex != c.RuleIndex) ||
		(o.MatchIndex >= 0 && c.MatchIndex >= 0 && o.MatchIndex != c.MatchIndex) ||
		(o.RuleName != "" && o.RuleName != c.RuleName) // an unnamed reading conflicts too
}

// routePart strips "<vhost>-route-<n>-" from a kgateway route name, so a
// marker inside the vhost name can't match. A name in another format is
// returned whole.
func routePart(vhost, name string) string {
	if rest, ok := cutRoutePrefix(vhost, name); ok {
		return rest
	}
	return name
}

// cutRoutePrefix returns name without its "<vhost>-route-<n>-" prefix, and
// whether it had one.
func cutRoutePrefix(vhost, name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, vhost+"-route-")
	if !ok {
		return "", false
	}
	n, after, ok := strings.Cut(rest, "-")
	if _, digits := index(n); !ok || !digits {
		return "", false
	}
	return after, true
}

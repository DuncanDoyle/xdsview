// Package filter narrows an [model.EnvoySnapshot] to the routes that belong
// to a specific Kubernetes HTTPRoute.
//
// Kgateway encodes the HTTPRoute identity in each Envoy route name using the
// convention:
//
//	<vhost>-route-<N>-httproute-<name>-<namespace>-<rule>-<match>[-<ruleName>]-matcher-<match>
//
// <N> is the route's position in the virtual host after precedence sorting, not
// the rule index.
//
// A route annotated by origin.Annotate is selected by its origin, and never
// when that origin is Ambiguous; [Ambiguous] lists those routes. Otherwise a
// route with kgateway's route_source metadata is selected by that metadata,
// and others by substring matching on that segment, without Kubernetes API
// calls. Listeners and virtual hosts that contain no matching routes are
// pruned from the returned snapshot, except listeners whose state may explain
// a missing route.
//
// # Known limitation
//
// Both the HTTPRoute name and its namespace can contain dashes. If a name and
// namespace pair produces the same concatenated string as another pair (e.g.
// HTTPRoute "foo-bar" in namespace "baz" versus "foo" in "bar-baz"), name
// matching selects both. Annotate the snapshot first, with the HTTPRoutes that
// exist as origin.KgatewayOptions.Known, to tell them apart.
//
// # Ownership
//
// [Filter] returns a new snapshot that shares nested slices, maps and pointers
// with its input. Treat both as read-only.
package filter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// Options controls which routes [Filter] retains from a snapshot.
//
// HTTPRouteName and HTTPRouteNamespace together identify a single HTTPRoute
// resource. They correspond to the metadata.name and metadata.namespace fields
// of the HTTPRoute and may differ from the Gateway name and namespace used for
// port-forwarding.
//
// RuleIndex narrows the selection to one rule within the HTTPRoute. Use
// [AllRules] to include routes from all rules; the zero value selects rule 0.
// RuleName selects a named rule instead; set RuleIndex to AllRules with it.
type Options struct {
	HTTPRouteName      string // HTTPRoute resource name, e.g. "api-example-com"
	HTTPRouteNamespace string // HTTPRoute resource namespace, e.g. "default"
	RuleIndex          int    // zero-based rule index within the HTTPRoute, or AllRules
	RuleName           string // rule name (spec.rules[].name), or empty
}

// AllRules is the [Options.RuleIndex] value that keeps routes from every rule.
const AllRules = -1

// Filter returns a new [model.EnvoySnapshot] that contains only the routes
// whose Envoy route name embeds the HTTPRoute identity described by opts.
//
// Pruning behaviour:
//   - Virtual hosts with no matching routes are removed.
//   - Filter chains with no matching virtual hosts keep their HCM (preserving
//     the HTTP filter pipeline for display) but have an empty VirtualHosts slice.
//   - Listeners where no filter chain retains any virtual hosts are removed.
//
// If opts.HTTPRouteName is empty, snapshot is returned unchanged.
func Filter(snapshot *model.EnvoySnapshot, opts Options) *model.EnvoySnapshot {
	if opts.HTTPRouteName == "" {
		return snapshot
	}

	result := &model.EnvoySnapshot{}
	for _, l := range snapshot.Listeners {
		if filtered := filterListener(l, opts); filtered != nil {
			result.Listeners = append(result.Listeners, *filtered)
		}
	}
	return result
}

// filterListener returns a copy of l containing only the virtual hosts that
// match opts, or nil if no filter chain in the listener has any matches.
func filterListener(l model.Listener, opts Options) *model.Listener {
	out := l
	out.FilterChains = nil
	for _, fc := range l.FilterChains {
		out.FilterChains = append(out.FilterChains, filterChain(fc, opts))
	}
	// Keep the listener when a filter chain has matching VHs, or when its state
	// may explain why a route is missing.
	for _, fc := range out.FilterChains {
		if fc.HCM != nil && fc.HCM.RouteConfig != nil && len(fc.HCM.RouteConfig.VirtualHosts) > 0 {
			return &out
		}
	}
	if explains(l) {
		return &out
	}
	return nil
}

// filterChain returns a copy of fc whose RouteConfig contains only the virtual
// hosts that have matching routes. When no VHs match, the HCM is preserved
// (with an empty VirtualHosts slice) so callers can still display the HTTP
// filter pipeline.
func filterChain(fc model.NetworkFilterChain, opts Options) model.NetworkFilterChain {
	out := fc
	if fc.HCM == nil || fc.HCM.RouteConfig == nil {
		return out
	}
	hcmCopy := *fc.HCM
	rcCopy := *fc.HCM.RouteConfig
	rcCopy.VirtualHosts = nil
	for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
		if filtered := filterVirtualHost(vh, opts); filtered != nil {
			rcCopy.VirtualHosts = append(rcCopy.VirtualHosts, *filtered)
		}
	}
	hcmCopy.RouteConfig = &rcCopy
	out.HCM = &hcmCopy
	return out
}

// filterVirtualHost returns a copy of vh containing only the routes that match
// opts, or nil if no routes match.
func filterVirtualHost(vh model.VirtualHost, opts Options) *model.VirtualHost {
	out := vh
	out.Routes = nil
	for _, r := range vh.Routes {
		if selected(r, opts) {
			out.Routes = append(out.Routes, r)
		}
	}
	if len(out.Routes) == 0 {
		return nil
	}
	return &out
}

// metadataMapper reads kgateway's route_source metadata.
var metadataMapper = origin.Kgateway(origin.KgatewayOptions{})

// selected reports whether r belongs to the HTTPRoute (and rule) in opts. An
// annotated route is judged by its origin, and never selected when that is
// Ambiguous. Otherwise a route with route_source metadata is judged by it, and
// others by their name.
func selected(r model.Route, opts Options) bool {
	if a := r.Origin; a != nil {
		if a.Certainty == model.Ambiguous || len(a.Candidates) != 1 {
			return false
		}
		return candidateMatches(a.Candidates[0], opts)
	}
	if _, ok := r.Metadata["dev.kgateway.route_source"]; ok {
		if a := metadataMapper.Route(r); a.Certainty == model.Exact {
			return candidateMatches(a.Candidates[0], opts)
		}
	}
	return routeMatches(r.Name, opts)
}

// routeMatches reports whether the Envoy route name encodes the HTTPRoute
// identity from opts, using substring matching on the kgateway naming
// convention:
//
//	...-httproute-<name>-<namespace>-<rule_idx>-<match_idx>[-<rule_name>]-matcher-<match_idx>
//
// When opts.RuleIndex >= 0 the match is further narrowed to that rule index.
func routeMatches(name string, opts Options) bool {
	// Base marker: identifies the HTTPRoute by name and namespace.
	// Example: "httproute-api-example-com-default-"
	base := fmt.Sprintf("httproute-%s-%s-", opts.HTTPRouteName, opts.HTTPRouteNamespace)
	if !strings.Contains(name, base) {
		return false
	}
	// Optional rule filter: narrows to a specific zero-based rule index.
	// Example: "httproute-api-example-com-default-1-"
	if opts.RuleIndex >= 0 {
		ruleMarker := fmt.Sprintf("%s%d-", base, opts.RuleIndex)
		if !strings.Contains(name, ruleMarker) {
			return false
		}
	}
	if opts.RuleName != "" {
		return hasRuleName(name, base, opts.RuleName)
	}
	return true
}

// hasRuleName reports whether name ends with
// "<base><rule>-<match>-<ruleName>-matcher-<match>". Anchoring at the end, with
// the same match index twice, keeps rule "a" from selecting "a-b" or "a-matcher-b".
func hasRuleName(name, base, ruleName string) bool {
	i := strings.LastIndex(name, base)
	if i < 0 {
		return false
	}
	parts := strings.SplitN(name[i+len(base):], "-", 3) // rule, match, rest
	if len(parts) != 3 || !isDigits(parts[0]) || !isDigits(parts[1]) {
		return false
	}
	return parts[2] == ruleName+"-matcher-"+parts[1]
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Ambiguous returns the names of the routes [Filter] leaves out because their
// origin is Ambiguous while one of its candidates is the HTTPRoute (and rule)
// in opts. Each name appears once, in snapshot order. Use it to tell the user
// that a route may be missing from the narrowed view.
func Ambiguous(snapshot *model.EnvoySnapshot, opts Options) []string {
	if opts.HTTPRouteName == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, l := range snapshot.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					if r.Origin == nil || r.Origin.Certainty != model.Ambiguous || seen[r.Name] {
						continue
					}
					if slices.ContainsFunc(r.Origin.Candidates, func(o model.RouteOrigin) bool { return candidateMatches(o, opts) }) {
						seen[r.Name] = true
						out = append(out, r.Name)
					}
				}
			}
		}
	}
	return out
}

// candidateMatches reports whether o is the HTTPRoute, and rule, in opts.
func candidateMatches(o model.RouteOrigin, opts Options) bool {
	return o.Route.Kind == "HTTPRoute" && o.Route.Name == opts.HTTPRouteName &&
		o.Route.Namespace == opts.HTTPRouteNamespace &&
		(opts.RuleIndex < 0 || o.RuleIndex == opts.RuleIndex) &&
		(opts.RuleName == "" || o.RuleName == opts.RuleName)
}

// explains reports whether l may explain a missing route: it is warming,
// draining or rejected, or Envoy rejected its last update. A listener without
// a status (built by hand, or from older JSON) is treated as active.
func explains(l model.Listener) bool {
	switch l.State.Status {
	case model.ListenerWarming, model.ListenerDraining, model.ListenerRejected:
		return true
	}
	return l.State.Error != nil
}

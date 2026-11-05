package origin

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// KgatewayOptions is the context a caller knows and the Envoy names don't carry.
type KgatewayOptions struct {
	// Gateway is the Gateway whose proxy produced the dump. Listener and route
	// config origins name it; without it, route configs map to Unknown.
	Gateway *model.Ref
	// Known lists HTTPRoutes that exist. It resolves route names with several readings.
	Known []model.Ref
}

// Kgateway returns the mapper for kgateway OSS. Its name formats and metadata
// shapes are verified on kgateway v2.4.7 and v2.5.0.
func Kgateway(opts KgatewayOptions) Mapper { return kgateway{opts} }

type kgateway struct{ opts KgatewayOptions }

const (
	gatewayGroup   = "gateway.networking.k8s.io"
	kgatewayGroup  = "gateway.kgateway.dev"
	routeSourceKey = "dev.kgateway.route_source"
	mergeKeyPrefix = "merge."
)

func (k kgateway) gatewayListener(port int, section string) model.ListenerOrigin {
	return model.ListenerOrigin{Certainty: model.Inferred, Gateway: k.opts.Gateway, Port: port, Section: section}
}

// listenerFromChain reads "listener~<port>" (a port) or a bare section name.
func (k kgateway) listenerFromChain(name string) (model.ListenerOrigin, bool) {
	if port, ok := strings.CutPrefix(name, "listener~"); ok {
		if p, err := strconv.Atoi(port); err == nil {
			return k.gatewayListener(p, ""), true
		}
		return model.ListenerOrigin{}, false
	}
	// A section name is a DNS label, which rules out cluster names such as
	// "xds_cluster". ListenerSet chains ("<ns>/<name>/<section>") aren't mapped yet.
	if !validDNSLabel(name) {
		return model.ListenerOrigin{}, false
	}
	return k.gatewayListener(0, name), true
}

func (k kgateway) Listener(l model.Listener) model.ListenerOrigin {
	if port, ok := strings.CutPrefix(l.Name, "listener~"); ok {
		if p, err := strconv.Atoi(port); err == nil {
			return k.gatewayListener(p, "")
		}
	}
	return model.ListenerOrigin{}
}

func (k kgateway) RouteConfig(name string) model.ListenerOrigin {
	if k.opts.Gateway == nil {
		return model.ListenerOrigin{}
	}
	o, _ := k.listenerFromChain(name)
	return o
}

// VirtualHost reads "<chain>~<hostname>", where the hostname's dots became "_".
func (k kgateway) VirtualHost(vh model.VirtualHost) model.VirtualHostOrigin {
	i := strings.LastIndex(vh.Name, "~")
	if i < 0 {
		return model.VirtualHostOrigin{}
	}
	l, ok := k.listenerFromChain(vh.Name[:i])
	if !ok {
		return model.VirtualHostOrigin{}
	}
	// Gateway API hostnames have no "_", so every "_" was a ".".
	return model.VirtualHostOrigin{Certainty: model.Inferred, Listener: l, Hostname: strings.ReplaceAll(vh.Name[i+1:], "_", ".")}
}

// routeName captures the HTTPRoute part of a route name and its final match index:
// <vhost>-route-<N>-httproute-<name>-<ns>-<rule>-<match>[-<ruleName>]-matcher-<match>.
var routeName = regexp.MustCompile(`-route-\d+-httproute-(.+)-matcher-(\d+)$`)

func (k kgateway) Route(r model.Route) model.RouteAnnotation {
	if o, ok := routeFromMetadata(r); ok {
		return model.RouteAnnotation{Certainty: model.Exact, Candidates: []model.RouteOrigin{o}}
	}
	cands := parseRouteName(r.Name)
	if len(k.opts.Known) > 0 && len(cands) > 1 {
		var known []model.RouteOrigin
		for _, c := range cands {
			if slices.Contains(k.opts.Known, c.Route) {
				known = append(known, c)
			}
		}
		if len(known) > 0 {
			cands = known
		}
	}
	switch len(cands) {
	case 0:
		return model.RouteAnnotation{}
	case 1:
		return model.RouteAnnotation{Certainty: model.Inferred, Candidates: cands}
	default:
		return model.RouteAnnotation{Certainty: model.Ambiguous, Candidates: cands}
	}
}

// parseRouteName returns every reading of a route name's HTTPRoute part.
// Names, namespaces and rule names may all contain "-", so a name can have
// several readings.
func parseRouteName(name string) []model.RouteOrigin {
	m := routeName.FindStringSubmatch(name)
	if m == nil {
		return nil
	}
	match, _ := strconv.Atoi(m[2])
	tokens := strings.Split(m[1], "-")
	var out []model.RouteOrigin
	// <name>-<ns> needs at least two tokens before "<rule>-<match>".
	for i := 2; i+1 < len(tokens); i++ {
		rule, err1 := strconv.Atoi(tokens[i])
		mi, err2 := strconv.Atoi(tokens[i+1])
		if err1 != nil || err2 != nil || mi != match {
			continue
		}
		ruleName := strings.Join(tokens[i+2:], "-")
		if ruleName != "" && !validDNSLabel(ruleName) {
			continue
		}
		for j := 1; j < i; j++ {
			n, ns := strings.Join(tokens[:j], "-"), strings.Join(tokens[j:i], "-")
			if !validDNSSubdomain(n) || !validDNSLabel(ns) {
				continue
			}
			out = append(out, model.RouteOrigin{
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

// routeFromMetadata reads dev.kgateway.route_source. Its shape differs by
// version:
//   - v2.5: rule is "_rule-<idx>" or the rule name, and match is "_match-<idx>".
//   - v2.4: rule is "httproute-<name>-<ns>-<rule>-<match>", and there is no match.
//
// Indices the metadata lacks come from the route name, read with the metadata's
// name and namespace, so that read is unambiguous.
func routeFromMetadata(r model.Route) (model.RouteOrigin, bool) {
	md, ok := r.Metadata[routeSourceKey].(map[string]any)
	if !ok {
		return model.RouteOrigin{}, false
	}
	str := func(k string) string { s, _ := md[k].(string); return s }
	ref := model.Ref{Group: str("group"), Kind: str("kind"), Namespace: str("namespace"), Name: str("name")}
	if ref.Kind == "" || ref.Name == "" {
		return model.RouteOrigin{}, false
	}
	o := model.RouteOrigin{Route: ref, RuleIndex: -1, MatchIndex: -1}
	rule, match := str("rule"), str("match")
	legacy := strings.ToLower(ref.Kind) + "-" + ref.Name + "-" + ref.Namespace + "-"
	if idx, ok := strings.CutPrefix(rule, "_rule-"); ok {
		o.RuleIndex, _ = strconv.Atoi(idx)
	} else if rest, ok := strings.CutPrefix(rule, legacy); ok {
		// v2.4: "<kind>-<name>-<ns>-<rule>-<match>".
		if r, m, ok := strings.Cut(rest, "-"); ok {
			if ri, err := strconv.Atoi(r); err == nil {
				o.RuleIndex = ri
			}
			if mi, err := strconv.Atoi(m); err == nil {
				o.MatchIndex = mi
			}
		}
	} else {
		o.RuleName = rule
	}
	if idx, ok := strings.CutPrefix(match, "_match-"); ok {
		o.MatchIndex, _ = strconv.Atoi(idx)
	}
	// Fill the rest from the name, restricted to this route's identity.
	for _, c := range parseRouteName(r.Name) {
		if c.Route.Name == ref.Name && c.Route.Namespace == ref.Namespace {
			if o.RuleIndex < 0 {
				o.RuleIndex = c.RuleIndex
			}
			if o.MatchIndex < 0 {
				o.MatchIndex = c.MatchIndex
			}
			if o.RuleName == "" {
				o.RuleName = c.RuleName
			}
			break
		}
	}
	return o, true
}

func (k kgateway) Cluster(name string) model.BackendOrigin {
	parts := strings.Split(name, "_")
	if len(parts) != 4 || parts[0] != "kube" {
		return model.BackendOrigin{}
	}
	port, err := strconv.Atoi(parts[3])
	if err != nil || !validDNSLabel(parts[1]) || !validDNSLabel(parts[2]) {
		return model.BackendOrigin{}
	}
	return model.BackendOrigin{Certainty: model.Inferred, Backend: model.Ref{Kind: "Service", Namespace: parts[1], Name: parts[2]}, Port: port}
}

// extensionFilters are the filter-name prefixes kgateway gives one filter per
// GatewayExtension, as "<prefix>/<ns>/<name>".
var extensionFilters = []string{"ext_auth", "ext_proc", "ratelimit", "jwt"}

func (k kgateway) HTTPFilter(f model.HTTPFilter) model.FilterOrigin {
	o := model.FilterOrigin{Kind: filterKind(f.Type)}
	parts := strings.Split(f.Name, "/")
	switch {
	case len(parts) == 2 && parts[0] == "global_disable":
		o.Helper = true
	case len(parts) == 3 && slices.Contains(extensionFilters, parts[0]):
		o.Extension = &model.Ref{Group: kgatewayGroup, Kind: "GatewayExtension", Namespace: parts[1], Name: parts[2]}
	}
	if o.Kind != "" || o.Helper || o.Extension != nil {
		o.Certainty = model.Inferred
	}
	return o
}

// filterKind returns the Envoy filter kind of an HTTP filter type, e.g. "ext_authz"
// for "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz".
func filterKind(typ string) string {
	_, rest, ok := strings.Cut(typ, ".filters.http.")
	if !ok {
		return ""
	}
	kind, _, _ := strings.Cut(rest, ".")
	return kind
}

// Policies reads every "merge.<Kind>.<group>" route metadata entry, which maps
// a policy field to "<group>/<Kind>/<ns>/<name>" policy IDs. Any Kind is read,
// so a vendor policy Kind appears without a plugin.
func (k kgateway) Policies(r model.Route) []model.PolicyRef {
	var out []model.PolicyRef
	for key, v := range r.Metadata {
		if !strings.HasPrefix(key, mergeKeyPrefix) {
			continue
		}
		fields, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for field, ids := range fields {
			list, _ := ids.([]any)
			for _, id := range list {
				s, _ := id.(string)
				p := strings.Split(s, "/")
				if len(p) != 4 {
					continue
				}
				out = append(out, model.PolicyRef{Policy: model.Ref{Group: p[0], Kind: p[1], Namespace: p[2], Name: p[3]}, Field: field})
			}
		}
	}
	slices.SortFunc(out, func(a, b model.PolicyRef) int {
		return strings.Compare(a.Policy.String()+"/"+a.Field, b.Policy.String()+"/"+b.Field)
	})
	return out
}

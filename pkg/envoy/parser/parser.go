// Package parser converts a raw Envoy /config_dump JSON payload into the
// domain model defined in [model]. The Envoy config_dump is a heterogeneous
// JSON array where each element has a "@type" discriminator; the parser dispatches
// on that field to extract ListenersConfigDump and RoutesConfigDump sections, then
// joins them into a [model.Snapshot].
//
// Non-fatal errors (e.g. a single malformed section) are collected as warnings in
// the best-effort snapshot so the caller can surface them to the user rather than
// silently dropping data.
//
// # Occurrence IDs
//
// Parse assigns every occurrence a [model.ID] as a structural path: L0 (listener),
// L0/C1 (filter chain), L0/C1/N0 (network filter; the HCM), .../F3 (HTTP filter
// slot), .../U0 (upstream filter), .../RC (route config use), .../RC/V2/R5/W1
// (virtual host, route, weighted backend). Route config sources are RD<n>
// (dynamic), RS<n> (static) and <HCM>/RCI (inline). RDS names resolve against
// dynamic sources only.
//
// # Ownership
//
// The returned snapshot belongs to the caller. Treat it as read-only once
// it is shared: [filter.Filter] returns views that share nested slices, maps and
// pointers with it. Store derived data alongside a snapshot, not inside it.
//
// [filter.Filter]: https://pkg.go.dev/github.com/DuncanDoyle/xdsview/pkg/envoy/filter#Filter
package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Options controls [Parse].
type Options struct {
	// Capture says whether the dump is known to be complete. The zero value
	// means [model.CaptureUnverified].
	Capture model.Capture
}

// Parse reads a complete Envoy /config_dump JSON document from r and returns
// its [model.Snapshot]. Trailing data after the document, or a read error,
// fails the parse. Non-fatal errors (e.g. a single malformed config section)
// are collected in [model.Snapshot.Warnings] rather than aborting the parse.
func Parse(r io.Reader, opts Options) (*model.Snapshot, error) {
	// ReadAll + Unmarshal (not a streaming Decoder) so trailing data is rejected.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading config dump: %w", err)
	}
	var dump configDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("parsing config dump JSON: %w", err)
	}
	capture := opts.Capture
	if capture == "" {
		capture = model.CaptureUnverified
	}

	// Parse each config section by @type, collecting warnings for any that fail.
	p := &parse{dynamic: map[string][]*routeEntry{}}
	var listeners []rawListener
	var statics []rawStaticListener
	listenersAt := -1 // section index of the listeners, where inline sources go

	for pos, raw := range dump.Configs {
		var typed typedConfig
		if err := json.Unmarshal(raw, &typed); err != nil {
			p.warn("", model.WarnSectionSkipped, "skipped config section: cannot read @type: %v", err)
			continue
		}

		switch typed.Type {
		case "type.googleapis.com/envoy.admin.v3.ListenersConfigDump":
			var ld listenersConfigDump
			if err := json.Unmarshal(raw, &ld); err != nil {
				p.warn("", model.WarnSectionSkipped, "skipped ListenersConfigDump: %v", err)
				continue
			}
			listeners, statics, listenersAt = nil, nil, pos
			if len(ld.DynamicListeners) > 0 {
				if err := json.Unmarshal(ld.DynamicListeners, &listeners); err != nil {
					listeners = nil
					p.warn("", model.WarnSectionSkipped, "skipped ListenersConfigDump: %v", err)
				}
			}
			if len(ld.StaticListeners) > 0 {
				if err := json.Unmarshal(ld.StaticListeners, &statics); err != nil {
					statics = nil
					p.warn("", model.WarnSectionSkipped, "skipped ListenersConfigDump static_listeners: %v", err)
				}
			}

		case "type.googleapis.com/envoy.admin.v3.RoutesConfigDump":
			var rd routesConfigDump
			if err := json.Unmarshal(raw, &rd); err != nil {
				p.warn("", model.WarnSectionSkipped, "skipped RoutesConfigDump: %v", err)
				continue
			}
			p.routeSection(pos, rd)
		}
	}

	// Build the Snapshot by joining listeners with route configs.
	snapshot := &model.Snapshot{Capture: capture}
	for i, rl := range listeners {
		snapshot.Listeners = append(snapshot.Listeners, p.listener(i, rl))
	}
	// Static listeners follow the dynamic ones, so dynamic listener IDs don't
	// depend on the bootstrap.
	for i, sl := range statics {
		snapshot.Listeners = append(snapshot.Listeners, p.staticListener(len(listeners)+i, sl))
	}
	if listenersAt >= 0 {
		p.batches = append(p.batches, batch{pos: listenersAt, entries: p.inline})
	}
	slices.SortStableFunc(p.batches, func(a, b batch) int { return a.pos - b.pos })
	for _, b := range p.batches {
		for _, e := range b.entries {
			if !e.matched {
				snapshot.RouteConfigs = append(snapshot.RouteConfigs, e.source)
			}
		}
	}
	snapshot.Warnings = p.warnings
	return snapshot, nil
}

// parse holds the state of one [Parse] call.
type parse struct {
	dynamic  map[string][]*routeEntry // dynamic sources by name, the only ones RDS resolves to
	static   []*routeEntry
	inline   []*routeEntry
	batches  []batch // sources per dump section, for dump order
	nDynamic int
	nStatic  int
	warnings []model.Warning
}

// routeEntry is one route config source with its parsed and raw content.
// A static entry is matched when it is an inline config's Reported entry.
type routeEntry struct {
	source  model.RouteConfigSource
	config  *model.RouteConfig
	raw     json.RawMessage
	matched bool
}

// batch is the sources of one dump section.
type batch struct {
	pos     int
	entries []*routeEntry
}

func (p *parse) warn(subject model.ID, code, format string, args ...any) {
	p.warnings = append(p.warnings, model.Warning{Subject: subject, Code: code, Message: fmt.Sprintf(format, args...)})
}

// routeSection adds the static and dynamic entries of one RoutesConfigDump
// section. Each field and entry is decoded on its own, so a bad one costs only
// itself.
func (p *parse) routeSection(pos int, rd routesConfigDump) {
	b := batch{pos: pos}
	for _, raw := range p.routeEntries("static_route_configs", rd.StaticRouteConfigs) {
		id := model.ID(fmt.Sprintf("RS%d", p.nStatic))
		p.nStatic++
		if e := p.routeEntry(id, model.SourceStatic, raw); e != nil {
			p.static = append(p.static, e)
			b.entries = append(b.entries, e)
		}
	}
	for _, raw := range p.routeEntries("dynamic_route_configs", rd.DynamicRouteConfigs) {
		id := model.ID(fmt.Sprintf("RD%d", p.nDynamic))
		p.nDynamic++
		if e := p.routeEntry(id, model.SourceDynamic, raw); e != nil {
			p.dynamic[e.source.Name] = append(p.dynamic[e.source.Name], e)
			b.entries = append(b.entries, e)
		}
	}
	p.batches = append(p.batches, b)
}

// routeEntries splits a RoutesConfigDump field into its entries, or warns and
// returns none.
func (p *parse) routeEntries(field string, raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		p.warn("", model.WarnSectionSkipped, "skipped RoutesConfigDump %s: %v", field, err)
		return nil
	}
	return entries
}

// routeEntry decodes a static or dynamic route config entry, or warns and
// returns nil.
func (p *parse) routeEntry(id model.ID, kind model.RouteConfigSourceKind, raw json.RawMessage) *routeEntry {
	var entry struct {
		RouteConfig rawRouteConfig `json:"route_config"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		var named struct {
			RouteConfig struct {
				Name string `json:"name"`
			} `json:"route_config"`
		}
		_ = json.Unmarshal(raw, &named) // best effort, for the warning
		p.warn("", model.WarnRouteConfigSkipped, "skipped route config %q: %v", named.RouteConfig.Name, err)
		return nil
	}
	// A null or nameless entry would join any HCM without an RDS name.
	if entry.RouteConfig.Name == "" {
		p.warn("", model.WarnRouteConfigSkipped, "skipped a route config with no name")
		return nil
	}
	var content struct {
		RouteConfig json.RawMessage `json:"route_config"`
	}
	_ = json.Unmarshal(raw, &content) // decoded above, so this succeeds
	return &routeEntry{
		source: model.RouteConfigSource{ID: id, Kind: kind, Name: entry.RouteConfig.Name},
		config: parseRouteConfig(entry.RouteConfig),
		raw:    content.RouteConfig,
	}
}

// listener converts raw dynamic listener i into the model.Listener, joining
// each HCM to its route config. It shows the active version, else the warming
// one, else the draining one, and warns for each HCM it skips.
func (p *parse) listener(i int, rl rawListener) model.Listener {
	l := model.Listener{
		ID:   model.ID(fmt.Sprintf("L%d", i)),
		Name: rl.Name,
	}
	if rl.ErrorState != nil {
		l.State.Error = &model.ListenerError{Details: rl.ErrorState.Details, LastAttempt: rl.ErrorState.LastUpdateAttempt}
	}

	shown := rl.ActiveState
	switch {
	case rl.ActiveState != nil:
		l.State.Status = model.ListenerActive
		l.State.Updating = rl.WarmingState != nil
		l.State.Draining = rl.DrainingState != nil
	case rl.WarmingState != nil:
		l.State.Status = model.ListenerWarming
		shown = rl.WarmingState
	case rl.DrainingState != nil:
		l.State.Status = model.ListenerDraining
		shown = rl.DrainingState
	default:
		l.State.Status = model.ListenerRejected
		return l
	}
	l.State.LastUpdated = shown.LastUpdated
	p.listenerBody(&l, shown.Listener)
	return l
}

// staticListener converts static listener entry i. Envoy serves a static
// listener from startup, so it is active.
func (p *parse) staticListener(i int, sl rawStaticListener) model.Listener {
	l := model.Listener{
		ID:     model.ID(fmt.Sprintf("L%d", i)),
		Name:   sl.Listener.Name,
		Static: true,
		State:  model.ListenerState{Status: model.ListenerActive, LastUpdated: sl.LastUpdated},
	}
	p.listenerBody(&l, sl.Listener)
	return l
}

// listenerBody fills l's address and filter chains from the listener resource.
func (p *parse) listenerBody(l *model.Listener, body rawListenerBody) {
	if body.Address.SocketAddress.Address != "" {
		l.Address = fmt.Sprintf("%s:%d",
			body.Address.SocketAddress.Address,
			body.Address.SocketAddress.PortValue,
		)
	}

	for i, rfc := range body.FilterChains {
		nfc := model.NetworkFilterChain{
			ID:   model.ID(fmt.Sprintf("%s/C%d", l.ID, i)),
			Name: rfc.Name,
		}

		// TLS context from filter_chain_match.server_names
		if len(rfc.FilterChainMatch.ServerNames) > 0 {
			nfc.TLS = &model.TLSContext{
				SNIHosts: rfc.FilterChainMatch.ServerNames,
			}
		}

		// List every network filter; the HCM is also parsed in full.
		for n, nf := range rfc.Filters {
			var typed typedConfig
			_ = json.Unmarshal(nf.TypedConfig, &typed) // a filter without a readable @type keeps an empty Type
			nfc.NetworkFilters = append(nfc.NetworkFilters, model.NetworkFilter{
				ID: model.ID(fmt.Sprintf("%s/N%d", nfc.ID, n)), Name: nf.Name, Type: typed.Type,
			})
			// Envoy picks the factory by typed_config type; the name is a label.
			if nf.Name != "envoy.filters.network.http_connection_manager" && typed.Type != hcmType {
				continue
			}
			label := fmt.Sprintf("listener %q filter chain %s", l.Name, chainLabel(i, rfc.Name))
			hcm, inline, err := parseHCM(nf.TypedConfig)
			if err != nil {
				p.warn(nfc.ID, model.WarnHCMSkipped, "%s: skipped HTTP connection manager: %v", label, err)
				continue
			}
			hcm.ID = model.ID(fmt.Sprintf("%s/N%d", nfc.ID, n))
			p.join(hcm, inline, label)
			assignIDs(hcm)
			nfc.HCM = hcm
		}

		l.FilterChains = append(l.FilterChains, nfc)
	}
}

// join resolves hcm's route config use: its inline config, else its RDS name
// against the dynamic sources. Each use gets its own copy of the content.
func (p *parse) join(hcm *model.HCMConfig, inline json.RawMessage, label string) {
	use := &hcm.RouteConfigUse
	use.ID = hcm.ID + "/RC"
	use.State = model.RouteConfigUnknown
	switch {
	case len(inline) > 0 && string(inline) != "null":
		var rrc rawRouteConfig
		if err := json.Unmarshal(inline, &rrc); err != nil {
			p.warn(use.ID, model.WarnRouteConfigSkipped, "%s: skipped route config %q: %v", label, rrc.Name, err)
			return
		}
		e := &routeEntry{source: model.RouteConfigSource{ID: hcm.ID + "/RCI", Kind: model.SourceInline, Name: rrc.Name}}
		e.source.Reported = p.reported(rrc.Name, inline)
		p.inline = append(p.inline, e)
		use.Source, use.State = e.source.ID, model.RouteConfigFound
		hcm.RouteConfig = parseRouteConfig(rrc)
	case hcm.RouteConfigName != "":
		switch found := p.dynamic[hcm.RouteConfigName]; len(found) {
		case 0:
			// Missing needs a complete capture; until then it is unknown.
		case 1:
			use.Source, use.State = found[0].source.ID, model.RouteConfigFound
			hcm.RouteConfig = cloneRouteConfig(found[0].config)
		default:
			use.State = model.RouteConfigAmbiguous
			p.warn(use.ID, model.WarnRDSAmbiguous, "%s: route config %q is ambiguous: %d dynamic route configs have that name",
				label, hcm.RouteConfigName, len(found))
		}
	}
}

// reported returns the first unmatched static entry with the inline config's
// name and content, and marks it matched; Envoy lists inline configs there.
func (p *parse) reported(name string, inline json.RawMessage) model.ID {
	for _, e := range p.static {
		if !e.matched && e.source.Name == name && sameConfig(e.raw, inline) {
			e.matched = true
			return e.source.ID
		}
	}
	return ""
}

// sameConfig reports whether two route_config documents are equal, ignoring
// the @type that Envoy adds to static entries.
func sameConfig(a, b json.RawMessage) bool {
	var va, vb map[string]any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	delete(va, "@type")
	delete(vb, "@type")
	return reflect.DeepEqual(va, vb)
}

// assignIDs gives hcm's filter slots and its use's content their IDs.
func assignIDs(hcm *model.HCMConfig) {
	for i := range hcm.HTTPFilters {
		hcm.HTTPFilters[i].ID = model.ID(fmt.Sprintf("%s/F%d", hcm.ID, i))
	}
	for i := range hcm.UpstreamFilters {
		hcm.UpstreamFilters[i].ID = model.ID(fmt.Sprintf("%s/U%d", hcm.ID, i))
	}
	if hcm.RouteConfig == nil {
		return
	}
	for v := range hcm.RouteConfig.VirtualHosts {
		vh := &hcm.RouteConfig.VirtualHosts[v]
		vh.ID = model.ID(fmt.Sprintf("%s/V%d", hcm.RouteConfigUse.ID, v))
		for r := range vh.Routes {
			route := &vh.Routes[r]
			route.ID = model.ID(fmt.Sprintf("%s/R%d", vh.ID, r))
			for w := range route.WeightedClusters {
				route.WeightedClusters[w].ID = model.ID(fmt.Sprintf("%s/W%d", route.ID, w))
			}
		}
	}
}

// chainLabel names filter chain i for a warning: by name, or by index if unnamed.
func chainLabel(i int, name string) string {
	if name == "" {
		return fmt.Sprintf("#%d", i)
	}
	return fmt.Sprintf("%q", name)
}

// parseHCM extracts the HCM config and its raw inline route_config from the
// raw `typed_config` JSON, or returns the decode error.
func parseHCM(raw json.RawMessage) (*model.HCMConfig, json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, errors.New("no typed_config")
	}
	var hcm rawHCM
	if err := json.Unmarshal(raw, &hcm); err != nil {
		return nil, nil, err
	}

	result := &model.HCMConfig{
		RouteConfigName: hcm.RDS.RouteConfigName,
	}

	for _, hf := range hcm.HTTPFilters {
		filter := parseHTTPFilter(hf)
		result.HTTPFilters = append(result.HTTPFilters, filter)
		if filter.Type == routerType {
			var router rawRouter
			if err := json.Unmarshal(hf.TypedConfig, &router); err == nil {
				for _, uf := range router.UpstreamHTTPFilters {
					result.UpstreamFilters = append(result.UpstreamFilters, parseHTTPFilter(uf))
				}
			}
		}
	}

	return result, hcm.RouteConfig, nil
}

const (
	hcmType           = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"
	routerType        = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	matcherType       = "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	compositeType     = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"
	executeActionType = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction"
)

// parseHTTPFilter converts one http_filters entry. Type is the filter's own
// @type, looking through an ExtensionWithMatcher wrapper and, when that wraps a
// Composite filter whose actions all run one filter type, through to that type.
func parseHTTPFilter(hf rawHTTPFilter) model.HTTPFilter {
	filter := model.HTTPFilter{Name: hf.Name, Disabled: hf.Disabled}
	if len(hf.TypedConfig) == 0 {
		return filter
	}
	var tc map[string]any
	if err := json.Unmarshal(hf.TypedConfig, &tc); err == nil {
		filter.TypedConfig = tc // raw config, shown in interactive mode
	}
	var typed rawFilterType
	if err := json.Unmarshal(hf.TypedConfig, &typed); err == nil {
		filter.Type = typed.Type
		if typed.Type == matcherType && typed.ExtensionConfig.TypedConfig.Type != "" {
			filter.Type = typed.ExtensionConfig.TypedConfig.Type
		}
		if filter.Type == compositeType {
			if delegated := compositeDelegate(tc); delegated != "" {
				filter.Type = delegated
			}
		}
	}
	return filter
}

// parseRouteConfig converts a raw route config into the model.
func parseRouteConfig(raw rawRouteConfig) *model.RouteConfig {
	rc := &model.RouteConfig{
		Name:                 raw.Name,
		TypedPerFilterConfig: decodeAll(raw.TypedPerFilterConfig),
	}

	for _, rvh := range raw.VirtualHosts {
		vh := model.VirtualHost{
			Name:                 rvh.Name,
			Domains:              rvh.Domains,
			TypedPerFilterConfig: decodeAll(rvh.TypedPerFilterConfig),
		}

		for _, rr := range rvh.Routes {
			route := model.Route{
				Name: rr.Name,
				Match: model.RouteMatch{
					Prefix:              rr.Match.Prefix,
					PathSeparatedPrefix: rr.Match.PathSeparatedPrefix,
					Path:                rr.Match.Path,
					Regex:               rr.Match.SafeRegex.Regex,
				},
			}

			// Extract cluster from the route action
			if rr.Route.Cluster != "" {
				route.Cluster = rr.Route.Cluster
			}
			for _, wc := range rr.Route.WeightedClusters.Clusters {
				route.WeightedClusters = append(route.WeightedClusters, model.WeightedCluster{
					Name: wc.Name, Weight: wc.Weight, TypedPerFilterConfig: decodeAll(wc.TypedPerFilterConfig),
				})
			}

			// Extract URLRewrite from regex_rewrite on the route action
			if rr.Route.RegexRewrite.Pattern.Regex != "" || rr.Route.PrefixRewrite != "" {
				route.Rewrite = &model.RouteRewrite{
					RegexPattern: rr.Route.RegexRewrite.Pattern.Regex,
					Substitution: rr.Route.RegexRewrite.Substitution,
					Prefix:       rr.Route.PrefixRewrite,
				}
			}

			// Extract mirror clusters from request_mirror_policies
			for _, mp := range rr.Route.RequestMirrorPolicies {
				if mp.Cluster != "" {
					route.MirrorClusters = append(route.MirrorClusters, mp.Cluster)
				}
			}

			// Extract request headers to add (HTTPRouteFilter RequestHeaderModifier)
			for _, h := range rr.RequestHeadersToAdd {
				route.RequestHeadersToAdd = append(route.RequestHeadersToAdd, model.HeaderOperation{
					Key:   h.Header.Key,
					Value: h.Header.Value,
				})
			}

			// Extract response headers to add (HTTPRouteFilter ResponseHeaderModifier)
			for _, h := range rr.ResponseHeadersToAdd {
				route.ResponseHeadersToAdd = append(route.ResponseHeadersToAdd, model.HeaderOperation{
					Key:   h.Header.Key,
					Value: h.Header.Value,
				})
			}

			// Extract response headers to remove
			route.ResponseHeadersToRemove = rr.ResponseHeadersToRemove

			// Extract header matches (Exact and RegularExpression)
			for _, hm := range rr.Match.Headers {
				if hm.StringMatch.SafeRegex.Regex != "" {
					route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
						Name:  hm.Name,
						Value: hm.StringMatch.SafeRegex.Regex,
						Regex: true,
					})
				} else {
					route.Match.Headers = append(route.Match.Headers, model.HeaderMatch{
						Name:  hm.Name,
						Value: hm.StringMatch.Exact,
					})
				}
			}

			// Extract query parameter matches (Exact and RegularExpression)
			for _, qp := range rr.Match.QueryParameters {
				if qp.StringMatch.SafeRegex.Regex != "" {
					route.Match.QueryParams = append(route.Match.QueryParams, model.QueryParamMatch{
						Name:  qp.Name,
						Value: qp.StringMatch.SafeRegex.Regex,
						Regex: true,
					})
				} else {
					route.Match.QueryParams = append(route.Match.QueryParams, model.QueryParamMatch{
						Name:  qp.Name,
						Value: qp.StringMatch.Exact,
					})
				}
			}

			route.TypedPerFilterConfig = decodeAll(rr.TypedPerFilterConfig)
			if rr.Metadata != nil {
				route.Metadata = decodeAll(rr.Metadata.FilterMetadata)
			}

			vh.Routes = append(vh.Routes, route)
		}

		rc.VirtualHosts = append(rc.VirtualHosts, vh)
	}

	return rc
}

// decodeAll decodes each value of a raw JSON map, skipping values that don't
// decode. It returns nil when nothing decodes.
func decodeAll(raw map[string]json.RawMessage) map[string]any {
	var out map[string]any
	for k, v := range raw {
		var parsed any
		if err := json.Unmarshal(v, &parsed); err == nil {
			if out == nil {
				out = map[string]any{}
			}
			out[k] = parsed
		}
	}
	return out
}

// --- Raw JSON structs matching the actual Envoy config dump format ---
//
// These types are intentionally minimal: only the fields that krp currently
// reads are declared. Unknown fields are silently ignored by encoding/json,
// which keeps these structs stable against Envoy API additions.

// configDump is the top-level /config_dump response. Each element of Configs
// is a distinct config section identified by its "@type" field.
type configDump struct {
	Configs []json.RawMessage `json:"configs"`
}

// typedConfig is used to peek at the "@type" discriminator before fully
// unmarshalling a config section into its concrete type.
type typedConfig struct {
	Type string `json:"@type"`
}

// Listeners

// listenersConfigDump corresponds to type.googleapis.com/envoy.admin.v3.ListenersConfigDump.
// Fields stay raw so each one is decoded on its own.
type listenersConfigDump struct {
	StaticListeners  json.RawMessage `json:"static_listeners"`
	DynamicListeners json.RawMessage `json:"dynamic_listeners"`
}

// rawListener is a single entry from `dynamic_listeners`. An entry without a
// name and with only `error_state` is an LDS-wide error.
type rawListener struct {
	Name          string            `json:"name"`
	ActiveState   *rawListenerState `json:"active_state"`
	WarmingState  *rawListenerState `json:"warming_state"`
	DrainingState *rawListenerState `json:"draining_state"`
	ErrorState    *rawUpdateFailure `json:"error_state"`
}

// rawListenerState is one version of a listener (DynamicListenerState).
type rawListenerState struct {
	LastUpdated string          `json:"last_updated"`
	Listener    rawListenerBody `json:"listener"`
}

// rawStaticListener is a `static_listeners` entry: a bootstrap listener.
type rawStaticListener struct {
	LastUpdated string          `json:"last_updated"`
	Listener    rawListenerBody `json:"listener"`
}

// rawListenerBody is one Listener resource.
type rawListenerBody struct {
	Name    string `json:"name"`
	Address struct {
		SocketAddress struct {
			Address   string `json:"address"`
			PortValue int    `json:"port_value"`
		} `json:"socket_address"`
	} `json:"address"`
	FilterChains []rawFilterChain `json:"filter_chains"`
}

// rawUpdateFailure is a rejected update (UpdateFailureState).
type rawUpdateFailure struct {
	Details           string `json:"details"`
	LastUpdateAttempt string `json:"last_update_attempt"`
}

// rawFilterChain is a single filter_chain within a listener.
type rawFilterChain struct {
	Name             string `json:"name"`
	FilterChainMatch struct {
		ServerNames []string `json:"server_names"`
	} `json:"filter_chain_match"`
	Filters []rawNetworkFilter `json:"filters"`
}

// rawNetworkFilter is a single entry in a filter chain's `filters` list.
type rawNetworkFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

// HCM

// rawHCM represents the `typed_config` of an `envoy.filters.network.http_connection_manager`
// network filter. Only the `rds`, `route_config` and `http_filters` fields are extracted;
// all other HCM options (timeouts, access log, tracing, etc.) are ignored.
type rawHCM struct {
	RDS struct {
		RouteConfigName string `json:"route_config_name"`
	} `json:"rds"`
	RouteConfig json.RawMessage `json:"route_config"` // inline; decoded in join
	HTTPFilters []rawHTTPFilter `json:"http_filters"`
}

// rawHTTPFilter is a single entry in the HCM `http_filters` list.
type rawHTTPFilter struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
	Disabled    bool            `json:"disabled"`
}

// compositeDelegate returns the filter type every ExecuteFilterAction in v
// runs, or "" when there are none, they differ, or one has no static type.
func compositeDelegate(v any) string {
	var found string
	mixed := false
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if v["@type"] == executeActionType {
				// An action without a static type (e.g. dynamic_config) could run anything.
				t := ""
				if inner, ok := v["typed_config"].(map[string]any); ok {
					if tc, ok := inner["typed_config"].(map[string]any); ok {
						t, _ = tc["@type"].(string)
					}
				}
				if t == "" || (found != "" && found != t) {
					mixed = true
				}
				found = t
				return
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(v)
	if mixed {
		return ""
	}
	return found
}

// rawFilterType reads a filter's @type and, for an ExtensionWithMatcher, the
// wrapped filter's @type.
type rawFilterType struct {
	Type            string `json:"@type"`
	ExtensionConfig struct {
		TypedConfig struct {
			Type string `json:"@type"`
		} `json:"typed_config"`
	} `json:"extension_config"`
}

// rawRouter reads the router filter's upstream HTTP filters.
type rawRouter struct {
	UpstreamHTTPFilters []rawHTTPFilter `json:"upstream_http_filters"`
}

// Routes

// routesConfigDump corresponds to type.googleapis.com/envoy.admin.v3.RoutesConfigDump.
// Fields and entries stay raw so each one is decoded on its own.
type routesConfigDump struct {
	StaticRouteConfigs  json.RawMessage `json:"static_route_configs"`
	DynamicRouteConfigs json.RawMessage `json:"dynamic_route_configs"`
}

// rawRouteConfig is the `route_config` object within a dynamic route config entry.
type rawRouteConfig struct {
	Name                 string                     `json:"name"`
	VirtualHosts         []rawVirtualHost           `json:"virtual_hosts"`
	TypedPerFilterConfig map[string]json.RawMessage `json:"typed_per_filter_config"`
}

// rawVirtualHost is a single virtual_host within a route config.
type rawVirtualHost struct {
	Name                 string                     `json:"name"`
	Domains              []string                   `json:"domains"`
	Routes               []rawRoute                 `json:"routes"`
	TypedPerFilterConfig map[string]json.RawMessage `json:"typed_per_filter_config"`
}

// rawRoute is a single route entry within a virtual host.
type rawRoute struct {
	Name  string `json:"name"`
	Match struct {
		Prefix              string `json:"prefix"`
		PathSeparatedPrefix string `json:"path_separated_prefix"`
		Path                string `json:"path"`
		// safe_regex is used for RegularExpression path matchers from the K8S Gateway API
		SafeRegex struct {
			Regex string `json:"regex"`
		} `json:"safe_regex"`
		Headers []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact     string `json:"exact"`
				SafeRegex struct {
					Regex string `json:"regex"`
				} `json:"safe_regex"`
			} `json:"string_match"`
		} `json:"headers"`
		// query_parameters captures K8S Gateway API QueryParam matchers
		QueryParameters []struct {
			Name        string `json:"name"`
			StringMatch struct {
				Exact     string `json:"exact"`
				SafeRegex struct {
					Regex string `json:"regex"`
				} `json:"safe_regex"`
			} `json:"string_match"`
		} `json:"query_parameters"`
	} `json:"match"`
	Route struct {
		Cluster          string `json:"cluster"`
		WeightedClusters struct {
			Clusters []struct {
				Name                 string                     `json:"name"`
				Weight               int                        `json:"weight"`
				TypedPerFilterConfig map[string]json.RawMessage `json:"typed_per_filter_config"`
			} `json:"clusters"`
		} `json:"weighted_clusters"`
		PrefixRewrite         string `json:"prefix_rewrite"`
		RequestMirrorPolicies []struct {
			Cluster string `json:"cluster"`
		} `json:"request_mirror_policies"`
		// regex_rewrite captures URLRewrite HTTPRouteFilter path rewrites
		RegexRewrite struct {
			Pattern struct {
				Regex string `json:"regex"`
			} `json:"pattern"`
			Substitution string `json:"substitution"`
		} `json:"regex_rewrite"`
	} `json:"route"`
	// Native Envoy header manipulation (used by HTTPRouteFilter RequestHeaderModifier / ResponseHeaderModifier)
	RequestHeadersToAdd []struct {
		Header struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"header"`
	} `json:"request_headers_to_add"`
	ResponseHeadersToAdd []struct {
		Header struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"header"`
	} `json:"response_headers_to_add"`
	ResponseHeadersToRemove []string                   `json:"response_headers_to_remove"`
	TypedPerFilterConfig    map[string]json.RawMessage `json:"typed_per_filter_config"`
	Metadata                *rawRouteMetadata          `json:"metadata"`
}

// rawRouteMetadata holds the `metadata` object on a route, used for EKTP policy cross-references.
type rawRouteMetadata struct {
	FilterMetadata map[string]json.RawMessage `json:"filter_metadata"`
}

// copyJSON deep-copies decoded JSON: maps, slices and scalars. It returns nil for nil.
func copyJSON(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = copyValue(v)
	}
	return out
}

func copyValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return copyJSON(v)
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = copyValue(e)
		}
		return out
	}
	return v
}

// cloneRouteConfig returns a deep copy of rc so each HCM owns its own RouteConfig.
// This prevents mutations in later phases from corrupting filter chains that share
// the same route_config_name.
func cloneRouteConfig(rc *model.RouteConfig) *model.RouteConfig {
	clone := &model.RouteConfig{
		Name:                 rc.Name,
		VirtualHosts:         make([]model.VirtualHost, len(rc.VirtualHosts)),
		TypedPerFilterConfig: copyJSON(rc.TypedPerFilterConfig),
	}
	for i, vh := range rc.VirtualHosts {
		cloneVH := model.VirtualHost{
			Name:                 vh.Name,
			Domains:              append([]string(nil), vh.Domains...),
			Routes:               make([]model.Route, len(vh.Routes)),
			TypedPerFilterConfig: copyJSON(vh.TypedPerFilterConfig),
		}
		for j, r := range vh.Routes {
			cloneRoute := r // copy all scalar fields
			// Deep-copy slice fields
			cloneRoute.MirrorClusters = append([]string(nil), r.MirrorClusters...)
			cloneRoute.WeightedClusters = append([]model.WeightedCluster(nil), r.WeightedClusters...)
			for k, wc := range cloneRoute.WeightedClusters {
				cloneRoute.WeightedClusters[k].TypedPerFilterConfig = copyJSON(wc.TypedPerFilterConfig)
			}
			cloneRoute.RequestHeadersToAdd = append([]model.HeaderOperation(nil), r.RequestHeadersToAdd...)
			cloneRoute.ResponseHeadersToAdd = append([]model.HeaderOperation(nil), r.ResponseHeadersToAdd...)
			cloneRoute.ResponseHeadersToRemove = append([]string(nil), r.ResponseHeadersToRemove...)
			cloneRoute.Match.Headers = append([]model.HeaderMatch(nil), r.Match.Headers...)
			cloneRoute.Match.QueryParams = append([]model.QueryParamMatch(nil), r.Match.QueryParams...)
			// Rewrite is a pointer — copy the struct value if set
			if r.Rewrite != nil {
				rw := *r.Rewrite
				cloneRoute.Rewrite = &rw
			}
			cloneRoute.TypedPerFilterConfig = copyJSON(r.TypedPerFilterConfig)
			cloneRoute.Metadata = copyJSON(r.Metadata)
			cloneVH.Routes[j] = cloneRoute
		}
		clone.VirtualHosts[i] = cloneVH
	}
	return clone
}

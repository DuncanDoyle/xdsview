// Package parser converts a raw Envoy /config_dump JSON payload into the
// domain model defined in [model]. The Envoy config_dump is a heterogeneous
// JSON array where each element has a "@type" discriminator; the parser dispatches
// on that field to extract the ListenersConfigDump, RoutesConfigDump and
// EcdsConfigDump sections, then joins them into a [model.Snapshot]. Every other
// section is read for its @type only and listed as ignored in
// [model.Snapshot.Coverage].
//
// Non-fatal errors (e.g. a single malformed section) are collected as warnings in
// the best-effort snapshot so the caller can surface them to the user rather than
// silently dropping data.
//
// # Occurrence IDs
//
// Parse assigns every occurrence a [model.ID] as a structural path: L0 (listener;
// dynamic listeners first, then static ones), L0/C1 (filter chain; L0/Cd is the
// default filter chain), L0/C1/N0 (network filter; an HCM has its network
// filter's ID), .../F3 (HTTP filter
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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"

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
	p := &parse{dynamic: map[string][]*routeEntry{}, ecds: map[string][]ecdsEntry{}}
	var listeners []rawListener
	var statics []rawStaticListener
	listenersAt := -1 // section index of the listeners, where inline sources go

	// Only @type is read from a section the parser ignores, so secrets are never decoded.
	for pos, raw := range dump.Configs {
		p.at = pos
		var typed typedConfig
		if err := json.Unmarshal(raw, &typed); err != nil {
			p.sections = append(p.sections, model.SectionCoverage{Status: model.CoverageIgnored})
			p.warn("", model.WarnSectionSkipped, "skipped config section: cannot read @type: %v", err)
			p.routesSkipped = true // it may have held route configs
			continue
		}
		p.sections = append(p.sections, model.SectionCoverage{Type: typed.Type, Status: model.CoverageIgnored})

		switch typed.Type {
		case listenersType, routesType, ecdsType:
			p.sections[pos].Status = model.CoverageParsed
		}
		switch typed.Type {
		case listenersType:
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

		case routesType:
			var rd routesConfigDump
			if err := json.Unmarshal(raw, &rd); err != nil {
				p.warn("", model.WarnSectionSkipped, "skipped RoutesConfigDump: %v", err)
				p.routesSkipped = true
				continue
			}
			p.routeSection(pos, rd)

		case ecdsType:
			p.ecdsSection(raw)
		}
	}
	p.at = listenersAt // later skips are in the listeners section
	// Missing RDS needs every dynamic route config: a complete capture, read in full.
	p.rdsComplete = capture == model.CaptureComplete && p.routesRead && !p.routesSkipped

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
	snapshot.Coverage.Sections = p.sections
	snapshot.Warnings = p.warnings
	return snapshot, nil
}

// Config dump sections the parser reads.
const (
	listenersType = "type.googleapis.com/envoy.admin.v3.ListenersConfigDump"
	routesType    = "type.googleapis.com/envoy.admin.v3.RoutesConfigDump"
	ecdsType      = "type.googleapis.com/envoy.admin.v3.EcdsConfigDump"
)

// parse holds the state of one [Parse] call.
type parse struct {
	dynamic  map[string][]*routeEntry // dynamic sources by name, the only ones RDS resolves to
	ecds     map[string][]ecdsEntry   // ECDS configs by name
	static   []*routeEntry
	inline   []*routeEntry
	batches  []batch // sources per dump section, for dump order
	nDynamic int
	nStatic  int
	warnings []model.Warning

	routesRead    bool // a RoutesConfigDump section was read
	routesSkipped bool // a route config, or a section that may hold one, was skipped
	rdsComplete   bool // an unmatched RDS name is missing, not unknown

	sections []model.SectionCoverage // one per dump section
	at       int                     // the section being parsed; -1 for none
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

// warn records a warning. One that skips something makes the current section partial.
func (p *parse) warn(subject model.ID, code, format string, args ...any) {
	p.warnings = append(p.warnings, model.Warning{Subject: subject, Code: code, Message: fmt.Sprintf(format, args...)})
	switch code {
	case model.WarnSectionSkipped, model.WarnRouteConfigSkipped, model.WarnHCMSkipped, model.WarnECDSSkipped:
		if p.at >= 0 && p.at < len(p.sections) && p.sections[p.at].Status == model.CoverageParsed {
			p.sections[p.at].Status = model.CoveragePartial
		}
	}
}

// routeSection adds the static and dynamic entries of one RoutesConfigDump
// section. Each field and entry is decoded on its own, so a bad one costs only
// itself.
func (p *parse) routeSection(pos int, rd routesConfigDump) {
	p.routesRead = true
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

// ecdsSection adds the entries of one EcdsConfigDump section. Envoy writes one
// section per filter category, so a name can recur; type_urls tells them apart.
func (p *parse) ecdsSection(raw json.RawMessage) {
	var section struct {
		Filters []json.RawMessage `json:"ecds_filters"`
	}
	if err := json.Unmarshal(raw, &section); err != nil {
		p.warn("", model.WarnSectionSkipped, "skipped EcdsConfigDump: %v", err)
		return
	}
	for _, f := range section.Filters {
		var entry struct {
			Filter *struct {
				Name        string          `json:"name"`
				TypedConfig json.RawMessage `json:"typed_config"`
			} `json:"ecds_filter"`
		}
		var typed typedConfig
		err := json.Unmarshal(f, &entry)
		if err == nil && entry.Filter != nil {
			err = json.Unmarshal(entry.Filter.TypedConfig, &typed)
		}
		if err != nil {
			p.warn("", model.WarnECDSSkipped, "skipped an ECDS config: %v", err)
			continue
		}
		if entry.Filter == nil || entry.Filter.Name == "" {
			continue // only an error_state: Envoy has no config for it
		}
		p.ecds[entry.Filter.Name] = append(p.ecds[entry.Filter.Name], ecdsEntry{typ: typed.Type, config: entry.Filter.TypedConfig})
	}
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
		p.routesSkipped = true
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
		p.routesSkipped = true
		return nil
	}
	// A null or nameless entry would join any HCM without an RDS name.
	if entry.RouteConfig.Name == "" {
		p.warn("", model.WarnRouteConfigSkipped, "skipped a route config with no name")
		p.routesSkipped = true
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
		l.FilterChains = append(l.FilterChains, p.filterChain(l, model.ID(fmt.Sprintf("%s/C%d", l.ID, i)), chainLabel(i, rfc.Name), rfc))
	}
	// Envoy uses the default chain when no other chain matches, and ignores
	// its filter_chain_match (listener.proto).
	if d := body.DefaultFilterChain; d != nil {
		label := "default"
		if d.Name != "" {
			label = fmt.Sprintf("default %q", d.Name)
		}
		fc := p.filterChain(l, l.ID+"/Cd", label, *d)
		fc.Default, fc.TLS = true, nil
		l.FilterChains = append(l.FilterChains, fc)
	}
}

// filterChain converts one filter chain of l, with its ID and warning label.
func (p *parse) filterChain(l *model.Listener, id model.ID, chain string, rfc rawFilterChain) model.NetworkFilterChain {
	nfc := model.NetworkFilterChain{ID: id, Name: rfc.Name}

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
		label := fmt.Sprintf("listener %q filter chain %s", l.Name, chain)
		active := l.State.Status == model.ListenerActive
		hcmID := model.ID(fmt.Sprintf("%s/N%d", nfc.ID, n))
		hcm, inline, err := p.parseHCM(hcmID, nf.TypedConfig, active)
		if err != nil {
			p.warn(nfc.ID, model.WarnHCMSkipped, "%s: skipped HTTP connection manager: %v", label, err)
			continue
		}
		hcm.ID = hcmID
		p.join(hcm, inline, label, active)
		assignIDs(hcm)
		nfc.HCM = hcm
	}
	return nfc
}

// join resolves hcm's route config use: its inline config, else its RDS name
// against the dynamic sources. Each use gets its own copy of the content.
// active says whether the HCM's listener version is the active one.
func (p *parse) join(hcm *model.HCMConfig, inline json.RawMessage, label string, active bool) {
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
			// A warming or draining listener may wait on RDS that never came; leave it unknown.
			if p.rdsComplete && active {
				use.State = model.RouteConfigMissing
				p.warn(use.ID, model.WarnRDSMissing, "%s: route config %q is missing: no dynamic route config has that name in a complete dump",
					label, hcm.RouteConfigName)
			}
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
// raw `typed_config` JSON, or returns the decode error. id names the HCM in
// warnings; active says whether its listener version is the active one.
func (p *parse) parseHCM(id model.ID, raw json.RawMessage, active bool) (*model.HCMConfig, json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, errors.New("no typed_config")
	}
	var hcm rawHCM
	if err := json.Unmarshal(raw, &hcm); err != nil {
		return nil, nil, err
	}

	result := &model.HCMConfig{
		RouteConfigName: hcm.RDS.RouteConfigName,
		ScopedRoutes:    present(hcm.ScopedRoutes),
	}

	for i, hf := range hcm.HTTPFilters {
		filter := p.httpFilter(model.ID(fmt.Sprintf("%s/F%d", id, i)), hf, active)
		result.HTTPFilters = append(result.HTTPFilters, filter)
		if filter.Type == routerType {
			var router rawRouter
			if err := json.Unmarshal(hf.TypedConfig, &router); err == nil {
				for _, uf := range router.UpstreamHTTPFilters {
					uid := model.ID(fmt.Sprintf("%s/U%d", id, len(result.UpstreamFilters)))
					result.UpstreamFilters = append(result.UpstreamFilters, p.httpFilter(uid, uf, active))
				}
			}
		}
	}

	return result, hcm.RouteConfig, nil
}

// httpFilter converts one http_filters entry, id. A filter with
// config_discovery takes the ECDS config whose name is the filter's name
// (http_connection_manager.proto, HttpFilter.name) and whose type is in
// type_urls; with none, an active listener runs default_config
// (config_source.proto, ExtensionConfigSource). Otherwise it has no config.
func (p *parse) httpFilter(id model.ID, hf rawHTTPFilter, active bool) model.HTTPFilter {
	d := hf.ConfigDiscovery
	if d == nil {
		return parseHTTPFilter(hf)
	}
	disc := &model.Discovery{TypeURLs: d.TypeURLs, DefaultConfig: present(d.DefaultConfig), ApplyDefaultWithoutWarming: d.ApplyDefaultConfigWithoutWarming}
	var found []json.RawMessage
	for _, e := range p.ecds[hf.Name] {
		if slices.ContainsFunc(d.TypeURLs, func(u string) bool { return typeName(u) == typeName(e.typ) }) {
			found = append(found, e.config)
		}
	}
	hf.TypedConfig = nil
	switch {
	case len(found) == 1:
		hf.TypedConfig, disc.Source = found[0], model.ConfigECDS
	case len(found) > 1:
		p.warn(id, model.WarnECDSAmbiguous, "filter %q: %d ECDS configs have its name and an allowed type", hf.Name, len(found))
	case disc.DefaultConfig && active:
		hf.TypedConfig, disc.Source = d.DefaultConfig, model.ConfigECDSDefault
	}
	filter := parseHTTPFilter(hf)
	filter.Discovery = disc
	return filter
}

// present reports whether raw holds a JSON value other than null.
func present(raw json.RawMessage) bool { return len(raw) > 0 && string(raw) != "null" }

// typeName strips the type URL prefix: "type.googleapis.com/a.B" → "a.B".
// Envoy compares ECDS types by this name (config_discovery_impl.h).
func typeName(typeURL string) string { return typeURL[strings.LastIndex(typeURL, "/")+1:] }

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
			route := model.Route{Name: rr.Name, Match: parseMatch(rr.Match), Action: routeAction(rr)}

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
	FilterChains       []rawFilterChain `json:"filter_chains"`
	DefaultFilterChain *rawFilterChain  `json:"default_filter_chain"`
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
	RouteConfig  json.RawMessage `json:"route_config"`  // inline; decoded in join
	ScopedRoutes json.RawMessage `json:"scoped_routes"` // not read beyond its presence
	HTTPFilters  []rawHTTPFilter `json:"http_filters"`
}

// rawHTTPFilter is a single entry in the HCM `http_filters` list.
type rawHTTPFilter struct {
	Name            string              `json:"name"`
	TypedConfig     json.RawMessage     `json:"typed_config"`
	ConfigDiscovery *rawConfigDiscovery `json:"config_discovery"`
	Disabled        bool                `json:"disabled"`
}

// rawConfigDiscovery is an ExtensionConfigSource.
type rawConfigDiscovery struct {
	DefaultConfig                    json.RawMessage `json:"default_config"`
	ApplyDefaultConfigWithoutWarming bool            `json:"apply_default_config_without_warming"`
	TypeURLs                         []string        `json:"type_urls"`
}

// ecdsEntry is one EcdsConfigDump ecds_filter: a TypedExtensionConfig.
type ecdsEntry struct {
	typ    string          // the typed_config @type
	config json.RawMessage // the typed_config
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
	Name  string        `json:"name"`
	Match rawRouteMatch `json:"match"`
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
		ClusterHeader                string `json:"cluster_header"`
		ClusterSpecifierPlugin       string `json:"cluster_specifier_plugin"`
		InlineClusterSpecifierPlugin *struct {
			Extension struct {
				Name        string      `json:"name"`
				TypedConfig typedConfig `json:"typed_config"`
			} `json:"extension"`
		} `json:"inline_cluster_specifier_plugin"`
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
	Redirect                *rawRedirect               `json:"redirect"`
	DirectResponse          *rawDirectResponse         `json:"direct_response"`
	FilterAction            *struct {
		Action typedConfig `json:"action"`
	} `json:"filter_action"`
	fields []string // every field the dump sets
}

// rawRedirect is a RedirectAction.
type rawRedirect struct {
	HTTPSRedirect  bool             `json:"https_redirect"`
	SchemeRedirect string           `json:"scheme_redirect"`
	HostRedirect   string           `json:"host_redirect"`
	PortRedirect   int              `json:"port_redirect"`
	PathRedirect   string           `json:"path_redirect"`
	PrefixRewrite  string           `json:"prefix_rewrite"`
	RegexRewrite   *rawRegexRewrite `json:"regex_rewrite"`
	PathRewrite    string           `json:"path_rewrite"`
	ResponseCode   json.RawMessage  `json:"response_code"`
	StripQuery     bool             `json:"strip_query"`
}

type rawRegexRewrite struct {
	Pattern      rawRegex `json:"pattern"`
	Substitution string   `json:"substitution"`
}

// rawDirectResponse is a DirectResponseAction. The body stays raw: only its
// size and source are read.
type rawDirectResponse struct {
	Status     int                        `json:"status"`
	Body       map[string]json.RawMessage `json:"body"`
	BodyFormat json.RawMessage            `json:"body_format"`
}

// redirectCodes maps RedirectAction.RedirectResponseCode, by name and by
// number, to its HTTP status. Unset is MOVED_PERMANENTLY.
var redirectCodes = map[string]int{
	"MOVED_PERMANENTLY": 301, "FOUND": 302, "SEE_OTHER": 303, "TEMPORARY_REDIRECT": 307, "PERMANENT_REDIRECT": 308,
	"0": 301, "1": 302, "2": 303, "3": 307, "4": 308,
}

func parseRedirect(r rawRedirect) *model.Redirect {
	out := &model.Redirect{
		ResponseCode: 301, Scheme: r.SchemeRedirect, Host: r.HostRedirect, Port: r.PortRedirect,
		Path: r.PathRedirect, PrefixRewrite: r.PrefixRewrite, PathRewrite: r.PathRewrite, StripQuery: r.StripQuery,
	}
	if r.HTTPSRedirect {
		out.Scheme = "https"
	}
	if rx := r.RegexRewrite; rx != nil {
		out.RegexPattern, out.RegexSubstitution = rx.Pattern.Regex, rx.Substitution
	}
	if c, ok := redirectCodes[strings.Trim(string(r.ResponseCode), `"`)]; ok {
		out.ResponseCode = c
	}
	return out
}

// parseDirectResponse reads a direct response's status and its body's
// source and size. The body is decoded only to measure it.
func parseDirectResponse(d rawDirectResponse) *model.DirectResponse {
	out := &model.DirectResponse{Status: d.Status, BodyFormat: present(d.BodyFormat)}
	var s string
	switch {
	case present(d.Body["inline_string"]):
		if json.Unmarshal(d.Body["inline_string"], &s) == nil {
			n := len(s)
			out.BodySource, out.BodySize = model.BodyInline, &n
		}
	case present(d.Body["inline_bytes"]):
		if json.Unmarshal(d.Body["inline_bytes"], &s) == nil {
			if b, err := base64.StdEncoding.DecodeString(s); err == nil {
				n := len(b)
				out.BodySource, out.BodySize = model.BodyInline, &n
			}
		}
	case present(d.Body["filename"]):
		out.BodySource = model.BodyFile
	case present(d.Body["environment_variable"]):
		out.BodySource = model.BodyEnv
	}
	return out
}

// UnmarshalJSON decodes the route and records which fields it sets.
func (r *rawRoute) UnmarshalJSON(b []byte) error {
	type plain rawRoute
	if err := json.Unmarshal(b, (*plain)(r)); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	r.fields = slices.Sorted(maps.Keys(fields))
	return nil
}

// routeAction reads the route's action oneof (route_components.proto, Route.action).
func routeAction(rr rawRoute) model.RouteAction {
	var a model.RouteAction
	switch {
	case slices.Contains(rr.fields, "route"):
		a.Kind = model.ActionRoute
		a.ClusterHeader = rr.Route.ClusterHeader
		a.ClusterSpecifierPlugin = rr.Route.ClusterSpecifierPlugin
		if p := rr.Route.InlineClusterSpecifierPlugin; p != nil {
			a.InlineClusterSpecifierPlugin = &model.Extension{Name: p.Extension.Name, Type: p.Extension.TypedConfig.Type}
		}
	case rr.Redirect != nil:
		a.Kind, a.Redirect = model.ActionRedirect, parseRedirect(*rr.Redirect)
	case rr.DirectResponse != nil:
		a.Kind, a.DirectResponse = model.ActionDirectResponse, parseDirectResponse(*rr.DirectResponse)
	case rr.FilterAction != nil:
		a.Kind, a.FilterActionType = model.ActionFilter, rr.FilterAction.Action.Type
	case slices.Contains(rr.fields, "non_forwarding_action"):
		a.Kind = model.ActionNonForwarding
	}
	return a
}

// rawRouteMatch is a RouteMatch. fields lists every field the dump sets.
type rawRouteMatch struct {
	Prefix              string             `json:"prefix"`
	PathSeparatedPrefix string             `json:"path_separated_prefix"`
	Path                string             `json:"path"`
	SafeRegex           rawRegex           `json:"safe_regex"`
	ConnectMatcher      json.RawMessage    `json:"connect_matcher"`
	CaseSensitive       *bool              `json:"case_sensitive"`
	Headers             []rawHeaderMatcher `json:"headers"`
	QueryParameters     []rawQueryMatcher  `json:"query_parameters"`
	fields              []string
}

// UnmarshalJSON decodes the match and records which fields it sets.
func (m *rawRouteMatch) UnmarshalJSON(b []byte) error {
	type plain rawRouteMatch
	if err := json.Unmarshal(b, (*plain)(m)); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	m.fields = slices.Sorted(maps.Keys(fields))
	return nil
}

// modelledMatch are the RouteMatch fields parseMatch reads.
var modelledMatch = []string{"prefix", "path", "safe_regex", "path_separated_prefix", "connect_matcher", "case_sensitive", "headers", "query_parameters"}

type rawRegex struct {
	Regex string `json:"regex"`
}

// rawHeaderMatcher is a HeaderMatcher, with the deprecated top-level forms.
type rawHeaderMatcher struct {
	Name           string            `json:"name"`
	ExactMatch     *string           `json:"exact_match"`
	SafeRegexMatch *rawRegex         `json:"safe_regex_match"`
	RangeMatch     *rawRange         `json:"range_match"`
	PresentMatch   *bool             `json:"present_match"`
	PrefixMatch    *string           `json:"prefix_match"`
	SuffixMatch    *string           `json:"suffix_match"`
	ContainsMatch  *string           `json:"contains_match"`
	StringMatch    *rawStringMatcher `json:"string_match"`
	InvertMatch    bool              `json:"invert_match"`
	MissingAsEmpty bool              `json:"treat_missing_header_as_empty"`
}

// rawRange is an Int64Range; ProtoJSON writes int64 as a string.
type rawRange struct {
	Start json.RawMessage `json:"start"`
	End   json.RawMessage `json:"end"`
}

// rawQueryMatcher is a QueryParameterMatcher.
type rawQueryMatcher struct {
	Name         string            `json:"name"`
	StringMatch  *rawStringMatcher `json:"string_match"`
	PresentMatch *bool             `json:"present_match"`
}

// rawStringMatcher is a StringMatcher.
type rawStringMatcher struct {
	Exact     *string   `json:"exact"`
	Prefix    *string   `json:"prefix"`
	Suffix    *string   `json:"suffix"`
	Contains  *string   `json:"contains"`
	SafeRegex *rawRegex `json:"safe_regex"`
	Custom    *struct {
		TypedConfig typedConfig `json:"typed_config"`
	} `json:"custom"`
	IgnoreCase bool `json:"ignore_case"`
}

// parseMatch converts a route match. Fields it doesn't model go to Unparsed.
func parseMatch(m rawRouteMatch) model.RouteMatch {
	out := model.RouteMatch{
		Prefix:              m.Prefix,
		PathSeparatedPrefix: m.PathSeparatedPrefix,
		Path:                m.Path,
		Regex:               m.SafeRegex.Regex,
		Connect:             present(m.ConnectMatcher),
		CaseSensitive:       m.CaseSensitive,
	}
	for _, h := range m.Headers {
		out.Headers = append(out.Headers, headerMatch(h))
	}
	for _, q := range m.QueryParameters {
		qm := model.QueryParamMatch{Name: q.Name, Kind: model.MatchPresent}
		switch {
		case q.StringMatch != nil:
			qm.Kind, qm.Value, qm.IgnoreCase = stringMatch(*q.StringMatch)
		case q.PresentMatch != nil && !*q.PresentMatch:
			qm.Kind = model.MatchAbsent
		}
		qm.Regex = qm.Kind == model.MatchRegex
		out.QueryParams = append(out.QueryParams, qm)
	}
	for _, f := range m.fields {
		if !slices.Contains(modelledMatch, f) {
			out.Unparsed = append(out.Unparsed, f)
		}
	}
	return out
}

// headerMatch converts a header matcher. With no match specifier, Envoy
// matches on presence (source/common/http/header_utility.cc).
func headerMatch(h rawHeaderMatcher) model.HeaderMatch {
	out := model.HeaderMatch{Name: h.Name, Kind: model.MatchPresent, Invert: h.InvertMatch, MissingAsEmpty: h.MissingAsEmpty}
	str := func(k model.MatchKind, v *string) { out.Kind, out.Value = k, *v }
	switch {
	case h.StringMatch != nil:
		out.Kind, out.Value, out.IgnoreCase = stringMatch(*h.StringMatch)
	case h.ExactMatch != nil && *h.ExactMatch == "":
		out.Kind = model.MatchAnyValue // header_utility.h: an empty expected value matches any value
	case h.ExactMatch != nil:
		str(model.MatchExact, h.ExactMatch)
	case h.PrefixMatch != nil:
		str(model.MatchPrefix, h.PrefixMatch)
	case h.SuffixMatch != nil:
		str(model.MatchSuffix, h.SuffixMatch)
	case h.ContainsMatch != nil:
		str(model.MatchContains, h.ContainsMatch)
	case h.SafeRegexMatch != nil:
		out.Kind, out.Value = model.MatchRegex, h.SafeRegexMatch.Regex
	case h.RangeMatch != nil:
		out.Kind, out.Value = model.MatchRange, "["+int64Text(h.RangeMatch.Start)+","+int64Text(h.RangeMatch.End)+")"
	case h.PresentMatch != nil && !*h.PresentMatch:
		out.Kind = model.MatchAbsent
	}
	out.Regex = out.Kind == model.MatchRegex
	return out
}

// stringMatch returns a StringMatcher's kind, value and ignore_case. One with
// no form set compares exactly with the empty string.
func stringMatch(s rawStringMatcher) (model.MatchKind, string, bool) {
	switch {
	case s.Prefix != nil:
		return model.MatchPrefix, *s.Prefix, s.IgnoreCase
	case s.Suffix != nil:
		return model.MatchSuffix, *s.Suffix, s.IgnoreCase
	case s.Contains != nil:
		return model.MatchContains, *s.Contains, s.IgnoreCase
	case s.SafeRegex != nil:
		return model.MatchRegex, s.SafeRegex.Regex, false // ignore_case has no effect on safe_regex
	case s.Custom != nil:
		return model.MatchCustom, s.Custom.TypedConfig.Type, false
	case s.Exact != nil:
		return model.MatchExact, *s.Exact, s.IgnoreCase
	}
	return model.MatchExact, "", s.IgnoreCase
}

// int64Text writes a ProtoJSON int64, a string or a number, as digits; unset is 0.
func int64Text(raw json.RawMessage) string {
	if !present(raw) {
		return "0"
	}
	return strings.Trim(string(raw), `"`)
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

// cloneAction returns a copy of a that shares no pointers with it.
func cloneAction(a model.RouteAction) model.RouteAction {
	if p := a.InlineClusterSpecifierPlugin; p != nil {
		c := *p
		a.InlineClusterSpecifierPlugin = &c
	}
	if r := a.Redirect; r != nil {
		c := *r
		a.Redirect = &c
	}
	if d := a.DirectResponse; d != nil {
		c := *d
		if d.BodySize != nil {
			n := *d.BodySize
			c.BodySize = &n
		}
		a.DirectResponse = &c
	}
	return a
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
			cloneRoute.Match.Unparsed = append([]string(nil), r.Match.Unparsed...)
			if r.Match.CaseSensitive != nil {
				cs := *r.Match.CaseSensitive
				cloneRoute.Match.CaseSensitive = &cs
			}
			// Rewrite is a pointer — copy the struct value if set
			if r.Rewrite != nil {
				rw := *r.Rewrite
				cloneRoute.Rewrite = &rw
			}
			cloneRoute.Action = cloneAction(r.Action)
			cloneRoute.TypedPerFilterConfig = copyJSON(r.TypedPerFilterConfig)
			cloneRoute.Metadata = copyJSON(r.Metadata)
			cloneVH.Routes[j] = cloneRoute
		}
		clone.VirtualHosts[i] = cloneVH
	}
	return clone
}

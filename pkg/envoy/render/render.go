// Package render writes a [model.Snapshot] and its effective chains as a
// deterministic plain-text tree: listeners, virtual hosts and routes, with each
// route's filter chain. It uses only the standard library and the model, so
// CLIs and UIs can print the same view. Callers add colour, a cursor,
// provenance or extra lines through the hooks in [Options], keyed by [Row].
package render

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Options controls what [Text] prints.
type Options struct {
	Header        string // first line, supplied by the caller (gateway, pod, capture time, file)
	ResponseOrder bool   // also print each route's response order
	AllFilters    bool   // list filters that don't run inline; default: one "not run:" line

	// Generic hooks for styled, interactive or provenance-aware callers; nil
	// gives plain text. Style gets the row the text belongs to.
	Style  func(c Class, r Row, s string) string
	Notes  func(r Row) []string // lines printed under a route or filter row; a route's come before its filters
	Labels func(r Row) []string // printed on the row itself, each after " · ", styled as ClassOrigin
}

// Row names one printed row. A filter row appears once per route, so Route
// and Slot together tell rows apart. Subject is the occurrence the row shows;
// Slot is empty above filter level and Route is empty above route level.
type Row struct{ Subject, Route, Slot model.ID }

// Class is the semantic role of a piece of text, for [Options.Style].
type Class int

// Classes.
const (
	ClassListener Class = iota
	ClassVirtualHost
	ClassRoute
	ClassFilter    // a filter that runs
	ClassFilterOff // a filter that doesn't run, or whose state is unknown
	ClassOrigin    // where something came from: origins, contributions, domains
	ClassBackend
	ClassWarning // listener states other than active, errors, missing config
)

const (
	branch = "├─ "
	last   = "└─ "
	pipe   = "│  "
	space  = "   "
)

// Text writes snap to w with each route's chain from chains, keyed by route
// ID as chain.Effective returns them. A route with no chain prints no filters.
// Output is LF-terminated with no trailing spaces, and follows the snapshot's
// order everywhere.
func Text(w io.Writer, snap *model.Snapshot, chains map[model.ID]model.EffectiveChain, opts Options) error {
	p := printer{opts: opts, chains: chains}
	if opts.Header != "" {
		p.line(clean(opts.Header))
	}
	if len(snap.Listeners) == 0 {
		p.line(p.style(ClassWarning, Row{}, "no listeners"))
	}
	for _, l := range snap.Listeners {
		p.listener(l)
	}
	var ignored []string
	for _, s := range snap.Coverage.Sections {
		if s.Status == model.CoverageIgnored {
			name := s.Type[strings.LastIndex(s.Type, ".")+1:]
			if name == "" {
				name = "(unreadable)"
			}
			ignored = append(ignored, name)
		}
	}
	if len(ignored) > 0 {
		p.line("ignored dump sections: " + clean(strings.Join(ignored, ", ")))
	}
	_, err := io.WriteString(w, p.b.String())
	return err
}

type printer struct {
	b      strings.Builder
	opts   Options
	chains map[model.ID]model.EffectiveChain
}

func (p *printer) line(s string) {
	p.b.WriteString(strings.TrimRight(s, " \t"))
	p.b.WriteByte('\n')
}

// style cleans s, which comes from the dump, then lets the caller style it.
func (p *printer) style(c Class, r Row, s string) string {
	s = clean(s)
	if p.opts.Style == nil || s == "" {
		return s
	}
	return p.opts.Style(c, r, s)
}

// labels returns r's labels, styled, each after " · ".
func (p *printer) labels(r Row) string {
	if p.opts.Labels == nil {
		return ""
	}
	var s string
	for _, l := range p.opts.Labels(r) {
		s += " · " + p.style(ClassOrigin, r, l)
	}
	return s
}

// notes prints r's notes under it, each line prefixed with indent.
func (p *printer) notes(r Row, indent string) {
	if p.opts.Notes == nil {
		return
	}
	for _, n := range p.opts.Notes(r) {
		p.line(indent + n)
	}
}

// clean escapes control characters, so dump strings can't move the cursor or
// change colours. ESC prints as `\x1b`. Spaces stay; line trims the line's end.
func clean(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isControl(r) {
			fmt.Fprintf(&b, `\x%02x`, r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) }

// address brackets an IPv6 host, e.g. ":::80" → "[::]:80".
func address(a string) string {
	i := strings.LastIndex(a, ":")
	if i <= 0 || !strings.Contains(a[:i], ":") || strings.HasPrefix(a, "[") {
		return clean(a)
	}
	return clean("[" + a[:i] + "]" + a[i:])
}

// node returns the prefix for a child and the indent for its own children.
func node(indent string, isLast bool) (string, string) {
	if isLast {
		return indent + last, indent + space
	}
	return indent + branch, indent + pipe
}

func (p *printer) listener(l model.Listener) {
	name := l.Name
	if name == "" {
		name = "LDS error"
	}
	row := Row{Subject: l.ID}
	parts := []string{p.style(ClassListener, row, name)}
	if l.Address != "" {
		parts[0] += " " + address(l.Address)
	}
	s := l.State
	if s.Status != "" {
		c := ClassWarning
		if s.Status == model.ListenerActive {
			c = ClassListener
		}
		parts = append(parts, p.style(c, row, string(s.Status)))
	}
	if l.Static {
		parts = append(parts, p.style(ClassListener, row, "static"))
	}
	if s.Updating {
		parts = append(parts, p.style(ClassWarning, row, "updating"))
	}
	if s.Draining {
		parts = append(parts, p.style(ClassWarning, row, "draining"))
	}
	if o := l.Origin; o != nil {
		parts = append(parts, p.style(ClassOrigin, row, listenerOrigin(*o)))
	}
	parts[len(parts)-1] += p.labels(row)
	if e := s.Error; e != nil {
		msg := "error: " + e.Details
		if e.LastAttempt != "" {
			msg += " (" + e.LastAttempt + ")"
		}
		parts = append(parts, p.style(ClassWarning, row, msg))
	}
	p.line(strings.Join(parts, " · "))

	// One plain filter chain is the common case: skip its level.
	if len(l.FilterChains) == 1 && l.FilterChains[0].TLS == nil && !l.FilterChains[0].Default && !otherNetworkFilters(l.FilterChains[0]) {
		p.chainBody(l.FilterChains[0], "")
		return
	}
	for i, fc := range l.FilterChains {
		prefix, indent := node("", i == len(l.FilterChains)-1)
		label := "chain"
		if fc.Default {
			label = "default chain"
		}
		label = strings.TrimSpace(label + " " + clean(fc.Name))
		if fc.TLS != nil && len(fc.TLS.SNIHosts) > 0 {
			label += " · TLS " + clean(strings.Join(fc.TLS.SNIHosts, ", "))
		}
		if otherNetworkFilters(fc) {
			label += " · " + networkFilters(fc)
		}
		p.line(prefix + label)
		p.chainBody(fc, indent)
	}
}

// otherNetworkFilters reports whether fc has an HCM and another network filter.
func otherNetworkFilters(fc model.NetworkFilterChain) bool {
	return fc.HCM != nil && len(fc.NetworkFilters) > 1
}

// networkFilters lists fc's network filters, each with its short type.
func networkFilters(fc model.NetworkFilterChain) string {
	var names []string
	for _, nf := range fc.NetworkFilters {
		names = append(names, nf.Name+shortType(nf.Type))
	}
	return "network filters: " + clean(strings.Join(names, ", "))
}

// shortType writes a type URL's message name, e.g. " (TcpProxy)"; "" when unset.
func shortType(typeURL string) string {
	if typeURL == "" {
		return ""
	}
	return " (" + typeURL[strings.LastIndex(typeURL, ".")+1:] + ")"
}

func (p *printer) chainBody(fc model.NetworkFilterChain, indent string) {
	hcm := fc.HCM
	if hcm == nil {
		prefix, _ := node(indent, true)
		line := p.style(ClassWarning, Row{Subject: fc.ID}, "no HTTP connection manager")
		if len(fc.NetworkFilters) > 0 {
			line = networkFilters(fc) + " · " + line
		}
		p.line(prefix + line)
		return
	}
	if hcm.RouteConfig == nil {
		prefix, body := node(indent, true)
		status := " not found"
		switch hcm.RouteConfigUse.State {
		case model.RouteConfigAmbiguous:
			status = " ambiguous: several route configs have this name"
		case model.RouteConfigMissing:
			status = " missing: no route config has this name in a complete dump"
		}
		text := "route config " + hcm.RouteConfigName + status
		if hcm.ScopedRoutes {
			text = "route config: scoped routes, not parsed"
		}
		p.line(prefix + p.style(ClassWarning, Row{Subject: hcm.RouteConfigUse.ID}, text))
		p.filters(p.chains[hcm.RouteConfigUse.ID], "", body)
		return
	}
	vhs := hcm.RouteConfig.VirtualHosts
	for i, vh := range vhs {
		row := Row{Subject: vh.ID}
		prefix, vhIndent := node(indent, i == len(vhs)-1)
		line := "vhost " + p.style(ClassVirtualHost, row, vh.Name)
		if len(vh.Domains) > 0 {
			line += " " + p.style(ClassOrigin, row, "["+strings.Join(vh.Domains, ", ")+"]")
		}
		if o := vh.Origin; o != nil {
			line += " · " + p.style(ClassOrigin, row, listenerOrigin(model.ListenerOrigin{Certainty: o.Certainty, Port: o.Listener.Port, Section: o.Listener.Section}))
		}
		p.line(prefix + line + p.labels(row))
		for j, r := range vh.Routes {
			rPrefix, rIndent := node(vhIndent, j == len(vh.Routes)-1)
			p.route(*hcm, r, rPrefix, rIndent)
		}
	}
}

func (p *printer) route(hcm model.HCMConfig, r model.Route, prefix, body string) {
	row := Row{Subject: r.ID, Route: r.ID}
	line := "route " + p.style(ClassRoute, row, formatMatch(r.Match))
	if o := routeOrigin(r.Origin); o != "" {
		line += " · " + p.style(ClassOrigin, row, o)
	}
	p.line(prefix + line + p.labels(row))
	p.notes(row, body)

	c := p.chains[r.ID]
	p.filters(c, r.ID, body)
	if c.Conditional {
		var parts []string
		for _, wc := range r.WeightedClusters {
			if len(wc.TypedPerFilterConfig) > 0 {
				parts = append(parts, wc.Name+" ("+strings.Join(slices.Sorted(maps.Keys(wc.TypedPerFilterConfig)), ", ")+")")
			}
		}
		p.line(body + "backend overrides: " + clean(strings.Join(parts, ", ")))
	}
	if p.opts.ResponseOrder {
		response := func(e model.ChainEntry) bool { return e.Runs != model.TriNo && e.Response != model.TriNo }
		p.rows(body, heading("response", c), c, r.ID, pick(c, response, true), func(e model.ChainEntry) model.Tri { return e.Response })
	}
	if len(hcm.UpstreamFilters) > 0 {
		var names []string
		for _, f := range hcm.UpstreamFilters {
			names = append(names, f.Name)
		}
		p.line(body + "router upstream filters (as configured): " + clean(strings.Join(names, ", ")))
	}
	if len(r.Policies) > 0 {
		var parts []string
		for _, pr := range r.Policies {
			parts = append(parts, policy(pr.Policy, pr.Field, pr.Certainty))
		}
		p.line(body + "policies: " + p.style(ClassOrigin, row, strings.Join(parts, ", ")))
	}
	p.routeActions(r, row, body)
}

// heading titles an order, and says when the chain is only the baseline.
func heading(order string, c model.EffectiveChain) string {
	if c.Conditional {
		return "filters (" + order + " order, baseline before backend selection):"
	}
	return "filters (" + order + " order):"
}

// pick returns the HCM indexes of c's entries that keep holds for, in HCM
// order or reversed. Indexes, not names: two filters can share a name.
func pick(c model.EffectiveChain, keep func(model.ChainEntry) bool, reverse bool) []int {
	var out []int
	for i, e := range c.Filters {
		if keep(e) {
			out = append(out, i)
		}
	}
	if reverse {
		slices.Reverse(out)
	}
	return out
}

// filterRow is the row of entry e printed under route (empty above route level).
func filterRow(route model.ID, e model.ChainEntry) Row {
	return Row{Subject: e.Slot, Route: route, Slot: e.Slot}
}

// filters prints the request order and, by default, a "not run:" line. An
// entry whose run state is unknown stays in position, never in "not run:".
func (p *printer) filters(c model.EffectiveChain, route model.ID, body string) {
	title := heading("request", c)
	request := func(e model.ChainEntry) model.Tri { return e.Request }
	if p.opts.AllFilters {
		p.rows(body, title, c, route, pick(c, func(model.ChainEntry) bool { return true }, false), request)
		return
	}
	p.rows(body, title, c, route, pick(c, func(e model.ChainEntry) bool { return e.Runs != model.TriNo && e.Request != model.TriNo }, false), request)

	var off, responseOnly []string
	for _, e := range c.Filters {
		row := filterRow(route, e)
		switch {
		case e.Runs == model.TriNo:
			off = append(off, p.style(ClassFilterOff, row, e.Name)+" ("+describeState(e)+")")
		case e.Request == model.TriNo && e.Response != model.TriNo:
			responseOnly = append(responseOnly, p.style(ClassFilter, row, e.Name))
		}
	}
	if len(off) > 0 {
		p.line(body + "not run: " + strings.Join(off, ", "))
	}
	if len(responseOnly) > 0 && !p.opts.ResponseOrder {
		p.line(body + "response only: " + strings.Join(responseOnly, ", "))
	}
}

// rows prints c's entries at idx as a numbered, aligned list. "?" marks an
// entry whose side in this order, or whose run state, is unknown. A
// conditional chain prints its heading even with no rows, so the baseline
// label isn't lost.
func (p *printer) rows(body, title string, c model.EffectiveChain, route model.ID, idx []int, side func(model.ChainEntry) model.Tri) {
	if len(idx) == 0 && !c.Conditional {
		return
	}
	p.line(body + title)
	nums := make([]string, len(idx))
	numWidth, nameWidth := 0, 0
	for n, i := range idx {
		nums[n] = fmt.Sprint(n + 1)
		if e := c.Filters[i]; side(e) == model.TriUnknown || e.Runs == model.TriUnknown {
			nums[n] += "?"
		}
		numWidth = max(numWidth, len(nums[n]))
		nameWidth = max(nameWidth, utf8.RuneCountInString(clean(c.Filters[i].Name)))
	}
	indent := body + "  "
	for n, i := range idx {
		e := c.Filters[i]
		row := filterRow(route, e)
		class := ClassFilter
		if e.Runs != model.TriYes {
			class = ClassFilterOff
		}
		num := nums[n] + strings.Repeat(" ", numWidth-len(nums[n]))
		pad := strings.Repeat(" ", nameWidth-utf8.RuneCountInString(clean(e.Name))+2)
		p.line(indent + num + " " + p.style(class, row, e.Name) + pad + describeState(e) + p.labels(row))
		p.notes(row, indent+"    ")
	}
}

func (p *printer) routeActions(r model.Route, row Row, body string) {
	var req, resp []string
	for _, h := range r.RequestHeadersToAdd {
		req = append(req, clean("+ "+h.Key+": "+h.Value))
	}
	for _, h := range r.ResponseHeadersToAdd {
		resp = append(resp, clean("+ "+h.Key+": "+h.Value))
	}
	for _, h := range r.ResponseHeadersToRemove {
		resp = append(resp, clean("- "+h))
	}
	if len(req) > 0 {
		p.line(body + "request headers: " + strings.Join(req, ", "))
	}
	if len(resp) > 0 {
		p.line(body + "response headers: " + strings.Join(resp, ", "))
	}
	if len(r.MirrorClusters) > 0 {
		p.line(body + "mirror: " + p.style(ClassBackend, row, strings.Join(r.MirrorClusters, ", ")))
	}
	if rw := r.Rewrite; rw != nil && rw.RegexPattern != "" {
		p.line(body + "rewrite: " + clean(rw.RegexPattern) + " → " + clean(rw.Substitution))
	}
	if rw := r.Rewrite; rw != nil && rw.Prefix != "" {
		p.line(body + "rewrite prefix: " + clean(rw.Prefix))
	}
	if r.Cluster != "" {
		p.line(body + "backend: " + p.style(ClassBackend, row, r.Cluster))
	}
	if len(r.WeightedClusters) > 0 {
		var parts []string
		for _, wc := range r.WeightedClusters {
			parts = append(parts, p.style(ClassBackend, row, wc.Name)+fmt.Sprintf(" (%d)", wc.Weight))
		}
		p.line(body + "backends: " + strings.Join(parts, ", "))
	}
	a := r.Action
	switch {
	case a.ClusterHeader != "":
		p.line(body + "backend: from request header " + p.style(ClassBackend, row, a.ClusterHeader))
	case a.ClusterSpecifierPlugin != "":
		p.line(body + "backend: from cluster specifier plugin " + p.style(ClassBackend, row, a.ClusterSpecifierPlugin))
	case a.InlineClusterSpecifierPlugin != nil:
		x := a.InlineClusterSpecifierPlugin
		p.line(body + "backend: from inline cluster specifier plugin " + p.style(ClassBackend, row, x.Name) + clean(shortType(x.Type)))
	case a.Redirect != nil:
		p.line(body + "redirect: " + clean(redirect(*a.Redirect)))
	case a.DirectResponse != nil:
		p.line(body + "direct response: " + directResponse(*a.DirectResponse))
	case a.Kind == model.ActionFilter:
		p.line(body + "filter action: " + clean(a.FilterActionType))
	case a.Kind == model.ActionNonForwarding:
		p.line(body + "non-forwarding")
	}
}

// redirect writes a redirect's status and the parts it changes.
func redirect(r model.Redirect) string {
	parts := []string{fmt.Sprint(r.ResponseCode)}
	add := func(label, v string) {
		if v != "" {
			parts = append(parts, label+" "+v)
		}
	}
	add("scheme", r.Scheme)
	add("host", r.Host)
	if r.Port != 0 {
		add("port", fmt.Sprint(r.Port))
	}
	add("path", r.Path)
	add("prefix", r.PrefixRewrite)
	if r.RegexPattern != "" {
		add("regex", r.RegexPattern+" → "+r.RegexSubstitution)
	}
	add("path rewrite", r.PathRewrite)
	if r.StripQuery {
		parts = append(parts, "strip query")
	}
	return strings.Join(parts, " · ")
}

// directResponse writes a direct response's status and its body's size or
// source, never the body.
func directResponse(d model.DirectResponse) string {
	parts := []string{fmt.Sprint(d.Status)}
	switch {
	case d.BodySize != nil:
		parts = append(parts, fmt.Sprintf("body %d bytes", *d.BodySize))
	case d.BodySource == model.BodyFile:
		parts = append(parts, "body from a file")
	case d.BodySource == model.BodyEnv:
		parts = append(parts, "body from an environment variable")
	}
	if d.BodyFormat {
		parts = append(parts, "body format")
	}
	return strings.Join(parts, " · ")
}

var levelWords = map[model.ConfigLevel]string{
	model.LevelRoute:       "route",
	model.LevelVirtualHost: "vhost",
	model.LevelRouteConfig: "route config",
}

// describeState says why an entry runs or not, e.g. "enabled by route", and
// where an ECDS filter's config comes from.
func describeState(e model.ChainEntry) string {
	switch e.ConfigSource {
	case model.ConfigECDS:
		return state(e) + " · ECDS"
	case model.ConfigECDSDefault:
		return state(e) + " · ECDS default config"
	}
	return state(e)
}

func state(e model.ChainEntry) string {
	at := levelWords[e.Level]
	switch e.State {
	case model.FilterUnresolved:
		return "unresolved: no ECDS config in this dump"
	case model.FilterEnabled:
		return "enabled by " + at
	case model.FilterDisabled:
		return "disabled by " + at
	case model.FilterUnknown:
		return "unknown: invalid FilterConfig at " + at
	case model.FilterOn:
		if at != "" {
			return "on · configured by " + at
		}
	}
	return string(e.State)
}

// policy formats a policy and its field, e.g. "TrafficPolicy ns/x (extAuth, inferred)".
func policy(r model.Ref, field string, c model.Certainty) string {
	var notes []string
	if field != "" {
		notes = append(notes, field)
	}
	if c != model.Exact {
		notes = append(notes, c.String())
	}
	if len(notes) == 0 {
		return r.String()
	}
	return r.String() + " (" + strings.Join(notes, ", ") + ")"
}

// suffix names an origin's certainty unless it is exact, which prints bare.
func suffix(c model.Certainty) string {
	if c == model.Exact {
		return ""
	}
	return " (" + c.String() + ")"
}

func listenerOrigin(o model.ListenerOrigin) string {
	var parts []string
	if o.Gateway != nil {
		parts = append(parts, o.Gateway.String())
	}
	switch {
	case o.Section != "":
		parts = append(parts, "listener "+o.Section)
	case o.Port > 0:
		parts = append(parts, fmt.Sprintf("port %d", o.Port))
	}
	return strings.Join(parts, " ") + suffix(o.Certainty)
}

// routeOrigin formats a route's origin, e.g. "HTTPRoute ns/web · rule 1 (api) · match 0".
func routeOrigin(a *model.RouteAnnotation) string {
	if a == nil || len(a.Candidates) == 0 {
		return ""
	}
	if a.Certainty == model.Ambiguous {
		var refs []string
		for _, c := range a.Candidates {
			refs = append(refs, c.Route.String())
		}
		return "ambiguous: " + strings.Join(slices.Compact(refs), ", ")
	}
	o := a.Candidates[0]
	s := o.Route.String()
	if o.RuleIndex >= 0 {
		s += fmt.Sprintf(" · rule %d", o.RuleIndex)
	}
	if o.RuleName != "" {
		s += " (" + o.RuleName + ")"
	}
	if o.MatchIndex >= 0 {
		s += fmt.Sprintf(" · match %d", o.MatchIndex)
	}
	if o.IndexCertainty == model.Inferred {
		s += " (partly from name)"
	}
	// "chosen from known HTTPRoutes" already says the reading was inferred.
	if a.NarrowedByKnown {
		return s + " (chosen from known HTTPRoutes)"
	}
	return s + suffix(a.Certainty)
}

// formatMatch writes a route match compactly. Several conditions are joined
// with " + "; a route without conditions matches everything and prints "/".
func formatMatch(m model.RouteMatch) string {
	var parts []string
	if m.Prefix != "" {
		parts = append(parts, m.Prefix+" (prefix)")
	}
	if m.PathSeparatedPrefix != "" {
		parts = append(parts, m.PathSeparatedPrefix+" (path-prefix)")
	}
	if m.Path != "" {
		parts = append(parts, m.Path+" (exact)")
	}
	if m.Regex != "" {
		parts = append(parts, m.Regex+" (regex)")
	}
	if m.Connect {
		parts = append(parts, "CONNECT")
	}
	if m.CaseSensitive != nil && !*m.CaseSensitive {
		parts = append(parts, "case-insensitive")
	}
	for _, h := range m.Headers {
		var mods []string
		if h.IgnoreCase {
			mods = append(mods, "ignore case")
		}
		if h.MissingAsEmpty {
			mods = append(mods, "missing as empty")
		}
		s := "header(" + condition(h.Name, h.Value, h.Kind, h.Regex, mods) + ")"
		if h.Invert {
			s = "not " + s
		}
		parts = append(parts, s)
	}
	for _, q := range m.QueryParams {
		var mods []string
		if q.IgnoreCase {
			mods = append(mods, "ignore case")
		}
		parts = append(parts, "query("+condition(q.Name, q.Value, q.Kind, q.Regex, mods)+")")
	}
	if len(m.Unparsed) > 0 {
		parts = append(parts, "unparsed: "+strings.Join(m.Unparsed, ", "))
	}
	if len(parts) == 0 {
		return "/"
	}
	return strings.Join(parts, " + ")
}

// condition writes one header or query condition, e.g. "x=a", "x~a+",
// "x prefix a" or "x present", then its modifiers. An empty kind is exact,
// or regex when regex is set (snapshots built by hand).
func condition(name, value string, kind model.MatchKind, regex bool, mods []string) string {
	var s string
	switch {
	case kind == model.MatchRegex || kind == "" && regex:
		s = name + "~" + value
	case kind == model.MatchExact || kind == "":
		s = name + "=" + value
	case kind == model.MatchPresent || kind == model.MatchAbsent:
		s = name + " " + string(kind)
	case kind == model.MatchRange:
		s = name + " in " + value
	default:
		s = name + " " + string(kind) + " " + value
	}
	if len(mods) > 0 {
		s += ", " + strings.Join(mods, ", ")
	}
	return s
}

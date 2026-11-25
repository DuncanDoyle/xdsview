// Package render writes a [model.Snapshot] as a deterministic plain-text
// tree: listeners, virtual hosts and routes, with each route's effective filter
// chain. It uses only the standard library and the pkg/envoy core, so CLIs and
// UIs can print the same view. Styled or interactive callers add colour,
// a cursor or extra lines through [Options.Style] and [Options.Extra].
package render

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// Options controls what [Text] prints.
type Options struct {
	Header        string // first line, supplied by the caller (gateway, pod, capture time, file)
	ResponseOrder bool   // also print each route's response order
	AllFilters    bool   // list filters that don't run inline; default: one "not run:" line

	// Generic hooks for styled or interactive callers; nil gives plain text.
	Style      func(c Class, ref *FilterRef, s string) string // ref is set on filter rows
	Extra      func(ref FilterRef) []string                   // lines printed under a filter row
	RouteExtra func(ref RouteRef) []string                    // lines printed under a route line, before its filters
}

// RouteRef identifies one route as printed: indexes into the snapshot's
// listeners, the listener's filter chains, and the HCM's virtual hosts and routes.
type RouteRef struct {
	ListenerIdx    int
	FilterChainIdx int
	VirtualHostIdx int
	RouteIdx       int
}

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

// FilterRef identifies one HCM filter as printed under one route. FilterIdx
// indexes the HCM's HTTP filters; the other fields index the snapshot.
type FilterRef struct {
	ListenerIdx    int
	FilterChainIdx int
	VirtualHostIdx int
	RouteIdx       int
	FilterIdx      int
}

const (
	branch = "├─ "
	last   = "└─ "
	pipe   = "│  "
	space  = "   "
)

// Text writes snap to w. Routes without a chain get one from [chain.Effective]
// with no mapper. Output is LF-terminated with no trailing spaces, and follows
// the snapshot's order everywhere.
func Text(w io.Writer, snap *model.Snapshot, opts Options) error {
	p := printer{opts: opts}
	if opts.Header != "" {
		p.line(clean(opts.Header))
	}
	if len(snap.Listeners) == 0 {
		p.line(p.style(ClassWarning, nil, "no listeners"))
	}
	for i, l := range snap.Listeners {
		p.ref = FilterRef{ListenerIdx: i}
		p.listener(l)
	}
	_, err := io.WriteString(w, p.b.String())
	return err
}

type printer struct {
	b    strings.Builder
	opts Options
	ref  FilterRef
}

func (p *printer) line(s string) {
	p.b.WriteString(strings.TrimRight(s, " \t"))
	p.b.WriteByte('\n')
}

// style cleans s, which comes from the dump, then lets the caller style it.
func (p *printer) style(c Class, ref *FilterRef, s string) string {
	s = clean(s)
	if p.opts.Style == nil || s == "" {
		return s
	}
	return p.opts.Style(c, ref, s)
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
	parts := []string{p.style(ClassListener, nil, name)}
	if l.Address != "" {
		parts[0] += " " + address(l.Address)
	}
	s := l.State
	if s.Status != "" {
		c := ClassWarning
		if s.Status == model.ListenerActive {
			c = ClassListener
		}
		parts = append(parts, p.style(c, nil, string(s.Status)))
	}
	if s.Updating {
		parts = append(parts, p.style(ClassWarning, nil, "updating"))
	}
	if s.Draining {
		parts = append(parts, p.style(ClassWarning, nil, "draining"))
	}
	if o := l.Origin; o != nil {
		parts = append(parts, p.style(ClassOrigin, nil, listenerOrigin(*o)))
	}
	if e := s.Error; e != nil {
		msg := "error: " + e.Details
		if e.LastAttempt != "" {
			msg += " (" + e.LastAttempt + ")"
		}
		parts = append(parts, p.style(ClassWarning, nil, msg))
	}
	p.line(strings.Join(parts, " · "))

	// One plain filter chain is the common case: skip its level.
	if len(l.FilterChains) == 1 && l.FilterChains[0].TLS == nil {
		p.ref.FilterChainIdx = 0
		p.chainBody(l.FilterChains[0], "")
		return
	}
	for i, fc := range l.FilterChains {
		p.ref.FilterChainIdx = i
		prefix, indent := node("", i == len(l.FilterChains)-1)
		label := "chain " + clean(fc.Name)
		if fc.TLS != nil && len(fc.TLS.SNIHosts) > 0 {
			label += " · TLS " + clean(strings.Join(fc.TLS.SNIHosts, ", "))
		}
		p.line(prefix + label)
		p.chainBody(fc, indent)
	}
}

func (p *printer) chainBody(fc model.NetworkFilterChain, indent string) {
	hcm := fc.HCM
	if hcm == nil {
		prefix, _ := node(indent, true)
		p.line(prefix + p.style(ClassWarning, nil, "no HTTP connection manager"))
		return
	}
	if hcm.RouteConfig == nil {
		prefix, body := node(indent, true)
		status := " not found"
		if hcm.RouteConfigUse.State == model.RouteConfigAmbiguous {
			status = " ambiguous: several route configs have this name"
		}
		p.line(prefix + p.style(ClassWarning, nil, "route config "+hcm.RouteConfigName+status))
		p.filters(chain.Effective(*hcm, model.VirtualHost{}, model.Route{}), body)
		return
	}
	vhs := hcm.RouteConfig.VirtualHosts
	for i, vh := range vhs {
		p.ref.VirtualHostIdx = i
		prefix, vhIndent := node(indent, i == len(vhs)-1)
		line := "vhost " + p.style(ClassVirtualHost, nil, vh.Name)
		if len(vh.Domains) > 0 {
			line += " " + p.style(ClassOrigin, nil, "["+strings.Join(vh.Domains, ", ")+"]")
		}
		if o := vh.Origin; o != nil {
			line += " · " + p.style(ClassOrigin, nil, listenerOrigin(model.ListenerOrigin{Certainty: o.Certainty, Port: o.Listener.Port, Section: o.Listener.Section}))
		}
		p.line(prefix + line)
		for j, r := range vh.Routes {
			p.ref.RouteIdx = j
			rPrefix, rIndent := node(vhIndent, j == len(vh.Routes)-1)
			p.route(*hcm, vh, r, rPrefix, rIndent)
		}
	}
}

func (p *printer) route(hcm model.HCMConfig, vh model.VirtualHost, r model.Route, prefix, body string) {
	line := "route " + p.style(ClassRoute, nil, formatMatch(r.Match))
	if o := routeOrigin(r.Origin); o != "" {
		line += " · " + p.style(ClassOrigin, nil, o)
	}
	p.line(prefix + line)
	if p.opts.RouteExtra != nil {
		ref := RouteRef{p.ref.ListenerIdx, p.ref.FilterChainIdx, p.ref.VirtualHostIdx, p.ref.RouteIdx}
		for _, x := range p.opts.RouteExtra(ref) {
			p.line(body + x)
		}
	}

	c := chain.Effective(hcm, vh, r)
	if r.Chain != nil {
		c = *r.Chain
	}
	p.filters(c, body)
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
		response := func(e model.ChainEntry) bool { return e.Runs && e.Response != model.SideNo }
		p.rows(body, heading("response", c), c, pick(c, response, true), func(e model.ChainEntry) model.Side { return e.Response })
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
		p.line(body + "policies: " + p.style(ClassOrigin, nil, strings.Join(parts, ", ")))
	}
	p.routeActions(r, body)
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

// filters prints the request order and, by default, a "not run:" line.
func (p *printer) filters(c model.EffectiveChain, body string) {
	title := heading("request", c)
	request := func(e model.ChainEntry) model.Side { return e.Request }
	if p.opts.AllFilters {
		p.rows(body, title, c, pick(c, func(model.ChainEntry) bool { return true }, false), request)
		return
	}
	p.rows(body, title, c, pick(c, func(e model.ChainEntry) bool { return e.Runs && e.Request != model.SideNo }, false), request)

	var off, responseOnly []string
	for i, e := range c.Filters {
		ref := p.ref
		ref.FilterIdx = i
		switch {
		case !e.Runs:
			off = append(off, p.style(ClassFilterOff, &ref, e.Name)+" ("+describeState(e)+")")
		case e.Request == model.SideNo && e.Response != model.SideNo:
			responseOnly = append(responseOnly, p.style(ClassFilter, &ref, e.Name))
		}
	}
	if len(off) > 0 {
		p.line(body + "not run: " + strings.Join(off, ", "))
	}
	if len(responseOnly) > 0 && !p.opts.ResponseOrder {
		p.line(body + "response only: " + strings.Join(responseOnly, ", "))
	}
}

// rows prints c's entries at idx as a numbered, aligned list. side marks an
// entry whose side in this order is unknown with "?". A conditional chain
// prints its heading even with no rows, so the baseline label isn't lost.
func (p *printer) rows(body, title string, c model.EffectiveChain, idx []int, side func(model.ChainEntry) model.Side) {
	if len(idx) == 0 && !c.Conditional {
		return
	}
	p.line(body + title)
	nums := make([]string, len(idx))
	numWidth, nameWidth := 0, 0
	for n, i := range idx {
		nums[n] = fmt.Sprint(n + 1)
		if side(c.Filters[i]) == model.SideUnknown {
			nums[n] += "?"
		}
		numWidth = max(numWidth, len(nums[n]))
		nameWidth = max(nameWidth, utf8.RuneCountInString(clean(c.Filters[i].Name)))
	}
	indent := body + "  "
	for n, i := range idx {
		e := c.Filters[i]
		ref := p.ref
		ref.FilterIdx = i
		class := ClassFilter
		if !e.Runs {
			class = ClassFilterOff
		}
		desc := describeState(e)
		for _, ct := range e.Contributions {
			desc += " · " + p.style(ClassOrigin, nil, contribution(ct))
		}
		num := nums[n] + strings.Repeat(" ", numWidth-len(nums[n]))
		pad := strings.Repeat(" ", nameWidth-utf8.RuneCountInString(clean(e.Name))+2)
		p.line(indent + num + " " + p.style(class, &ref, e.Name) + pad + desc)
		if p.opts.Extra != nil {
			for _, x := range p.opts.Extra(ref) {
				p.line(indent + "    " + x)
			}
		}
	}
}

func (p *printer) routeActions(r model.Route, body string) {
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
		p.line(body + "mirror: " + p.style(ClassBackend, nil, strings.Join(r.MirrorClusters, ", ")))
	}
	if rw := r.Rewrite; rw != nil && rw.RegexPattern != "" {
		p.line(body + "rewrite: " + clean(rw.RegexPattern) + " → " + clean(rw.Substitution))
	}
	if rw := r.Rewrite; rw != nil && rw.Prefix != "" {
		p.line(body + "rewrite prefix: " + clean(rw.Prefix))
	}
	if r.Cluster != "" {
		p.line(body + "backend: " + p.style(ClassBackend, nil, r.Cluster))
	}
	if len(r.WeightedClusters) > 0 {
		var parts []string
		for _, wc := range r.WeightedClusters {
			parts = append(parts, p.style(ClassBackend, nil, wc.Name)+fmt.Sprintf(" (%d)", wc.Weight))
		}
		p.line(body + "backends: " + strings.Join(parts, ", "))
	}
}

var levelWords = map[model.ConfigLevel]string{
	model.LevelRoute:       "route",
	model.LevelVirtualHost: "vhost",
	model.LevelRouteConfig: "route config",
}

// describeState says why an entry runs or not, e.g. "enabled by route".
func describeState(e model.ChainEntry) string {
	at := levelWords[e.Level]
	switch e.State {
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

func contribution(c model.Contribution) string {
	if c.Kind == model.ContributionPolicy {
		return policy(c.Ref, c.Field, c.Certainty)
	}
	return c.Ref.String() + suffix(c.Certainty)
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
	for _, h := range m.Headers {
		op := "="
		if h.Regex {
			op = "~"
		}
		parts = append(parts, fmt.Sprintf("header(%s%s%s)", h.Name, op, h.Value))
	}
	for _, q := range m.QueryParams {
		op := "="
		if q.Regex {
			op = "~"
		}
		parts = append(parts, fmt.Sprintf("query(%s%s%s)", q.Name, op, q.Value))
	}
	if len(parts) == 0 {
		return "/"
	}
	return strings.Join(parts, " + ")
}

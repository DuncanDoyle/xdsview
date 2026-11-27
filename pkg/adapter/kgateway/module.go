// Package kgateway is the provenance module for kgateway OSS: it claims where
// listeners, virtual hosts, routes, filters and backends came from, records
// the policies kgateway's merge metadata names at each level, and links them
// to filter slots. It also selects the routes of one HTTPRoute ([Select]) and
// maps bare names ([Names]).
//
// Name formats and metadata shapes are checked against the minors in
// [CheckedMinors]; the policy field table against [PolicyTableVersion].
package kgateway

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// ModuleName is the name of the module [New] returns. It labels claims; don't
// select claims by it, because a downstream module may wrap this one under
// its own name (see [provenance.Claim]).
const ModuleName = "kgateway"

// New returns the kgateway module. It takes no caller context: a consumer
// that knows which Gateway it captured shows that itself.
func New() provenance.Module { return module{} }

type module struct{}

func (module) Name() string { return ModuleName }

// Claims reads snap. Static (bootstrap) listeners and their static or inline
// route configs get no claims: kgateway didn't produce them. Route config a
// static listener fetches, e.g. over RDS, gets the claims a dynamic one does.
func (module) Claims(snap *model.Snapshot) []provenance.Claim {
	e := &emitter{}
	static := map[model.ID]bool{} // sources static listeners use
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if l.Static && fc.HCM != nil && fc.HCM.RouteConfigUse.Source != "" {
				static[fc.HCM.RouteConfigUse.Source] = true
			}
		}
	}
	sources := map[model.ID]model.RouteConfigSource{}
	for _, s := range snap.RouteConfigs {
		sources[s.ID] = s
		if s.Kind == model.SourceStatic || s.Kind == model.SourceInline && static[s.ID] {
			continue
		}
		if ls, ok := listenerFromName(s.Name); ok {
			e.add(s.ID, ls, model.Inferred, nameEvidence(s.ID, "name", nil))
		}
		e.policies(s.ID, provenance.LevelRouteConfig, s.Metadata)
	}
	for _, l := range snap.Listeners {
		if l.Static {
			for _, fc := range l.FilterChains {
				if fc.HCM == nil {
					continue
				}
				if s, ok := sources[fc.HCM.RouteConfigUse.Source]; !ok || s.Kind != model.SourceInline {
					e.routes(*fc.HCM, s, nil) // fetched content; the listener and its slots stay bootstrap
				}
			}
			continue
		}
		if port, ok := strings.CutPrefix(l.Name, "listener~"); ok {
			if p, ok := index(port); ok {
				e.add(l.ID, provenance.ListenerSource{Port: p}, model.Inferred, nameEvidence(l.ID, "name", nil))
			}
		}
		listenerRecords := e.policies(l.ID, provenance.LevelListener, l.Metadata)
		for _, fc := range l.FilterChains {
			if ls, ok := listenerFromName(fc.Name); ok && fc.Name != "" {
				e.add(fc.ID, ls, model.Inferred, nameEvidence(fc.ID, "name", nil))
			}
			if fc.HCM == nil {
				continue
			}
			e.hcm(*fc.HCM, sources[fc.HCM.RouteConfigUse.Source], listenerRecords)
		}
	}
	return e.claims
}

// emitter collects claims and names policy records uniquely.
type emitter struct {
	claims    []provenance.Claim
	records   int
	bySubject map[model.ID][]record
}

func (e *emitter) add(subject model.ID, a provenance.Assertion, c model.Certainty, ev ...provenance.Evidence) {
	e.claims = append(e.claims, provenance.Claim{Subject: subject, Assertion: a, Certainty: c, Evidence: ev})
}

func nameEvidence(subject model.ID, field string, object *model.Ref) provenance.Evidence {
	return provenance.Evidence{Kind: provenance.EvidenceName, Subject: subject, Field: field, Object: object}
}

// record is a policy record the module emitted, for filter links.
type record struct {
	ref     string
	subject model.ID
	key     string // the merge metadata key it was read from
	policy  model.Ref
	field   string
}

// hcm claims an HCM's filter slots, virtual hosts and routes, and links the
// policy records that apply to each route.
func (e *emitter) hcm(hcm model.HCMConfig, src model.RouteConfigSource, listenerRecords []record) {
	for _, list := range [][]model.HTTPFilter{hcm.HTTPFilters, hcm.UpstreamFilters} {
		for _, f := range list {
			e.extension(f)
		}
	}
	e.routes(hcm, src, listenerRecords)
}

// routes claims the virtual hosts and routes of hcm's route config, and links
// the policy records that apply to each route.
func (e *emitter) routes(hcm model.HCMConfig, src model.RouteConfigSource, listenerRecords []record) {
	rc := hcm.RouteConfig
	if rc == nil {
		return
	}
	srcRecords := e.bySubject[src.ID] // emitted once, with the sources
	for _, vh := range rc.VirtualHosts {
		e.vhost(vh)
		vhRecords := e.policies(vh.ID, provenance.LevelVirtualHost, vh.Metadata)
		for _, r := range vh.Routes {
			e.route(vh.Name, r)
			routeRecords := e.policies(r.ID, provenance.LevelRoute, r.Metadata)
			for _, recs := range [][]record{routeRecords, vhRecords, srcRecords, listenerRecords} {
				for _, rec := range recs {
					e.link(hcm, rc, vh, r, rec)
				}
			}
		}
	}
}

// extension claims what a filter slot is: its kind from its type, and the
// GatewayExtension or helper role its name gives.
func (e *emitter) extension(f model.HTTPFilter) {
	a := provenance.ExtensionSource{FilterKind: filterKind(f.Type)}
	var ev []provenance.Evidence
	if a.FilterKind != "" {
		ev = append(ev, provenance.Evidence{Kind: provenance.EvidenceStructure, Subject: f.ID, Field: "type"})
	}
	parts := strings.Split(f.Name, "/")
	switch {
	case len(parts) == 2 && parts[0] == "global_disable":
		a.Helper = true
		ev = append(ev, nameEvidence(f.ID, "name", nil))
	case len(parts) == 3 && slices.Contains(extensionFilters, parts[0]):
		a.Extension = &model.Ref{Group: kgatewayGroup, Kind: "GatewayExtension", Namespace: parts[1], Name: parts[2]}
		ev = append(ev, nameEvidence(f.ID, "name", a.Extension))
	}
	if len(ev) > 0 {
		e.add(f.ID, a, model.Inferred, ev...)
	}
}

// vhost claims the listener and hostname a "<listener>~<hostname>" name
// carries; the hostname's dots became "_", and Gateway API hostnames have no "_".
func (e *emitter) vhost(vh model.VirtualHost) {
	i := strings.LastIndex(vh.Name, "~")
	if i < 0 {
		return
	}
	ls, ok := listenerFromName(vh.Name[:i])
	if !ok {
		return
	}
	e.add(vh.ID, ls, model.Inferred, nameEvidence(vh.ID, "name", nil))
	e.add(vh.ID, provenance.HostSource{Hostname: strings.ReplaceAll(vh.Name[i+1:], "_", ".")}, model.Inferred, nameEvidence(vh.ID, "name", nil))
}

// route claims a route's HTTPRoute readings and its backends. vhost is the
// name of the route's virtual host.
func (e *emitter) route(vhost string, r model.Route) {
	if o, fromName, ok := routeFromMetadata(vhost, r); ok {
		ev := []provenance.Evidence{{Kind: provenance.EvidenceMetadata, Subject: r.ID, Field: "metadata.filter_metadata." + routeSourceKey, Object: &o.Route}}
		if fromName {
			ev = append(ev, nameEvidence(r.ID, "name", &o.Route))
		}
		o.Exhaustive = true // the metadata names the route
		e.add(r.ID, o, model.Exact, ev...)
	} else {
		// An anchored name's parser enumerates every split; an unanchored one may miss some.
		readings, anchored := parseRouteName(vhost, r.Name)
		for _, o := range readings {
			o.Incomplete, o.Exhaustive = !anchored, anchored
			e.add(r.ID, o, model.Inferred, nameEvidence(r.ID, "name", &o.Route))
		}
	}
	if b, port, ok := clusterBackend(r.Cluster); ok {
		e.add(r.ID, provenance.BackendSource{Backend: b, Port: port}, model.Inferred, nameEvidence(r.ID, "cluster", &b))
	}
	for _, w := range r.WeightedClusters {
		if b, port, ok := clusterBackend(w.Name); ok {
			e.add(w.ID, provenance.BackendSource{Backend: b, Port: port}, model.Inferred, nameEvidence(w.ID, "name", &b))
		}
	}
	for _, m := range r.MirrorClusters {
		if b, port, ok := clusterBackend(m); ok {
			e.add(r.ID, provenance.BackendSource{Backend: b, Port: port, Mirror: m}, model.Inferred, nameEvidence(r.ID, "mirrorClusters", &b))
		}
	}
}

// policies emits a record for every policy ID in metadata's "merge.<Kind>.<group>"
// entries, which map a policy field to "<group>/<Kind>/<ns>/<name>" IDs. Any
// Kind is read, so a downstream policy Kind appears without its own module.
// Each record is exact: the metadata names that ID for that field. An ID whose
// group or kind differs from its key's, or that isn't a valid Kubernetes
// name, is skipped. Records are sorted by policy group, kind, namespace and
// name, then field.
func (e *emitter) policies(subject model.ID, level provenance.Level, md map[string]any) []record {
	var out []record
	for _, key := range slices.Sorted(maps.Keys(md)) {
		v := md[key]
		gk, ok := strings.CutPrefix(key, mergeKeyPrefix)
		if !ok {
			continue
		}
		kind, group, _ := strings.Cut(gk, ".")
		fields, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for field, ids := range fields {
			list, _ := ids.([]any)
			for _, id := range list {
				s, _ := id.(string)
				p := strings.Split(s, "/")
				if len(p) != 4 || p[0] != group || p[1] != kind || !validGroupKind(group, kind) || !validDNSLabel(p[2]) || !validDNSSubdomain(p[3]) {
					continue
				}
				out = append(out, record{subject: subject, key: key, policy: model.Ref{Group: p[0], Kind: p[1], Namespace: p[2], Name: p[3]}, field: field})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b record) int {
		return cmp.Or(strings.Compare(a.policy.Group, b.policy.Group), strings.Compare(a.policy.Kind, b.policy.Kind),
			strings.Compare(a.policy.Namespace, b.policy.Namespace), strings.Compare(a.policy.Name, b.policy.Name),
			strings.Compare(a.field, b.field))
	})
	for i := range out {
		e.records++
		out[i].ref = fmt.Sprintf("policy/%d", e.records)
		e.claims = append(e.claims, provenance.Claim{
			Ref: out[i].ref, Subject: subject, Certainty: model.Exact,
			Assertion: provenance.PolicyRecord{Policy: out[i].policy, Field: out[i].field, Level: level},
			Evidence: []provenance.Evidence{{Kind: provenance.EvidenceMetadata, Subject: subject,
				Field: "metadata.filter_metadata." + out[i].key, Object: &out[i].policy}},
		})
	}
	if e.bySubject == nil {
		e.bySubject = map[model.ID][]record{}
	}
	e.bySubject[subject] = append(e.bySubject[subject], out...)
	return out
}

// link links rec to the one HTTP filter slot of hcm that has per-filter config
// for r and that rec's field configures (design §5 rule 4). With none, or
// several, the record stays unplaced.
func (e *emitter) link(hcm model.HCMConfig, rc *model.RouteConfig, vh model.VirtualHost, r model.Route, rec record) {
	var match []model.HTTPFilter
	for _, f := range hcm.HTTPFilters {
		if configuredFor(f.Name, rc, vh, r) && configures(rec.policy, rec.field, f) {
			match = append(match, f)
		}
	}
	if len(match) != 1 {
		return
	}
	slot := match[0].ID
	policy := rec.policy
	e.add(r.ID, provenance.FilterLink{Record: rec.ref, Slot: slot}, model.Inferred,
		provenance.Evidence{Kind: provenance.EvidenceMetadata, Subject: rec.subject, Field: "metadata.filter_metadata." + rec.key, Object: &policy},
		provenance.Evidence{Kind: provenance.EvidenceTable, Subject: slot, Field: "policy field table " + PolicyTableVersion + ": " + rec.field})
}

// configuredFor reports whether a route has per-filter config for the named
// filter: at route, vhost or route config level, or on a weighted backend.
func configuredFor(name string, rc *model.RouteConfig, vh model.VirtualHost, r model.Route) bool {
	for _, pfc := range []map[string]any{r.TypedPerFilterConfig, vh.TypedPerFilterConfig, rc.TypedPerFilterConfig} {
		if _, ok := pfc[name]; ok {
			return true
		}
	}
	for _, w := range r.WeightedClusters {
		if _, ok := w.TypedPerFilterConfig[name]; ok {
			return true
		}
	}
	return false
}

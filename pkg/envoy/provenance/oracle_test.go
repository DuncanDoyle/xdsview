package provenance_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// TestDependencies_Oracle checks, for every corpus dump and fixture, that each
// item krp's JSON output shows for a listener, virtual host, route or route
// config source block maps to an input of Dependencies(block). It walks the
// JSON, not the index, so an input class the index omits fails here.
func TestDependencies_Oracle(t *testing.T) {
	for _, path := range dumps(t) {
		name, _ := filepath.Rel("../../../testdata", filepath.Dir(path))
		snap := parseFile(t, path)
		ix := compose(t, snap, kgateway.New())
		o := newOracle(t, snap)
		missing := map[string]bool{}
		for _, block := range o.blocks {
			inputs := provenance.Inputs(ix, model.ID(block))
			for _, it := range o.items {
				if o.shows(block, it) && !o.matches(block, it, inputs) {
					missing[block+": "+it.kind+" "+it.subject+" "+it.name] = true
				}
			}
		}
		for _, m := range slices.Sorted(func(yield func(string) bool) {
			for k := range missing {
				if !yield(k) {
					return
				}
			}
		}) {
			t.Errorf("%s: %s is shown but is no input", name, m)
		}
	}
}

// oItem is one displayed thing that needs attribution.
type oItem struct {
	subject string // the occurrence the input's gap names
	kind    string // the assertion kind that attributes it
	name    string // policyRecord: the per-filter config key; backendSource: a mirror cluster
	at      string // the occurrence that shows it
}

// oracle is the JSON output read as a tree of occurrences and the items on it.
type oracle struct {
	items     []oItem
	blocks    []string
	parent    map[string]string   // occurrence → the occurrence enclosing it
	slots     map[string][]string // HCM → its downstream and upstream slots
	slotName  map[string]string
	down      map[string]bool     // downstream slots
	useSource map[string]string   // HCM → the source its use resolves to
	srcKind   map[string]string   // source → its kind
	reports   map[string][]string // static source → the inline sources reporting it
}

func newOracle(t *testing.T, snap *model.Snapshot) *oracle {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"snapshot": snap, "chains": chain.Effective(snap)})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	o := &oracle{parent: map[string]string{}, slots: map[string][]string{}, slotName: map[string]string{}, down: map[string]bool{},
		useSource: map[string]string{}, srcKind: map[string]string{}, reports: map[string][]string{}}
	s := obj(doc["snapshot"])
	for _, rc := range list(s["routeConfigs"]) {
		m := obj(rc)
		id := str(m["id"])
		o.blocks = append(o.blocks, id)
		o.srcKind[id] = str(m["kind"])
		if r := str(m["reported"]); r != "" {
			o.reports[r] = append(o.reports[r], id)
		}
		o.entries(m, id, id)
	}
	for _, l := range list(s["listeners"]) {
		lm := obj(l)
		lid := str(lm["id"])
		o.blocks = append(o.blocks, lid)
		o.add(lid, provenance.KindGatewaySource, "", lid)
		o.entries(lm, lid, lid)
		for _, c := range list(lm["filterChains"]) {
			cm := obj(c)
			cid := str(cm["id"])
			o.parent[cid] = lid
			hm := obj(cm["hcm"])
			hid := str(hm["id"])
			for _, nf := range list(cm["networkFilters"]) {
				if id := str(obj(nf)["id"]); id != hid {
					o.add(id, provenance.KindExtensionSource, "", cid)
				}
			}
			if hm == nil {
				continue
			}
			o.parent[hid] = cid
			for _, key := range []string{"httpFilters", "upstreamFilters"} {
				for _, f := range list(hm[key]) {
					fm := obj(f)
					fid := str(fm["id"])
					o.slots[hid] = append(o.slots[hid], fid)
					o.slotName[fid] = str(fm["name"])
					o.down[fid] = key == "httpFilters"
					if !router(str(fm["type"])) {
						o.add(fid, provenance.KindExtensionSource, "", hid)
					}
				}
			}
			use := obj(hm["routeConfigUse"])
			o.parent[str(use["id"])] = hid
			src := str(use["source"])
			o.useSource[hid] = src
			rcm := obj(hm["routeConfig"])
			if rcm == nil {
				continue
			}
			o.entries(rcm, src, hid)
			for _, vh := range list(rcm["virtualHosts"]) {
				vm := obj(vh)
				vid := str(vm["id"])
				o.parent[vid] = hid
				o.blocks = append(o.blocks, vid)
				o.add(vid, provenance.KindHostSource, "", vid)
				o.entries(vm, vid, vid)
				for _, r := range list(vm["routes"]) {
					o.route(obj(r), vid)
				}
			}
		}
	}
	for at, ch := range obj(doc["chains"]) {
		for _, f := range list(obj(ch)["filters"]) {
			if fm := obj(f); !router(str(fm["type"])) {
				o.add(str(fm["slot"]), provenance.KindExtensionSource, "", at)
			}
		}
	}
	return o
}

// route reads a route: its own items and its weighted backends'.
func (o *oracle) route(rm map[string]any, vid string) {
	rid := str(rm["id"])
	o.parent[rid] = vid
	o.blocks = append(o.blocks, rid)
	o.add(rid, provenance.KindRouteSource, "", rid)
	o.entries(rm, rid, rid)
	am := obj(rm["action"])
	if str(rm["cluster"]) != "" || str(am["clusterHeader"]) != "" || str(am["clusterSpecifierPlugin"]) != "" || am["inlineClusterSpecifierPlugin"] != nil {
		o.add(rid, provenance.KindBackendSource, "", rid)
	}
	for _, mc := range list(rm["mirrorClusters"]) {
		o.add(rid, provenance.KindBackendSource, str(mc), rid)
	}
	for _, w := range list(rm["weightedClusters"]) {
		wm := obj(w)
		wid := str(wm["id"])
		o.parent[wid] = rid
		if str(wm["name"]) != "" {
			o.add(wid, provenance.KindBackendSource, "", wid)
		}
		o.entries(wm, wid, wid)
	}
}

func (o *oracle) add(subject, kind, name, at string) {
	o.items = append(o.items, oItem{subject: subject, kind: kind, name: name, at: at})
}

// entries adds an item per per-filter config entry on m.
func (o *oracle) entries(m map[string]any, subject, at string) {
	for k := range obj(m["typedPerFilterConfig"]) {
		o.add(subject, provenance.KindPolicyRecord, k, at)
	}
}

// ancestors returns id and the occurrences enclosing it.
func (o *oracle) ancestors(id string) []string {
	var out []string
	for ok := true; ok; id, ok = o.parent[id] {
		out = append(out, id)
	}
	return out
}

// serving returns the HCMs whose use resolves to source, or to an inline
// source reporting it.
func (o *oracle) serving(source string) []string {
	var out []string
	for h, s := range o.useSource {
		if s == source || slices.Contains(o.reports[source], s) {
			out = append(out, h)
		}
	}
	return out
}

// shows reports whether block shows item.
func (o *oracle) shows(block string, it oItem) bool {
	if _, ok := o.srcKind[block]; ok {
		if it.at == block {
			return true
		}
		for _, h := range o.serving(block) {
			if it.at == h {
				return true
			}
			// An inline source sits in its listener, so it shows the listener's items.
			if o.srcKind[o.useSource[h]] == string(model.SourceInline) && slices.Contains(o.ancestors(h), it.at) {
				return true
			}
		}
		return false
	}
	return slices.Contains(o.ancestors(block), it.at) || slices.Contains(o.ancestors(it.at), block)
}

// matches reports whether an input of block attributes it.
func (o *oracle) matches(block string, it oItem, inputs []provenance.Gap) bool {
	subjects := append([]string{it.subject}, o.reports[it.subject]...)
	if it.kind != provenance.KindPolicyRecord {
		return slices.ContainsFunc(inputs, func(g provenance.Gap) bool {
			return slices.Contains(subjects, string(g.Subject)) && g.Kind == it.kind && g.Cluster == it.name
		})
	}
	// A per-filter entry is matched to the slots of that name it can configure.
	var hcms []string
	if _, ok := o.srcKind[it.at]; ok {
		hcms = o.serving(it.at)
	}
	for _, a := range o.ancestors(it.at) {
		if _, ok := o.slots[a]; ok {
			hcms = append(hcms, a)
		}
	}
	var downs, ups []string
	for _, h := range hcms {
		for _, s := range o.slots[h] {
			if o.slotName[s] != it.name {
				continue
			}
			if o.down[s] {
				downs = append(downs, s)
			} else {
				ups = append(ups, s)
			}
		}
	}
	want := downs
	if len(want) == 0 {
		want = ups
	}
	if len(want) == 0 {
		want = []string{""}
	}
	return slices.ContainsFunc(inputs, func(g provenance.Gap) bool {
		return slices.Contains(subjects, string(g.Subject)) && g.Kind == provenance.KindPolicyRecord && slices.Contains(want, string(g.Slot))
	})
}

func router(typ string) bool { return strings.HasSuffix(typ, ".filters.http.router.v3.Router") }

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }

func list(v any) []any { l, _ := v.([]any); return l }

func str(v any) string { s, _ := v.(string); return s }

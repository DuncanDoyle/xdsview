package kgateway_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// routePayload is a consumer's own route payload (kgwctl-shaped), built from
// exported API only: no model type is embedded.
type routePayload struct {
	ID      model.ID     `json:"id"`
	Match   string       `json:"match"`
	Backend string       `json:"backend"`
	Source  string       `json:"source"`
	Chain   []chainEntry `json:"chain"`
}

type chainEntry struct {
	Filter   string   `json:"filter"`
	Runs     string   `json:"runs"`
	Policies []string `json:"policies,omitempty"`
}

// Example_routePayload builds a route payload for one HTTPRoute: select its
// routes, then join each route's match, backend, origin and chain with the
// filter links, by occurrence ID (9.3 test 12, E15).
func Example_routePayload() {
	f, _ := os.Open("../../../testdata/scenarios/03-multi-policy/envoy/kgateway-v2.5.0/config_dump.json")
	defer f.Close()
	snap, _ := parser.Parse(f, parser.Options{})
	chains := chain.Effective(snap)
	ix, _ := provenance.Compose(snap, kgateway.New())

	web := model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com"}
	sel := kgateway.Select(ix, snap, kgateway.Query{Route: web, Known: []model.Ref{web}})
	// This consumer also keeps routes that Known narrowed to one reading.
	ids := sel.Routes
	for _, n := range sel.Narrowed {
		if len(n.Kept) == 1 {
			ids = append(ids, n.Route)
		}
	}
	for _, id := range ids {
		r, _ := snap.Route(id)
		p := routePayload{ID: id, Match: r.Match.Prefix, Backend: r.Cluster}
		if res := provenance.Resolve[provenance.RouteSource](ix, id); len(res.Candidates) == 1 {
			p.Source = fmt.Sprintf("%s rule %d (%s)", res.Candidates[0].Route, res.Candidates[0].RuleIndex, res.Certainty)
		}
		for _, n := range sel.Narrowed {
			if n.Route == id {
				o := n.Kept[0].Assertion.(provenance.RouteSource)
				p.Source = fmt.Sprintf("%s rule %d (narrowed)", o.Route, o.RuleIndex)
			}
		}
		for _, e := range chains[id].Filters {
			ce := chainEntry{Filter: e.Name, Runs: string(e.Runs)}
			for _, c := range ix.About(id) {
				if l, ok := c.Assertion.(provenance.FilterLink); ok && l.Slot == e.Slot {
					rec, _ := ix.Record(c)
					pr := rec.Assertion.(provenance.PolicyRecord)
					ce.Policies = append(ce.Policies, pr.Policy.String()+" "+pr.Field)
				}
			}
			p.Chain = append(p.Chain, ce)
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(b))
	}
	// Output:
	// {
	//   "id": "L0/C0/N0/RC/V0/R0",
	//   "match": "/",
	//   "backend": "kube_httpbin_httpbin_8000",
	//   "source": "HTTPRoute kgateway-system/api-example-com rule 0 (narrowed)",
	//   "chain": [
	//     {
	//       "filter": "global_disable/ext_auth",
	//       "runs": "no"
	//     },
	//     {
	//       "filter": "envoy.filters.http.cors",
	//       "runs": "yes",
	//       "policies": [
	//         "TrafficPolicy kgateway-system/cors cors"
	//       ]
	//     },
	//     {
	//       "filter": "ext_auth/kgateway-system/ext-authz",
	//       "runs": "yes",
	//       "policies": [
	//         "TrafficPolicy kgateway-system/ext-auth extAuth"
	//       ]
	//     },
	//     {
	//       "filter": "ratelimit/kgateway-system/ratelimit",
	//       "runs": "yes",
	//       "policies": [
	//         "TrafficPolicy kgateway-system/rate-limit rateLimit.global"
	//       ]
	//     },
	//     {
	//       "filter": "envoy.filters.http.router",
	//       "runs": "yes"
	//     }
	//   ]
	// }
}

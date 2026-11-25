package kgateway_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
)

// benchDump is a kgateway-style dump with vhosts × routes routes. Every route
// has route_source and merge metadata and enables ext_authz per route.
func benchDump(vhosts, routes int) []byte {
	var b strings.Builder
	b.WriteString(`{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "dynamic_listeners": [
 {"name": "listener~80", "active_state": {"listener": {"name": "listener~80", "filter_chains": [{"name": "listener~80", "filters": [
  {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~80"}, "http_filters": [
   {"name": "ext_auth/ns/authz", "disabled": true, "typed_config": {"@type": "` + authzType + `"}},
   {"name": "envoy.filters.http.cors", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors"}},
   {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}}]},
 {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [{"route_config": {"name": "listener~80", "virtual_hosts": [`)
	for v := range vhosts {
		if v > 0 {
			b.WriteString(",")
		}
		vh := fmt.Sprintf("listener~80~h%d_example_com", v)
		fmt.Fprintf(&b, `{"name": %q, "domains": ["h%d.example.com"], "routes": [`, vh, v)
		for r := range routes {
			if r > 0 {
				b.WriteString(",")
			}
			name := fmt.Sprintf("r%d", r)
			fmt.Fprintf(&b, `{"name": "%s-route-%d-httproute-%s-ns-0-0-matcher-0", "match": {"prefix": "/%s"}, "route": {"cluster": "kube_ns_%s_80"},
 "typed_per_filter_config": {"ext_auth/ns/authz": {}},
 "metadata": {"filter_metadata": {"dev.kgateway.route_source": {"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "ns", "name": "%s", "rule": "_rule-0", "match": "_match-0"},
  "merge.TrafficPolicy.gateway.kgateway.dev": {"extAuth": ["gateway.kgateway.dev/TrafficPolicy/ns/%s"]}}}}`, vh, r, name, name, name, name, name)
		}
		b.WriteString("]}")
	}
	b.WriteString("]}}]}]}")
	return []byte(b.String())
}

// The close benchmark (plan §9 close): parse, execution and composition on
// 5,000 routes. Run with -bench . -benchmem.
const benchVhosts, benchRoutes = 50, 100

func BenchmarkParse(b *testing.B) {
	dump := benchDump(benchVhosts, benchRoutes)
	b.SetBytes(int64(len(dump)))
	for b.Loop() {
		if _, err := parser.Parse(bytes.NewReader(dump), parser.Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEffective(b *testing.B) {
	snap, err := parser.Parse(bytes.NewReader(benchDump(benchVhosts, benchRoutes)), parser.Options{})
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if len(chain.Effective(snap)) < benchVhosts*benchRoutes {
			b.Fatal("missing chains")
		}
	}
}

func BenchmarkCompose(b *testing.B) {
	snap, err := parser.Parse(bytes.NewReader(benchDump(benchVhosts, benchRoutes)), parser.Options{})
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		ix, err := provenance.Compose(snap, kgateway.New())
		if err != nil || len(ix.Warnings()) > 0 {
			b.Fatal(err, ix.Warnings())
		}
	}
}

package kgateway_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
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

// staticRDSDump is a dump of n static listeners, each fetching its own
// one-route config over RDS with route_source metadata.
func staticRDSDump(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump", "static_listeners": [`)
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"listener": {"name": "boot%d", "filter_chains": [{"filters": [
 {"name": "envoy.filters.network.http_connection_manager", "typed_config": {"rds": {"route_config_name": "listener~%d"}, "http_filters": [
  {"name": "envoy.filters.http.router", "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}]}}]}]}}`, i, i)
	}
	b.WriteString(`]}, {"@type": "type.googleapis.com/envoy.admin.v3.RoutesConfigDump", "dynamic_route_configs": [`)
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"route_config": {"name": "listener~%d", "virtual_hosts": [{"name": "listener~%d~h_example_com", "domains": ["h.example.com"], "routes": [
 {"name": "r", "match": {"prefix": "/"}, "route": {"cluster": "kube_ns_r%d_80"},
  "metadata": {"filter_metadata": {"dev.kgateway.route_source": {"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "ns", "name": "r%d", "rule": "_rule-0", "match": "_match-0"}}}}]}]}}`, i, i, i, i)
	}
	b.WriteString("]}]}")
	return []byte(b.String())
}

// BenchmarkDependencies composes and checks every route block, as Snellius
// does: on one dynamic listener, and on static listeners fetching over RDS.
func BenchmarkDependencies(b *testing.B) {
	b.Run("dynamic", func(b *testing.B) { benchDependencies(b, benchDump(benchVhosts, benchRoutes)) })
	b.Run("staticRDS", func(b *testing.B) { benchDependencies(b, staticRDSDump(benchVhosts*benchRoutes)) })
}

func benchDependencies(b *testing.B, dump []byte) {
	snap, err := parser.Parse(bytes.NewReader(dump), parser.Options{})
	if err != nil {
		b.Fatal(err)
	}
	var routes []model.ID
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					routes = append(routes, r.ID)
				}
			}
		}
	}
	if len(routes) != benchVhosts*benchRoutes {
		b.Fatalf("%d routes", len(routes))
	}
	for b.Loop() {
		ix, err := provenance.Compose(snap, kgateway.New())
		if err != nil {
			b.Fatal(err)
		}
		for _, r := range routes {
			if len(ix.Dependencies(r).Objects) == 0 {
				b.Fatal("no objects")
			}
		}
	}
}

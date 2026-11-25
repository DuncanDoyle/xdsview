package snaptest

import "github.com/DuncanDoyle/xdsview/pkg/envoy/model"

// Filter types the hand-built snapshots use.
const (
	DynamicModuleType = "type.googleapis.com/envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter"
	RouterType        = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"
	matcherType       = "type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	compositeType     = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite"
	executeActionType = "type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction"
	filterConfigType  = "type.googleapis.com/envoy.config.route.v3.FilterConfig"
)

// P1 is a kgateway snapshot with one route per Phase 8 deferred P1 finding:
//   - R0 (#2): route_source metadata names rule "api.v1"; only the route name
//     gives its index, 0.
//   - R1 (#5): a TrafficPolicy transformation on a Composite whose delegate is
//     the rustformation instance (slot F0).
//   - R2 (#4): a transformation on the rustformation instance (slot F1) whose
//     dynamic_module_config.module loads another library.
//
// IDs are set as the parser sets them.
func P1() *model.Snapshot {
	rustformation := func(module map[string]any) map[string]any {
		lib := map[string]any{"name": "rust_module"}
		if module != nil {
			lib["module"] = module
		}
		return map[string]any{"@type": DynamicModuleType, "filter_name": "rustformation", "dynamic_module_config": lib}
	}
	composite := map[string]any{
		"@type":            matcherType,
		"extension_config": map[string]any{"name": "composite", "typed_config": map[string]any{"@type": compositeType}},
		"xds_matcher": map[string]any{"on_no_match": map[string]any{"action": map[string]any{
			"name": "xform",
			"typed_config": map[string]any{"@type": executeActionType, "typed_config": map[string]any{
				"name": "dynamic_modules/simple_mutations", "typed_config": rustformation(nil)}},
		}}},
	}
	other := rustformation(map[string]any{"local": map[string]any{"filename": "/usr/lib/libother.so"}})
	transformation := func(name string) map[string]any {
		return map[string]any{"merge.TrafficPolicy.gateway.kgateway.dev": map[string]any{
			"transformation": []any{"gateway.kgateway.dev/TrafficPolicy/shop/" + name}}}
	}
	enable := func(filter string) map[string]any {
		return map[string]any{filter: map[string]any{"@type": filterConfigType, "config": map[string]any{}}}
	}
	routes := []model.Route{
		{
			Name: "listener~80~shop_example-route-0-httproute-web-shop-0-0-api.v1-matcher-0", Match: model.RouteMatch{Prefix: "/v1"}, Cluster: "kube_shop_web_8080",
			Metadata: map[string]any{"dev.kgateway.route_source": map[string]any{
				"group": "gateway.networking.k8s.io", "kind": "HTTPRoute", "namespace": "shop", "name": "web", "rule": "api.v1", "match": "_match-0"}},
		},
		{
			Name: "listener~80~shop_example-route-1-httproute-web-shop-1-0-matcher-0", Match: model.RouteMatch{Prefix: "/composite"}, Cluster: "kube_shop_web_8080",
			TypedPerFilterConfig: enable("composite/xform"), Metadata: transformation("composite"),
		},
		{
			Name: "listener~80~shop_example-route-2-httproute-web-shop-2-0-matcher-0", Match: model.RouteMatch{Prefix: "/module"}, Cluster: "kube_shop_web_8080",
			TypedPerFilterConfig: enable("dynamic_modules/simple_mutations"), Metadata: transformation("module"),
		},
	}
	hcm := &model.HCMConfig{
		RouteConfigName: "listener~80",
		HTTPFilters: []model.HTTPFilter{
			{Name: "composite/xform", Type: DynamicModuleType, TypedConfig: composite, Disabled: true},
			{Name: "dynamic_modules/simple_mutations", Type: DynamicModuleType, TypedConfig: other, Disabled: true},
			{Name: "envoy.filters.http.router", Type: RouterType},
		},
		RouteConfigUse: model.RouteConfigUse{Source: "RD0", State: model.RouteConfigFound},
		RouteConfig: &model.RouteConfig{Name: "listener~80", VirtualHosts: []model.VirtualHost{
			{Name: "listener~80~shop_example", Domains: []string{"shop.example"}, Routes: routes},
		}},
	}
	snap := &model.Snapshot{
		Listeners: []model.Listener{{Name: "listener~80", Address: "[::]:80", State: model.ListenerState{Status: model.ListenerActive},
			FilterChains: []model.NetworkFilterChain{{Name: "listener~80", HCM: hcm}}}},
		RouteConfigs: []model.RouteConfigSource{{ID: "RD0", Kind: model.SourceDynamic, Name: "listener~80"}},
	}
	return IDs(snap)
}

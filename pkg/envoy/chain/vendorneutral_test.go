package chain_test

import (
	"reflect"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
)

// xformType is a downstream transformation filter that the core does not know.
const xformType = "type.googleapis.com/example.transformation.v1.Transformation"

// oneFilter is one route that enables filter f per route and carries meta.
func oneFilter(f model.HTTPFilter, meta map[string]any) *model.EnvoySnapshot {
	f.Disabled = true
	rc := &model.RouteConfig{VirtualHosts: []model.VirtualHost{{Routes: []model.Route{{
		TypedPerFilterConfig: map[string]any{f.Name: map[string]any{"@type": filterConfigType, "config": map[string]any{}}},
		Metadata:             meta,
	}}}}}
	return &model.EnvoySnapshot{Listeners: []model.Listener{{FilterChains: []model.NetworkFilterChain{{HCM: &model.HCMConfig{
		HTTPFilters: []model.HTTPFilter{f}, RouteConfig: rc,
	}}}}}}
}

// mergeID is "merge.<kind>.<group>" metadata naming one policy for field.
func mergeID(group, kind, field string) map[string]any {
	return map[string]any{"merge." + kind + "." + group: map[string]any{field: []any{group + "/" + kind + "/ns/p"}}}
}

// xformPlugin is a downstream plugin that claims the vendor transformation type.
type xformPlugin struct{ origin.Mapper }

func (xformPlugin) PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool {
	return p.Field == "transformation" && f.Type == xformType
}

// TestVendorNeutral_PolicyKind: the core links only kgateway's TrafficPolicy.
// Another group or kind stays listed on Route.Policies, unlinked.
func TestVendorNeutral_PolicyKind(t *testing.T) {
	authz := model.HTTPFilter{Name: "ext_auth/ns/authz", Type: extAuthzType}
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	for _, c := range []struct {
		group, kind string
		linked      bool
	}{
		{"gateway.kgateway.dev", "TrafficPolicy", true},
		{"example.com", "TrafficPolicy", false},
		{"gateway.kgateway.dev", "AcmePolicy", false},
	} {
		ref := model.Ref{Group: c.group, Kind: c.kind, Namespace: "ns", Name: "p"}
		r := composed(t, oneFilter(authz, mergeID(c.group, c.kind, "extAuth")), kgw)
		var got []model.Contribution
		for _, ct := range contributionsOf(r)[authz.Name] {
			if ct.Kind == model.ContributionPolicy {
				got = append(got, ct)
			}
		}
		var want []model.Contribution
		if c.linked {
			want = []model.Contribution{{Kind: model.ContributionPolicy, Ref: ref, Field: "extAuth", Certainty: model.Inferred}}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s/%s: policy contributions = %+v, want %+v", c.group, c.kind, got, want)
		}
		if !hasPolicy(r, model.PolicyRef{Policy: ref, Field: "extAuth", Certainty: model.Exact}) {
			t.Errorf("%s/%s: Route.Policies = %+v, want %v listed", c.group, c.kind, r.Policies, ref)
		}
	}
}

// TestVendorNeutral_Transformation: the core does not know the vendor
// transformation type, so its sides are unknown and it gets no policy link.
// A plugin in origin.Chain that claims the type adds the link back.
func TestVendorNeutral_Transformation(t *testing.T) {
	xf := model.HTTPFilter{Name: "transformation", Type: xformType}
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	ref := model.Ref{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "ns", Name: "p"}
	kgw := origin.Kgateway(origin.KgatewayOptions{})

	r := composed(t, oneFilter(xf, meta), kgw)
	e := r.Chain.Filters[0]
	if len(e.Contributions) != 0 {
		t.Errorf("core: contributions = %+v, want none", e.Contributions)
	}
	if e.Request != model.SideUnknown || e.Response != model.SideUnknown {
		t.Errorf("core: sides = %v/%v, want unknown/unknown", e.Request, e.Response)
	}

	r = composed(t, oneFilter(xf, meta), origin.Chain(xformPlugin{kgw}, kgw))
	want := []model.Contribution{{Kind: model.ContributionPolicy, Ref: ref, Field: "transformation", Certainty: model.Inferred}}
	if got := contributionsOf(r)[xf.Name]; !reflect.DeepEqual(got, want) {
		t.Errorf("with plugin: contributions = %+v, want %+v", got, want)
	}
}

// TestVendorNeutral_TransformationNeedsType: the OSS transformation instance
// name alone is not evidence; a different type under it gets no link (phase review #3).
func TestVendorNeutral_TransformationNeedsType(t *testing.T) {
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	for _, typ := range []string{xformType, "type.googleapis.com/example.filters.http.dynamic_modules.v1.Other"} {
		r := composed(t, oneFilter(model.HTTPFilter{Name: "dynamic_modules/simple_mutations", Type: typ}, meta), kgw)
		if got := r.Chain.Filters[0].Contributions; len(got) != 0 {
			t.Errorf("%s: contributions = %+v, want none", typ, got)
		}
	}
}

// TestVendorNeutral_TransformationModule: DynamicModuleFilter is a generic
// loader, so the link also needs the rustformation module (phase fix review 2 #4).
func TestVendorNeutral_TransformationModule(t *testing.T) {
	meta := mergeID("gateway.kgateway.dev", "TrafficPolicy", "transformation")
	kgw := origin.Kgateway(origin.KgatewayOptions{})
	for module, linked := range map[string]bool{"rustformation": true, "other": false} {
		f := model.HTTPFilter{Name: "dynamic_modules/simple_mutations", Type: dynModType, TypedConfig: map[string]any{"filter_name": module}}
		r := composed(t, oneFilter(f, meta), kgw)
		if got := len(r.Chain.Filters[0].Contributions) == 1; got != linked {
			t.Errorf("module %s: contributions %+v, want linked=%v", module, r.Chain.Filters[0].Contributions, linked)
		}
	}
}

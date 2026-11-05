package kube

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/pkg/envoy/model"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func httpRouteObj(ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("gateway.networking.k8s.io/v1")
	u.SetKind("HTTPRoute")
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

func TestListHTTPRoutes(t *testing.T) {
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{httpRouteGVR: "HTTPRouteList"},
		httpRouteObj("kgateway-system", "api-example-com"), httpRouteObj("shop", "web"))
	got, err := listHTTPRoutes(context.Background(), dc)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b model.Ref) int {
		return cmp.Or(strings.Compare(a.Namespace, b.Namespace), strings.Compare(a.Name, b.Name))
	})
	want := []model.Ref{
		{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com"},
		{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

package kube

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DuncanDoyle/krp/pkg/envoy/model"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
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

// TestListHTTPRoutes_Error returns the API error, e.g. RBAC, to the caller.
func TestListHTTPRoutes_Error(t *testing.T) {
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{httpRouteGVR: "HTTPRouteList"})
	dc.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(httpRouteGVR.GroupResource(), "", errors.New("no list permission"))
	})
	if _, err := listHTTPRoutes(context.Background(), dc); !apierrors.IsForbidden(err) {
		t.Errorf("err = %v, want Forbidden", err)
	}
}

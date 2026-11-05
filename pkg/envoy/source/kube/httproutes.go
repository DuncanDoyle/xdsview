package kube

import (
	"context"
	"fmt"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var httpRouteGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}

// HTTPRoutes lists the HTTPRoutes in every namespace, as refs. Pass them as
// origin.KgatewayOptions.Known to resolve route names with several readings.
// It needs "list httproutes" cluster-wide.
func HTTPRoutes(ctx context.Context, opts Options) ([]model.Ref, error) {
	cfg := opts.RESTConfig
	if cfg == nil {
		var err error
		if cfg, err = buildRestConfig(opts.KubeContext); err != nil {
			return nil, err
		}
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	return listHTTPRoutes(ctx, dc)
}

func listHTTPRoutes(ctx context.Context, dc dynamic.Interface) ([]model.Ref, error) {
	list, err := dc.Resource(httpRouteGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing HTTPRoutes: %w", err)
	}
	refs := make([]model.Ref, 0, len(list.Items))
	for _, item := range list.Items {
		refs = append(refs, model.Ref{Group: httpRouteGVR.Group, Kind: "HTTPRoute", Namespace: item.GetNamespace(), Name: item.GetName()})
	}
	return refs, nil
}

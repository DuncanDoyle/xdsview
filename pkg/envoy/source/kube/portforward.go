// Package kube provides [source.Source] implementations that reach a gateway
// proxy's Envoy admin API through a Kubernetes port-forward.
//
// The tunnel opens inside ConfigDump and closes when the returned reader is
// closed, or before ConfigDump returns an error. Callers never manage it.
package kube

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/DuncanDoyle/krp/pkg/envoy/source"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const envoyAdminPort = 19000

// Options selects the cluster to connect to.
type Options struct {
	// RESTConfig is used when set, so callers can reuse the config they already have.
	RESTConfig *rest.Config
	// KubeContext picks a kubeconfig context when RESTConfig is nil. Empty means
	// the current context.
	KubeContext string
}

// podFinder is a function that resolves a pod name to port-forward to.
type podFinder func(ctx context.Context, kc kubernetes.Interface, namespace string) (string, error)

// opener opens a tunnel and returns the admin base URL and a function that closes it.
type opener func(ctx context.Context) (baseURL string, stop func(), err error)

// Gateway returns a [source.Source] that port-forwards to a ready proxy pod of
// the named Gateway, selected by the gateway.networking.k8s.io/gateway-name label.
func Gateway(name, namespace string, opts Options) source.Source {
	return &kubeSource{open: func(ctx context.Context) (string, func(), error) {
		return portForward(ctx, namespace, opts, func(ctx context.Context, kc kubernetes.Interface, namespace string) (string, error) {
			return findGatewayProxyPod(ctx, kc, name, namespace)
		})
	}}
}

// Deployment returns a [source.Source] that port-forwards to a ready pod of the
// named Deployment.
func Deployment(name, namespace string, opts Options) source.Source {
	return &kubeSource{open: func(ctx context.Context) (string, func(), error) {
		return portForward(ctx, namespace, opts, func(ctx context.Context, kc kubernetes.Interface, namespace string) (string, error) {
			return findDeploymentPod(ctx, kc, name, namespace)
		})
	}}
}

type kubeSource struct {
	open opener
}

// ConfigDump opens the tunnel, fetches the dump through it, and ties the
// tunnel's lifetime to the returned reader.
func (k *kubeSource) ConfigDump(ctx context.Context) (io.ReadCloser, error) {
	baseURL, stop, err := k.open(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Envoy admin API: %w", err)
	}
	rc, err := source.Admin(baseURL).ConfigDump(ctx)
	if err != nil {
		stop()
		return nil, fmt.Errorf("fetching config dump: %w", err)
	}
	return &tunnelReader{ReadCloser: rc, stop: stop}, nil
}

// tunnelReader closes the tunnel, once, when the body is closed.
type tunnelReader struct {
	io.ReadCloser
	stop func()
	once sync.Once
}

func (t *tunnelReader) Close() error {
	err := t.ReadCloser.Close()
	t.once.Do(t.stop)
	return err
}

// portForward is the shared implementation: it builds a K8S client, resolves a pod
// via the provided podFinder, and opens a port-forward to the Envoy admin port.
func portForward(ctx context.Context, namespace string, opts Options, finder podFinder) (string, func(), error) {
	cfg := opts.RESTConfig
	if cfg == nil {
		var err error
		if cfg, err = buildRestConfig(opts.KubeContext); err != nil {
			return "", nil, err
		}
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	podName, err := finder(ctx, kc, namespace)
	if err != nil {
		return "", nil, err
	}

	localPort, err := freePort()
	if err != nil {
		return "", nil, fmt.Errorf("finding free local port: %w", err)
	}

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("creating SPDY transport: %w", err)
	}

	url := kc.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	ports := []string{fmt.Sprintf("%d:%d", localPort, envoyAdminPort)}

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return "", nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	go func() {
		errCh <- fw.ForwardPorts()
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		return "", nil, fmt.Errorf("port-forward failed: %w", err)
	case <-ctx.Done():
		close(stopCh)
		return "", nil, ctx.Err()
	}

	return fmt.Sprintf("http://localhost:%d", localPort), func() { close(stopCh) }, nil
}

// buildRestConfig loads the kubeconfig from the default discovery chain
// (KUBECONFIG env var → ~/.kube/config) and optionally overrides the active
// context. An empty kubeContext string leaves the current context unchanged.
func buildRestConfig(kubeContext string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, overrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}
	return cfg, nil
}

// findGatewayProxyPod finds the first ready pod for a kgateway Gateway using the
// standard Gateway API pod label (gateway.networking.k8s.io/gateway-name).
func findGatewayProxyPod(ctx context.Context, kc kubernetes.Interface, gatewayName, namespace string) (string, error) {
	selector := fmt.Sprintf("gateway.networking.k8s.io/gateway-name=%s", gatewayName)
	pods, err := kc.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("listing gateway-proxy pods: %w", err)
	}

	for _, pod := range pods.Items {
		if isPodReady(&pod) {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf(
		"no ready gateway-proxy pod found for Gateway %s/%s (selector: %s)\n"+
			"Hint: ensure the gateway pod is running and you have RBAC permissions for 'get pods' and 'create pods/portforward' in namespace %s",
		namespace, gatewayName, selector, namespace,
	)
}

// findDeploymentPod finds the first ready pod owned by the named Deployment by
// reading the Deployment's pod selector and listing matching pods.
func findDeploymentPod(ctx context.Context, kc kubernetes.Interface, deploymentName, namespace string) (string, error) {
	dep, err := kc.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting deployment %s/%s: %w", namespace, deploymentName, err)
	}

	selector := metav1.FormatLabelSelector(dep.Spec.Selector)

	pods, err := kc.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("listing pods for deployment %s/%s: %w", namespace, deploymentName, err)
	}

	for _, pod := range pods.Items {
		if isPodReady(&pod) {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf(
		"no ready pod found for Deployment %s/%s\n"+
			"Hint: ensure the deployment pods are running and you have RBAC permissions for 'get deployments', 'get pods' and 'create pods/portforward' in namespace %s",
		namespace, deploymentName, namespace,
	)
}

// isPodReady reports whether the pod's Ready condition is True.
// Pods that are running but not yet ready (e.g. during startup) are excluded
// to avoid connecting to an Envoy instance that is still initialising.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// freePort returns a free TCP port on the loopback interface by briefly listening
// on port 0 (OS assigns an ephemeral port) and immediately closing the listener.
// There is a small TOCTOU window between Close and portforward.New, but this is
// acceptable for local dev tooling.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

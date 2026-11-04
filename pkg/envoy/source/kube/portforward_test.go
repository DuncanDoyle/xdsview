package kube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// fakeTunnel stands in for a port-forward: it points at baseURL and counts stops.
type fakeTunnel struct {
	baseURL string
	stops   int
}

func (f *fakeTunnel) source() *kubeSource {
	return &kubeSource{open: func(context.Context) (string, func(), error) {
		return f.baseURL, func() { f.stops++ }, nil
	}}
}

func adminServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"configs": []}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestConfigDump_CloseStopsTunnel(t *testing.T) {
	tun := &fakeTunnel{baseURL: adminServer(t, http.StatusOK).URL}
	rc, err := tun.source().ConfigDump(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatal(err)
	}
	if tun.stops != 0 {
		t.Fatalf("tunnel stopped before Close: stops = %d", tun.stops)
	}
	_ = rc.Close()
	_ = rc.Close()
	if tun.stops != 1 {
		t.Errorf("stops after two Close calls = %d, want 1", tun.stops)
	}
}

func TestConfigDump_FailureStopsTunnel(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		baseURL string
		ctx     context.Context
	}{
		{"request error", closedURL, context.Background()},
		{"non-200", adminServer(t, http.StatusServiceUnavailable).URL, context.Background()},
		{"cancelled ctx", adminServer(t, http.StatusOK).URL, cancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tun := &fakeTunnel{baseURL: tt.baseURL}
			rc, err := tun.source().ConfigDump(tt.ctx)
			if err == nil {
				rc.Close()
				t.Fatal("expected an error")
			}
			if tun.stops != 1 {
				t.Errorf("stops = %d, want 1", tun.stops)
			}
		})
	}
}

func TestConfigDump_OpenError(t *testing.T) {
	src := &kubeSource{open: func(context.Context) (string, func(), error) {
		return "", nil, errors.New("no pod")
	}}
	if _, err := src.ConfigDump(context.Background()); err == nil {
		t.Fatal("expected an error when the tunnel cannot open")
	}
}

func readyPod(name, ns string, labels map[string]string, ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}}},
	}
}

func TestFindGatewayProxyPod(t *testing.T) {
	gw := map[string]string{"gateway.networking.k8s.io/gateway-name": "gw"}
	kc := fake.NewClientset(
		readyPod("gw-starting", "kgateway-system", gw, false),
		readyPod("gw-ready", "kgateway-system", gw, true),
		readyPod("other", "kgateway-system", map[string]string{"gateway.networking.k8s.io/gateway-name": "other"}, true),
	)
	got, err := findGatewayProxyPod(context.Background(), kc, "gw", "kgateway-system")
	if err != nil || got != "gw-ready" {
		t.Errorf("got (%q, %v), want gw-ready", got, err)
	}
	if _, err := findGatewayProxyPod(context.Background(), kc, "missing", "kgateway-system"); err == nil {
		t.Error("expected an error for a Gateway without ready pods")
	}
}

func TestFindDeploymentPod(t *testing.T) {
	sel := map[string]string{"app": "gw"}
	kc := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns"},
			Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: sel}},
		},
		readyPod("gw-1", "ns", sel, true),
	)
	got, err := findDeploymentPod(context.Background(), kc, "gw", "ns")
	if err != nil || got != "gw-1" {
		t.Errorf("got (%q, %v), want gw-1", got, err)
	}
	if _, err := findDeploymentPod(context.Background(), kc, "missing", "ns"); err == nil {
		t.Error("expected an error for a missing Deployment")
	}
}

// TestConfigDump_CancelDuringUpgrade cancels while the API server holds the
// port-forward upgrade unanswered (#25).
func TestConfigDump_CancelDuringUpgrade(t *testing.T) {
	received := make(chan struct{})
	clientGone := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	apiServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(received) })
		select {
		case <-r.Context().Done(): // the client closed the connection
			close(clientGone)
		case <-release:
		}
	}))
	defer apiServer.Close()
	defer close(release) // runs first, so Close doesn't wait on a held handler

	exited := make(chan struct{})
	forwardExited = func() { close(exited) }
	defer func() { forwardExited = func() {} }()

	cfg := &rest.Config{
		Host:            apiServer.URL,
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
		Proxy:           func(*http.Request) (*url.URL, error) { return nil, nil },
	}
	finder := func(context.Context, kubernetes.Interface, string) (string, error) { return "gw-0", nil }
	src := &kubeSource{open: func(ctx context.Context) (string, func(), error) {
		return portForward(ctx, "ns", Options{RESTConfig: cfg}, finder)
	}}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		rc, err := src.ConfigDump(ctx)
		if rc != nil {
			rc.Close()
		}
		errc <- err
	}()

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatalf("API server never received the upgrade request (ConfigDump: %v)", <-errc)
	}
	cancel()

	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ConfigDump error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ConfigDump did not return within 2s of cancel")
	}
	select {
	case <-exited:
	default:
		t.Error("forwarder goroutine still running when ConfigDump returned")
	}
	select {
	case <-clientGone:
	case <-time.After(time.Second):
		t.Error("API server still holds the connection 1s after ConfigDump returned")
	}
}

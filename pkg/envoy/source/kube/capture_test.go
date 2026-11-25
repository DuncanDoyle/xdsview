package kube

import (
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

// TestCapture: both port-forward sources fetch an unfiltered /config_dump.
func TestCapture(t *testing.T) {
	for name, src := range map[string]interface{ Capture() model.Capture }{
		"gateway":    Gateway("gw", "ns", Options{}),
		"deployment": Deployment("d", "ns", Options{}),
	} {
		if got := src.Capture(); got != model.CaptureComplete {
			t.Errorf("%s capture = %q, want complete", name, got)
		}
	}
}

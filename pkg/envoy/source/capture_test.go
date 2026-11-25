package source_test

import (
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source"
)

// TestCapture: a file may hold a filtered dump; the admin API's is complete.
func TestCapture(t *testing.T) {
	if got := source.File("dump.json").Capture(); got != model.CaptureUnverified {
		t.Errorf("File capture = %q, want unverified", got)
	}
	if got := source.Admin("http://localhost:19000").Capture(); got != model.CaptureComplete {
		t.Errorf("Admin capture = %q, want complete", got)
	}
}

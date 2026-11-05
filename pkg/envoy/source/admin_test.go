package source_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/source"
)

const dumpBody = `{"configs": []}`

func dumpServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config_dump" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dumpBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAdmin_Success(t *testing.T) {
	rc, err := source.Admin(dumpServer(t).URL).ConfigDump(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != dumpBody {
		t.Errorf("body = %q, want %q", got, dumpBody)
	}
}

func TestAdmin_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := source.Admin(srv.URL).ConfigDump(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err = %v, want an error naming HTTP 500", err)
	}
}

func TestAdmin_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Admin(dumpServer(t).URL).ConfigDump(ctx); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.json")
	if err := os.WriteFile(path, []byte(dumpBody), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, err := source.File(path).ConfigDump(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != dumpBody {
		t.Errorf("body = %q, want %q", got, dumpBody)
	}

	if _, err := source.File(filepath.Join(t.TempDir(), "missing.json")).ConfigDump(context.Background()); err == nil {
		t.Error("expected an error for a missing file")
	}
}

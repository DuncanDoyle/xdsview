package render_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/origin"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
)

var update = flag.Bool("update", false, "rewrite the golden .txt files in testdata/golden")

const testdata = "../../../testdata"

// TestText_Golden renders every OSS corpus dump, annotated with the kgateway
// mapper, and compares it with testdata/golden/scenarios/<scenario>/<version>.txt.
func TestText_Golden(t *testing.T) {
	dumps, _ := filepath.Glob(filepath.Join(testdata, "scenarios/*/envoy/kgateway-*/config_dump.json"))
	if len(dumps) == 0 {
		t.Fatal("no corpus dumps")
	}
	for _, dump := range dumps {
		version := filepath.Base(filepath.Dir(dump))
		scenario := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dump))))
		t.Run(scenario+"/"+version, func(t *testing.T) {
			f, err := os.Open(dump)
			if err != nil {
				t.Fatal(err)
			}
			res, err := parser.Parse(f, parser.Options{})
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			// The corpus HTTPRoutes, as krp's live mode lists them.
			known := []model.Ref{
				{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com"},
				{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "kgateway-system", Name: "api-example-com-admin"},
			}
			m := origin.Kgateway(origin.KgatewayOptions{Known: known})
			snap := chain.Annotate(origin.Annotate(res, m), m)

			var a, b bytes.Buffer
			if err := render.Text(&a, snap, render.Options{}); err != nil {
				t.Fatal(err)
			}
			if err := render.Text(&b, snap, render.Options{}); err != nil {
				t.Fatal(err)
			}
			out := a.String()
			if out != b.String() {
				t.Error("two renders differ")
			}
			if strings.Contains(out, "\x1b") {
				t.Error("output contains an escape sequence")
			}
			if !strings.HasSuffix(out, "\n") {
				t.Error("output doesn't end with LF")
			}
			for i, l := range strings.Split(out, "\n") {
				if strings.TrimRight(l, " \t") != l {
					t.Errorf("line %d has trailing whitespace: %q", i+1, l)
				}
			}

			path := filepath.Join(testdata, "golden/scenarios", scenario, version+".txt")
			if *update {
				if err := os.WriteFile(path, a.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading golden (run go test ./pkg/envoy/render -update): %v", err)
			}
			if out != string(want) {
				t.Errorf("output differs from %s (run go test ./pkg/envoy/render -update and review the diff)", path)
			}
		})
	}
}

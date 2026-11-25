package main

import (
	"bytes"
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

// TestText_Golden renders every OSS corpus dump and hand-built fixture the
// way krp does, with origins and contribution labels from the kgateway
// mapper, and compares it with testdata/golden/scenarios/<scenario>/<version>.txt
// or testdata/golden/fixtures/<fixture>.txt.
func TestText_Golden(t *testing.T) {
	dumps, _ := filepath.Glob(filepath.Join(testdataDir, "scenarios/*/envoy/kgateway-*/config_dump.json"))
	if len(dumps) == 0 {
		t.Fatal("no corpus dumps")
	}
	golden := map[string]string{}
	for _, dump := range dumps {
		version := filepath.Base(filepath.Dir(dump))
		scenario := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dump))))
		golden[dump] = filepath.Join("scenarios", scenario, version)
	}
	fixtures, _ := filepath.Glob(filepath.Join(testdataDir, "fixtures/*/envoy/config_dump.json"))
	for _, dump := range fixtures {
		golden[dump] = filepath.Join("fixtures", filepath.Base(filepath.Dir(filepath.Dir(dump))))
		dumps = append(dumps, dump)
	}
	for _, dump := range dumps {
		t.Run(golden[dump], func(t *testing.T) {
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
			snap := origin.Annotate(res, m)
			chains := chain.Effective(snap)
			opts := render.Options{Labels: attribute(snap, chains, m).labels}

			var a, b bytes.Buffer
			if err := render.Text(&a, snap, chains, opts); err != nil {
				t.Fatal(err)
			}
			if err := render.Text(&b, snap, chains, opts); err != nil {
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
			checkGolden(t, filepath.Join(testdataDir, "golden", golden[dump]+".txt"), out)
		})
	}
}

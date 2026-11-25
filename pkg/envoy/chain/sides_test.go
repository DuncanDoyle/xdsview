package chain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// sidesExclusions are filter types seen in the corpus or fixtures that the
// sides table leaves out on purpose, with the reason.
var sidesExclusions = map[string]string{}

// TestSides_CoverCorpus: every HTTP and upstream filter type in every corpus
// and fixture dump is in the sides table or a recorded exclusion, and the
// table names the Envoy version it was checked against (R14).
func TestSides_CoverCorpus(t *testing.T) {
	if sidesEnvoyVersion == "" {
		t.Error("the sides table names no Envoy version")
	}
	var dumps []string
	err := filepath.WalkDir("../../../testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "golden" {
			return filepath.SkipDir
		}
		if d.Name() == "config_dump.json" {
			dumps = append(dumps, path)
		}
		return nil
	})
	if err != nil || len(dumps) == 0 {
		t.Fatalf("no dumps found: %v", err)
	}
	for _, path := range dumps {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		snap, err := parser.Parse(f, parser.Options{})
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range snap.Listeners {
			for _, fc := range l.FilterChains {
				if fc.HCM == nil {
					continue
				}
				for _, hf := range append(append([]model.HTTPFilter(nil), fc.HCM.HTTPFilters...), fc.HCM.UpstreamFilters...) {
					typ := typeName(installedType(hf))
					if _, ok := sides[typ]; ok {
						continue
					}
					if _, ok := sidesExclusions[typ]; !ok {
						t.Errorf("%s: filter %q has type %q, in neither the sides table nor its exclusions", path, hf.Name, typ)
					}
				}
			}
		}
	}
}

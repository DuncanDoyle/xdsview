// Package envoy_test guards the dependency boundary of krp's core library
// packages (decision D-004).
package envoy_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const modulePrefix = "github.com/DuncanDoyle/xdsview/pkg/envoy/"

// core packages may import only the standard library and each other, so that
// consumers such as kgateway-ui's client-go-free core can use them.
var core = []string{"model", "parser", "filter", "source", "origin", "chain", "render"}

// TestRenderHasNoCharmbracelet is the check kgwctl relies on: the text
// renderer pulls in no terminal styling library.
func TestRenderHasNoCharmbracelet(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", modulePrefix+"render").CombinedOutput()
	if err != nil {
		t.Fatalf("go list render: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "github.com/charmbracelet/") {
			t.Errorf("render depends on %s", dep)
		}
	}
}

// pure packages answer what Envoy runs from Envoy data alone, so they may not
// import provenance (design §1, P-010 rule 1).
var pure = []string{"model", "parser", "chain", "render"}

// provenancePkgs are the packages, and package trees, a pure package may not reach.
var provenancePkgs = []string{
	modulePrefix + "origin",
	modulePrefix + "filter",
	modulePrefix + "provenance",
	"github.com/DuncanDoyle/xdsview/pkg/adapter",
}

func TestPureCoreImportsNoProvenance(t *testing.T) {
	for _, pkg := range pure {
		out, err := exec.Command("go", "list", "-deps", modulePrefix+pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			for _, bad := range provenancePkgs {
				if dep == bad || strings.HasPrefix(dep, bad+"/") {
					t.Errorf("%s depends on %s; pure core packages may not import provenance", pkg, dep)
				}
			}
		}
	}
}

func TestCoreImportsOnlyStdlib(t *testing.T) {
	for _, pkg := range core {
		out, err := exec.Command("go", "list", "-deps",
			"-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", modulePrefix+pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if !slices.Contains(core, strings.TrimPrefix(dep, modulePrefix)) {
				t.Errorf("%s depends on %s; core packages may import only the standard library and pkg/envoy core", pkg, dep)
			}
		}
	}
}

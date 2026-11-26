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
var core = []string{"model", "parser", "source", "chain", "render"}

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
			if !strings.HasPrefix(dep, "github.com/DuncanDoyle/xdsview/") && strings.Contains(strings.SplitN(dep, "/", 2)[0], ".") {
				t.Errorf("%s depends on third-party %s", pkg, dep)
			}
			for _, bad := range provenancePkgs {
				if dep == bad || strings.HasPrefix(dep, bad+"/") {
					t.Errorf("%s depends on %s; pure core packages may not import provenance", pkg, dep)
				}
			}
		}
	}
}

// removed are the packages provenance replaced (Phase 9.3). Nothing in the
// module, tests included, may import them.
var removed = []string{modulePrefix + "origin", modulePrefix + "filter"}

func TestNothingImportsRemovedPackages(t *testing.T) {
	out, err := exec.Command("go", "list", "-test", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "github.com/DuncanDoyle/xdsview/...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		for _, imp := range fields[1:] {
			if slices.Contains(removed, imp) {
				t.Errorf("%s imports %s, which provenance replaced", fields[0], imp)
			}
		}
	}
}

func TestCoreImportsOnlyStdlib(t *testing.T) {
	var paths []string
	for _, pkg := range core {
		paths = append(paths, modulePrefix+pkg)
	}
	checkStdlibOnly(t, paths, paths)
}

// contract holds the packages consumers import for provenance (design §7). They
// may import only the standard library, the core and provenance.
var contract = []string{modulePrefix + "provenance", "github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"}

func TestContractImportsOnlyStdlib(t *testing.T) {
	allowed := []string{modulePrefix + "provenance"}
	for _, pkg := range core {
		allowed = append(allowed, modulePrefix+pkg)
	}
	checkStdlibOnly(t, contract, allowed)
}

// checkStdlibOnly fails for each non-standard dependency of pkgs outside allowed.
func checkStdlibOnly(t *testing.T, pkgs, allowed []string) {
	t.Helper()
	for _, pkg := range pkgs {
		out, err := exec.Command("go", "list", "-deps",
			"-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if dep != pkg && !slices.Contains(allowed, dep) {
				t.Errorf("%s depends on %s; it may import only the standard library and %v", pkg, dep, allowed)
			}
		}
	}
}

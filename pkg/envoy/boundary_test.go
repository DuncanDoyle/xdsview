// Package envoy_test guards the dependency boundary of krp's core library
// packages (decision D-004).
package envoy_test

import (
	"os/exec"
	"strings"
	"testing"
)

const modulePrefix = "github.com/DuncanDoyle/xdsview/pkg/envoy/"

// core packages may import only the standard library and each other, so that
// consumers such as kgateway-ui's client-go-free core can use them.
var core = []string{"model", "parser", "filter", "source", "origin"}

func TestCoreImportsOnlyStdlib(t *testing.T) {
	for _, pkg := range core {
		out, err := exec.Command("go", "list", "-deps",
			"-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", modulePrefix+pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if !strings.HasPrefix(dep, modulePrefix) || strings.HasPrefix(dep, modulePrefix+"source/kube") {
				t.Errorf("%s depends on %s; core packages may import only the standard library and pkg/envoy core", pkg, dep)
			}
		}
	}
}

package xdsview_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"
)

// forbidden matches local paths, sibling checkout names and edition names,
// which must stay out of this public repo. This file is the only exemption.
var forbidden = regexp.MustCompile(`(?i)/Users/|\.agents-tmp|enterprise|(^|[^a-z])solo([^a-z]|$)|gloo|sefk|_ee([^a-z0-9]|$)|kgwctl-claude|snellius-claude|kgateway-tooling-claude|filterchain-printer-cli-claude`)

// allowance permits one literal in files whose path matches glob.
type allowance struct {
	glob, literal, reason string
}

var allowances = []allowance{
	{"testdata/scenarios/*/k8s/httpbin.yaml", "gcr.io/solo-public/docs/hey", "OSS test image in runnable manifests"},
	{"docs/phases/phase-8/implementation-plan.md", "solo-public/docs/hey", "names the OSS test image"},
	{"docs/phases/phase-8/phase-8.2-brief.md", "solo-public/docs/hey", "names the OSS test image"},
	{"docs/phases/phase-8/phase-8.2-complete.md", "gcr.io/solo-public/docs/hey", "names the OSS test image"},
	{"docs/phases/phase-8/reviews/design-codex.md", "solo-public", "names the OSS test image"},
	{"docs/phases/phase-6/phase-6.2-complete.md", "envoy-gloo `2fac0b83`", "dated historical source citation"},
	{"go.sum", "h1:Y/CXytFA4m6baUTXGLOoWe4PQhGxaX0KpnayAqC48p4=", "checksum"},
}

// TestHygiene_NoLocalPathsOrEditionNames scans every tracked path and text file.
func TestHygiene_NoLocalPathsOrEditionNames(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git ls-files unavailable: %v", err)
	}
	used := make([]bool, len(allowances))
	for _, p := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if forbidden.MatchString(p) {
			t.Errorf("tracked path %q matches %q", p, forbidden.FindString(p))
		}
		if p == "hygiene_test.go" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read %s: %v", p, err)
			continue
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue // binary
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 64<<20)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			for i, a := range allowances {
				if ok, _ := path.Match(a.glob, p); ok && strings.Contains(line, a.literal) {
					line = strings.ReplaceAll(line, a.literal, "")
					used[i] = true
				}
			}
			if m := forbidden.FindString(line); m != "" {
				t.Errorf("%s:%d matches %q", p, n, m)
			}
		}
		if err := sc.Err(); err != nil {
			t.Errorf("scan %s: %v", p, err)
		}
	}
	// A stale allowance is a standing exception with nothing left to cover.
	for i, a := range allowances {
		if !used[i] {
			t.Errorf("allowance %q in %s (%s) no longer matches; remove it", a.literal, a.glob, a.reason)
		}
	}
}

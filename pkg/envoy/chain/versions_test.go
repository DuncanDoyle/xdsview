package chain_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
)

// recordingDir matches a recorded dump directory, "kgateway-v2.5.0[-suffix]".
var recordingDir = regexp.MustCompile(`^kgateway-v(\d+)\.(\d+)\.(\d+)(-.+)?$`)

// TestVersionTables_MatchCorpus (P-012 rule 4): the version-keyed tables name
// the versions the recorded dumps were taken on. The sides table names the
// Envoy of the newest recording; the kgateway minors are the recorded ones;
// the policy field table's kgateway version has a recording.
func TestVersionTables_MatchCorpus(t *testing.T) {
	dirs, err := filepath.Glob("../../../testdata/scenarios/*/envoy/kgateway-*")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no recordings found: %v", err)
	}
	var newest [3]int
	var envoy string
	var minors []string
	policyRecorded := false
	for _, dir := range dirs {
		m := recordingDir.FindStringSubmatch(filepath.Base(dir))
		if m == nil {
			t.Errorf("%s: not a kgateway-vX.Y.Z directory", dir)
			continue
		}
		v := [3]int{atoi(m[1]), atoi(m[2]), atoi(m[3])}
		if minor := "v" + m[1] + "." + m[2]; !slices.Contains(minors, minor) {
			minors = append(minors, minor)
		}
		if "v"+m[1]+"."+m[2]+"."+m[3] == kgateway.PolicyTableVersion {
			policyRecorded = true
		}
		if ev, ok := envoyVersion(t, filepath.Join(dir, "server_info.json")); ok && slices.Compare(v[:], newest[:]) > 0 {
			newest, envoy = v, ev
		}
	}
	if envoy != chain.SidesEnvoyVersion {
		t.Errorf("sides table names Envoy %q; the newest recording (kgateway v%d.%d.%d) runs %q", chain.SidesEnvoyVersion, newest[0], newest[1], newest[2], envoy)
	}
	slices.Sort(minors)
	if checked := kgateway.CheckedMinors(); !slices.Equal(minors, checked) {
		t.Errorf("recorded kgateway minors %v, CheckedMinors %v", minors, checked)
	}
	if !policyRecorded {
		t.Errorf("no recording of kgateway %s, the policy field table's version", kgateway.PolicyTableVersion)
	}
}

// envoyVersion reads server_info.json's "<sha>/<version>/..." as
// "v<version> (<sha[:8]>)", the sides table's form. ok is false without the file.
func envoyVersion(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	var info struct {
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal(b, &info) != nil {
		t.Fatalf("%s: unreadable: %v", path, err)
	}
	parts := strings.Split(info.Version, "/")
	if len(parts) < 2 || len(parts[0]) < 8 {
		t.Fatalf("%s: version %q", path, info.Version)
	}
	return "v" + parts[1] + " (" + parts[0][:8] + ")", true
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

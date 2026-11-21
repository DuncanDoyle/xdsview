package parser_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
)

// stateListener returns one listener version for a dynamic_listeners entry. The
// filter chain is named after the version so a test can see which one is shown.
func stateListener(version, updated string) string {
	return `{"version_info": "` + version + `", "last_updated": "` + updated + `",
 "listener": {"name": "listener~80", "address": {"socket_address": {"address": "0.0.0.0", "port_value": 80}},
  "filter_chains": [{"name": "` + version + `", "filters": []}]}}`
}

const stateError = `{"details": "duplicate filter chain match", "last_update_attempt": "2026-11-20T19:39:58Z",
 "failed_configuration": {"@type": "type.googleapis.com/envoy.config.listener.v3.Listener", "name": "listener~80"}}`

func parseListeners(t *testing.T, entries ...string) []model.Listener {
	t.Helper()
	dump := `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
 "dynamic_listeners": [` + strings.Join(entries, ",") + `]}]}`
	result, err := parser.Parse(strings.NewReader(dump))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings: %v", result.Warnings)
	}
	return result.Snapshot.Listeners
}

func chainNames(l model.Listener) []string {
	var out []string
	for _, fc := range l.FilterChains {
		out = append(out, fc.Name)
	}
	return out
}

// TestParse_ListenerState covers every dynamic_listeners shape Envoy writes
// (listener_manager_impl.cc, dumpListenerConfigs): which version is shown, and its state.
func TestParse_ListenerState(t *testing.T) {
	active := `"active_state": ` + stateListener("v2", "2026-11-20T19:40:00Z")
	warming := `"warming_state": ` + stateListener("v3", "2026-11-20T19:41:00Z")
	draining := `"draining_state": ` + stateListener("v1", "2026-11-20T19:30:00Z")
	errState := `"error_state": ` + stateError
	wantErr := &model.ListenerError{Details: "duplicate filter chain match", LastAttempt: "2026-11-20T19:39:58Z"}

	tests := []struct {
		name       string
		entry      string
		wantName   string
		wantAddr   string
		wantChains []string
		want       model.ListenerState
	}{
		{"active only", `{"name": "listener~80", ` + active + `}`, "listener~80", "0.0.0.0:80", []string{"v2"},
			model.ListenerState{Status: model.ListenerActive, LastUpdated: "2026-11-20T19:40:00Z"}},
		{"warming only", `{"name": "listener~80", ` + warming + `}`, "listener~80", "0.0.0.0:80", []string{"v3"},
			model.ListenerState{Status: model.ListenerWarming, LastUpdated: "2026-11-20T19:41:00Z"}},
		{"draining only", `{"name": "listener~80", ` + draining + `}`, "listener~80", "0.0.0.0:80", []string{"v1"},
			model.ListenerState{Status: model.ListenerDraining, LastUpdated: "2026-11-20T19:30:00Z"}},
		{"active and warming", `{"name": "listener~80", ` + active + `, ` + warming + `}`, "listener~80", "0.0.0.0:80", []string{"v2"},
			model.ListenerState{Status: model.ListenerActive, Updating: true, LastUpdated: "2026-11-20T19:40:00Z"}},
		{"active and draining", `{"name": "listener~80", ` + active + `, ` + draining + `}`, "listener~80", "0.0.0.0:80", []string{"v2"},
			model.ListenerState{Status: model.ListenerActive, Draining: true, LastUpdated: "2026-11-20T19:40:00Z"}},
		{"error only", `{"name": "listener~80", ` + errState + `}`, "listener~80", "", nil,
			model.ListenerState{Status: model.ListenerRejected, Error: wantErr}},
		{"active and error", `{"name": "listener~80", ` + active + `, ` + errState + `}`, "listener~80", "0.0.0.0:80", []string{"v2"},
			model.ListenerState{Status: model.ListenerActive, LastUpdated: "2026-11-20T19:40:00Z", Error: wantErr}},
		{"LDS-wide error, no name", `{` + errState + `}`, "", "", nil,
			model.ListenerState{Status: model.ListenerRejected, Error: wantErr}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := parseListeners(t, tt.entry)
			if len(ls) != 1 {
				t.Fatalf("got %d listeners, want 1", len(ls))
			}
			l := ls[0]
			if l.Name != tt.wantName || l.Address != tt.wantAddr {
				t.Errorf("Name, Address = %q, %q; want %q, %q", l.Name, l.Address, tt.wantName, tt.wantAddr)
			}
			if got := chainNames(l); !reflect.DeepEqual(got, tt.wantChains) {
				t.Errorf("filter chains = %v, want %v", got, tt.wantChains)
			}
			if !reflect.DeepEqual(l.State, tt.want) {
				t.Errorf("State = %+v, want %+v", l.State, tt.want)
			}
		})
	}
}

// TestParse_UndecodableHCMWarns: an HCM whose config doesn't decode leaves its
// filter chain without an HCM, and a warning says where.
func TestParse_UndecodableHCMWarns(t *testing.T) {
	dump := `{"configs": [{"@type": "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
 "dynamic_listeners": [{"name": "listener~80", "active_state": {"listener": {"name": "listener~80",
  "filter_chains": [{"name": "fc-bad", "filters": [{"name": "envoy.filters.network.http_connection_manager",
   "typed_config": {"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
    "http_filters": "not a list"}}]}]}}}]}]}`
	result, err := parser.Parse(strings.NewReader(dump))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], `"listener~80"`) || !strings.Contains(result.Warnings[0], `"fc-bad"`) {
		t.Errorf("warnings = %q, want one naming listener~80 and fc-bad", result.Warnings)
	}
	ls := result.Snapshot.Listeners
	if len(ls) != 1 || !reflect.DeepEqual(chainNames(ls[0]), []string{"fc-bad"}) || ls[0].FilterChains[0].HCM != nil {
		t.Errorf("listeners = %+v, want listener~80 with filter chain fc-bad and no HCM", ls)
	}
}

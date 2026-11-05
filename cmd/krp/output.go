package main

import (
	"encoding/json"
	"io"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const (
	outputText = "text"
	outputJSON = "json"
)

// schemaVersion versions the -o json envelope. It changes when a field is
// removed or renamed, not when one is added (decision D-005).
const schemaVersion = "v1alpha1"

// jsonEnvelope is the -o json document. The golden files in testdata/golden pin
// its shape; consumers test against them.
type jsonEnvelope struct {
	SchemaVersion string               `json:"schemaVersion"`
	Snapshot      *model.EnvoySnapshot `json:"snapshot"`
	Warnings      []string             `json:"warnings"`
}

// writeJSON writes snap and the parse warnings as one indented JSON document.
func writeJSON(w io.Writer, snap *model.EnvoySnapshot, warnings []string) error {
	if warnings == nil {
		warnings = []string{} // always an array, never null
	}
	if snap != nil && snap.Listeners == nil {
		// A filter that matches nothing yields no listeners; emit [] rather than null.
		empty := *snap
		empty.Listeners = []model.Listener{}
		snap = &empty
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonEnvelope{SchemaVersion: schemaVersion, Snapshot: snap, Warnings: warnings})
}

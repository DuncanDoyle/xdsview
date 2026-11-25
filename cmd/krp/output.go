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
	SchemaVersion string          `json:"schemaVersion"`
	Snapshot      *model.Snapshot `json:"snapshot"`
	// Contributions are by route ID, then slot ID: the entries of each
	// route's effectiveChain that krp attributes objects to.
	Contributions contributions `json:"contributions,omitempty"`
	Warnings      []string      `json:"warnings"`
}

// writeJSON writes snap with each route's chain, krp's contributions, and the
// parse warnings' messages as one indented JSON document. The typed warnings
// are in snap.
func writeJSON(w io.Writer, snap *model.Snapshot, chains map[model.ID]model.EffectiveChain, contribs contributions, parsed []model.Warning) error {
	warnings := []string{} // always an array, never null
	for _, pw := range parsed {
		warnings = append(warnings, pw.Message)
	}
	out := withChains(snap, chains)
	if out.Listeners == nil {
		// A filter that matches nothing yields no listeners; emit [] rather than null.
		out.Listeners = []model.Listener{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonEnvelope{SchemaVersion: schemaVersion, Snapshot: out, Contributions: contribs, Warnings: warnings})
}

// withChains returns a copy of snap with Route.Chain set from chains, and
// RouteConfigUse.Chain for an HCM with no route config. snap is not modified;
// the copy shares the parts it doesn't change.
func withChains(snap *model.Snapshot, chains map[model.ID]model.EffectiveChain) *model.Snapshot {
	out := *snap
	if snap.Listeners == nil {
		return &out
	}
	out.Listeners = make([]model.Listener, len(snap.Listeners))
	for i, l := range snap.Listeners {
		fcs := make([]model.NetworkFilterChain, len(l.FilterChains))
		for j, fc := range l.FilterChains {
			if fc.HCM != nil && fc.HCM.RouteConfig == nil {
				if c, ok := chains[fc.HCM.RouteConfigUse.ID]; ok {
					hcm := *fc.HCM
					hcm.RouteConfigUse.Chain = &c
					fc.HCM = &hcm
				}
			}
			if fc.HCM != nil && fc.HCM.RouteConfig != nil {
				hcm := *fc.HCM
				rc := *hcm.RouteConfig
				rc.VirtualHosts = make([]model.VirtualHost, len(hcm.RouteConfig.VirtualHosts))
				for k, vh := range hcm.RouteConfig.VirtualHosts {
					routes := make([]model.Route, len(vh.Routes))
					for n, r := range vh.Routes {
						if c, ok := chains[r.ID]; ok {
							r.Chain = &c
						}
						routes[n] = r
					}
					vh.Routes = routes
					rc.VirtualHosts[k] = vh
				}
				hcm.RouteConfig = &rc
				fc.HCM = &hcm
			}
			fcs[j] = fc
		}
		l.FilterChains = fcs
		out.Listeners[i] = l
	}
	return &out
}

package provenance

import "github.com/DuncanDoyle/xdsview/pkg/envoy/model"

// Covers returns, per covered input of block, the IDs of the claims that can cover it.
func Covers(ix *Index, block model.ID) [][]string {
	_, inputs := ix.inputs(block)
	var out [][]string
	for _, in := range inputs {
		if !in.covered {
			continue
		}
		var ids []string
		for _, i := range in.covers {
			ids = append(ids, ix.claims[i].ID)
		}
		out = append(out, ids)
	}
	return out
}

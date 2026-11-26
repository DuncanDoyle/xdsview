package provenance

import "github.com/DuncanDoyle/xdsview/pkg/envoy/model"

// Inputs returns the gap each input of block reports when no claim covers it.
func Inputs(ix *Index, block model.ID) []Gap {
	_, inputs := ix.inputs(block)
	var out []Gap
	for _, in := range inputs {
		out = append(out, in.gap)
	}
	return out
}

// Cover is one covered input: its gap and the IDs of the claims that can cover it.
type Cover struct {
	Gap Gap
	IDs []string
}

// Covers returns the covered inputs of block.
func Covers(ix *Index, block model.ID) []Cover {
	_, inputs := ix.inputs(block)
	var out []Cover
	for _, in := range inputs {
		if !in.covered || in.bootstrap {
			continue
		}
		c := Cover{Gap: in.gap}
		for _, i := range in.covers {
			c.IDs = append(c.IDs, ix.claims[i].ID)
		}
		out = append(out, c)
	}
	return out
}

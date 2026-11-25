package model_test

import (
	"testing"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

func TestID_Within(t *testing.T) {
	cases := []struct {
		id, block model.ID
		want      bool
	}{
		{"L0/C1/N0/RC/V2/R5", "L0/C1/N0/RC/V2/R5", true},
		{"L0/C1/N0/RC/V2/R5", "L0/C1/N0/RC/V2", true},
		{"L0/C1/N0/RC/V2/R5", "L0", true},
		{"L0/C1/N0/RC/V2/R5", "L0/C1/N0/RC/V2/R50", false},
		{"L0/C1/N0/RC/V2/R5", "L0/C1/N0/RC/V1", false},
		{"L10/C0", "L1", false},
		{"L1", "L10", false},
		{"L0", "", false},
	}
	for _, c := range cases {
		if got := c.id.Within(c.block); got != c.want {
			t.Errorf("%q.Within(%q) = %v, want %v", c.id, c.block, got, c.want)
		}
	}
}

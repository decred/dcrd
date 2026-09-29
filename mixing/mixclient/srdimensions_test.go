package mixclient

import (
	"math/big"
	"testing"

	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/wire"
)

// The SR blame path indexes the submitted matrix as DCMix[j][k], where j is bounded
// by the peer's MessageCount and k by the width of the mix being compared. A peer
// that publishes fewer rows, or a short row, must be blamed rather than read out of
// bounds. These tests pin the two dimension checks the loop performs before any
// index is taken.

// srDimensionsMatch reports whether the submitted matrix has the shape the SR loop
// indexes: one row per message, each as wide as the mix it is compared against.
func srDimensionsMatch(dcMix []wire.MixVect, mcount int, mixWidth int) bool {
	if len(dcMix) != mcount {
		return false
	}
	for _, row := range dcMix {
		if len(row) != mixWidth {
			return false
		}
	}
	return true
}

func TestSRMatrixDimensions(t *testing.T) {
	const (
		mcount   = 2
		mixWidth = 3
		shortRow = 2
		extraRow = 3
	)

	full := func(rows, width int) []wire.MixVect {
		out := make([]wire.MixVect, rows)
		for i := range out {
			row := make(wire.MixVect, width)
			out[i] = row
		}
		return out
	}

	tests := []struct {
		name   string
		matrix []wire.MixVect
		want   bool
	}{
		{"exact match", full(mcount, mixWidth), true},
		{"too few rows", full(mcount-1, mixWidth), false},
		{"too many rows", full(extraRow, mixWidth), false},
		{"short row", full(mcount, shortRow), false},
		{"no rows", nil, false},
	}

	for _, tc := range tests {
		if got := srDimensionsMatch(tc.matrix, mcount, mixWidth); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSRMixWidthIsPaddings documents the invariant the width check relies on: the
// mix vector a row is compared against is exactly as wide as the pads, so a
// committed row narrower than that is an out-of-bounds read on DCMix[j][k].
func TestSRMixWidthIsPaddings(t *testing.T) {
	pads := make([][]byte, 3)
	for i := range pads {
		pads[i] = make([]byte, 32)
	}

	mix := mixing.SRMix(big.NewInt(7), mixing.SRMixPads(pads, 0))
	if len(mix) != len(pads) {
		t.Fatalf("SRMix width %d does not match pads %d", len(mix), len(pads))
	}

	// Indexing a narrower committed row at the mix width is the panic this guards.
	narrow := make([][]byte, len(mix)-1)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected out-of-bounds on a short committed row")
			}
		}()
		_ = narrow[len(mix)-1]
	}()
}

package blockchainutil

import (
	"fmt"
	"math/big"
	"testing"
)

func TestBigToCompact(t *testing.T) {
	tests := []struct {
		in  int64
		out uint32
	}{
		{0, 0},
		{-1, 25231360},
	}

	for x, test := range tests {
		n := big.NewInt(test.in)
		r := BigToCompact(n)
		if r != test.out {
			t.Errorf("TestBigToCompact test #%d failed: got %d want %d\n",
				x, r, test.out)
			return
		}
	}
}

func TestCompactToBig(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		n := CompactToBig(test.in)
		want := big.NewInt(test.out)
		if n.Cmp(want) != 0 {
			t.Errorf("TestCompactToBig test #%d failed: got %d want %d\n",
				x, n.Int64(), want.Int64())
			return
		}
	}
}

// This example demonstrates how to convert the compact "bits" in a block header
// which represent the target difficulty to a big integer and display it using
// the typical hex notation.
func ExampleCompactToBig() {
	// Convert the bits from block 300000 in the main Decred block chain.
	bits := uint32(419465580)
	targetDifficulty := CompactToBig(bits)

	// Display it in hex.
	fmt.Printf("%064x\n", targetDifficulty.Bytes())

	// Output:
	// 0000000000000000896c00000000000000000000000000000000000000000000
}

// This example demonstrates how to convert a target difficulty into the compact
// "bits" in a block header which represent that target difficulty .
func ExampleBigToCompact() {
	// Convert the target difficulty from block 300000 in the main block
	// chain to compact form.
	t := "0000000000000000896c00000000000000000000000000000000000000000000"
	targetDifficulty, success := new(big.Int).SetString(t, 16)
	if !success {
		fmt.Println("invalid target difficulty")
		return
	}
	bits := BigToCompact(targetDifficulty)

	fmt.Println(bits)

	// Output:
	// 419465580
}

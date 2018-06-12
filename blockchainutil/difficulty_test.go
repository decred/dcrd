package blockchainutil

import (
	"math/big"
	"testing"
)

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)
)

var testsArray = []struct {
	hashString string
	compact    uint32
	bigInt     *big.Int
}{
	{
		hashString: "00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		compact:    0x1d00ffff,
		bigInt:     new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
	},
	{
		hashString: "00 00 00 00 ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff ff",
		compact:    0x1d00ffff,
		bigInt:     new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
	},
	{
		hashString: "00 00 00 00 ffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		compact:    0x1d00ffff,
		bigInt:     new(big.Int).Sub(new(big.Int).Lsh(bigOne, 224), bigOne),
	},
	{
		hashString: "000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		compact:    0x1e00ffff,
		bigInt:     new(big.Int).Sub(new(big.Int).Lsh(bigOne, 232), bigOne),
	},
	{
		hashString: "7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		compact:    0x207fffff,
		bigInt:     new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne),
	},
}

func TestDifficulty(t *testing.T) {
	for i, test := range testsArray {
		diffFromHash := NewDifficultyFromHashString(test.hashString).Print()

		bigInt := diffFromHash.ToBigInt()
		if bigInt.Cmp(test.bigInt) != 0 {
			t.Fatalf("TestDifficulty[%v] values mismatch: diffFromHash.ToBigInt()=%v is not equal to test.bigInt=%v", i, diffFromHash.ToBigInt(), test.bigInt)
		}

		compact := diffFromHash.ToCompact()
		if compact != test.compact {
			t.Fatalf("TestDifficulty[%v] values mismatch: diffFromHash.ToCompact()=%v is not equal to test.compact=%v", i, diffFromHash.ToCompact(), test.compact)
		}
	}

}

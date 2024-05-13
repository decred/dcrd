package mixing

import (
	"math/big"
)

// FieldPrime is the field prime 2**127 - 1.
var F *big.Int

func init() {
	F, _ = new(big.Int).SetString("7fffffffffffffffffffffffffffffff", 16)
}

// InField returns whether x is bounded by the field F.
func InField(x *big.Int) bool {
	return x.Sign() != -1 && x.Cmp(F) == -1
}

// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

package secp256k1

import (
	"github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/internal/cpufeat"
)

// field64UseBMI2AndADX is enabled when the CPU supports both BMI2 (MULX) and
// ADX (ADCX/ADOX).
var field64UseBMI2AndADX = func() bool {
	f := cpufeat.Supported()
	return f.BMI2 && f.ADX
}()

//go:noescape
func field64MulReduceADX(r *[4]uint64, a, b *[4]uint64)

//go:noescape
func field64SquareReduceADX(r *[4]uint64, a *[4]uint64)

// field64MulReduce sets r = a * b (mod p)
func field64MulReduce(r *[4]uint64, a, b *[4]uint64) {
	if field64UseBMI2AndADX {
		field64MulReduceADX(r, a, b)
		return
	}
	field64MulReduceGeneric(r, a, b)
}

// field64SquareReduce sets r = a^2 (mod p)
func field64SquareReduce(r *[4]uint64, a *[4]uint64) {
	if field64UseBMI2AndADX {
		field64SquareReduceADX(r, a)
		return
	}
	field64SquareReduceGeneric(r, a)
}

// field64MulReduceGeneric sets r = a * b (mod p)
func field64MulReduceGeneric(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	arith.Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64SquareReduceGeneric sets r = a^2 (mod p)
func field64SquareReduceGeneric(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	arith.Square512(&product, a)
	field64Reduce512(r, &product)
}

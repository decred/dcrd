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
func field64MulADX(r *[4]uint64, a, b *[4]uint64)

//go:noescape
func field64SquareADX(r *[4]uint64, a *[4]uint64)

// field64Mul sets r = a * b (mod p)
func field64Mul(r *[4]uint64, a, b *[4]uint64) {
	if field64UseBMI2AndADX {
		field64MulADX(r, a, b)
		return
	}
	field64MulGeneric(r, a, b)
}

// field64Square sets r = a^2 (mod p)
func field64Square(r *[4]uint64, a *[4]uint64) {
	if field64UseBMI2AndADX {
		field64SquareADX(r, a)
		return
	}
	field64SquareGeneric(r, a)
}

// field64MulGeneric sets r = a * b (mod p)
func field64MulGeneric(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	arith.Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64SquareGeneric sets r = a^2 (mod p)
func field64SquareGeneric(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	arith.Square512(&product, a)
	field64Reduce512(r, &product)
}

// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !purego

package field4x64

import (
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

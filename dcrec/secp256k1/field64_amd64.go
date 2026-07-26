// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

package secp256k1

// field64UseADX reports whether the CPU supports both BMI2 (MULX) and ADX (ADCX/ADOX).
var field64UseADX = func() bool {
	const (
		eaxMax  = 0
		eaxExt  = 7
		bmi2Bit = 8
		adxBit  = 19
	)

	// Leaf 0x07 is only valid when the CPU reports it within the max input.
	eax, _, _, _ := field64CPUID(eaxMax, 0)
	if eax < eaxExt {
		return false
	}

	_, ebx, _, _ := field64CPUID(eaxExt, 0)
	hasBMI2 := ebx>>bmi2Bit&1 == 1
	hasADX := ebx>>adxBit&1 == 1
	return hasBMI2 && hasADX
}()

//go:noescape
func field64MulADX(r *[4]uint64, a, b *[4]uint64)

//go:noescape
func field64SquareADX(r *[4]uint64, a *[4]uint64)

// field64CPUID provides access to the CPUID opcode.
//
//go:noescape
func field64CPUID(eaxIn, ecxIn uint32) (eax, ebx, ecx, edx uint32)

// field64Mul sets r = a * b (mod p)
func field64Mul(r *[4]uint64, a, b *[4]uint64) {
	if field64UseADX {
		field64MulADX(r, a, b)
		return
	}
	field64MulGeneric(r, a, b)
}

// field64Square sets r = a^2 (mod p)
func field64Square(r *[4]uint64, a *[4]uint64) {
	if field64UseADX {
		field64SquareADX(r, a)
		return
	}
	field64SquareGeneric(r, a)
}

// field64MulGeneric sets r = a * b (mod p)
func field64MulGeneric(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	field64Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64SquareGeneric sets r = a^2 (mod p)
func field64SquareGeneric(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	field64Square512(&product, a)
	field64Reduce512(r, &product)
}

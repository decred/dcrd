// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"github.com/decred/dcrd/internal/staging/primitives/uint256"
)

// DiffBitsToUint256 converts the compact representation used to encode
// difficulty targets to an unsigned 256-bit integer.  The representation is
// similar to IEEE754 floating point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
//	* the most significant 8 bits represent the unsigned base 256 exponent
//	* bit 23 (the 24th bit) represents the sign bit
//	* the least significant 23 bits represent the mantissa
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	-------------------------------------------------
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
//	N = (-1^sign) * mantissa * 256^(exponent-3)
//
// Note that this encoding is capable of representing negative numbers as well
// as numbers much larger than the maximum value of an unsigned 256-bit integer.
// However, it is only used in Decred to encode unsigned 256-bit integers which
// represent difficulty targets, so rather than using a much less efficient
// arbitrary precision big integer, this implementation uses an unsigned 256-bit
// integer and returns flags to indicate whether or not the encoding was for a
// negative value and/or overflows a uint256 to enable proper error detection
// and stay consistent with legacy code.
func DiffBitsToUint256(bits uint32) (n uint256.Uint256, isNegative bool, overflows bool) {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := bits & 0x007fffff
	isSignBitSet := bits&0x00800000 != 0
	exponent := bits >> 24

	// Nothing to do when the mantissa is zero as any multiple of it will
	// necessarily also be 0 and therefore it can never be negative or overflow.
	if mantissa == 0 {
		return n, false, false
	}

	// Since the base for the exponent is 256 = 2^8, the exponent is a multiple
	// of 8 and thus the full 256-bit number is computed by shifting the
	// mantissa right or left accordingly.
	//
	// This and the following section are equivalent to:
	//
	// N = mantissa * 256^(exponent-3)
	if exponent <= 3 {
		n.SetUint64(uint64(mantissa >> (8 * (3 - exponent))))
		return n, isSignBitSet, false
	}

	// Notice that since this is decoding into a uint256, any values that take
	// more than 256 bits to represent will overflow.  Also, since the encoded
	// exponent value is decreased by 3 and then multiplied by 8, any encoded
	// exponent of 35 or greater will cause an overflow because 256/8 + 3 = 35.
	// Decreasing the encoded exponent by one to 34 results in making 8 bits
	// available, meaning the max mantissa can be 0xff in that case.  Similarly,
	// decreasing the encoded exponent again to 33 results in making a total of
	// 16 bits available, meaning the max mantissa can be 0xffff in that case.
	// Finally, decreasing the encoded exponent again to 32 results in making a
	// total of 24 bits available and since the mantissa only encodes 23 bits,
	// overflow is impossible for all encoded exponents of 32 or lower.
	overflows = exponent >= 35 || (exponent >= 34 && mantissa > 0xff) ||
		(exponent >= 33 && mantissa > 0xffff)
	if overflows {
		return n, isSignBitSet, true
	}
	n.SetUint64(uint64(mantissa))
	n.Lsh(8 * (exponent - 3))
	return n, isSignBitSet, false
}

// uint256ToDiffBits converts a uint256 to a compact representation using an
// unsigned 32-bit integer.  The compact representation only provides 23 bits of
// precision, so values larger than (2^23 - 1) only encode the most significant
// digits of the number.  See DiffBitsToUint256 for details.
//
// NOTE: The only difference between this function and the exported variant is
// that this one accepts a parameter to indicate whether or not the encoding
// should be for a negative value whereas the exported variant is always
// positive as expected for an unsigned value.  This is done in order to stay
// consistent with the encoding used by legacy code and for testing purposes,
// however, difficulty bits are only used in Decred to encode unsigned 256-bit
// integers which represent difficulty targets, so, it will always be called
// with false in practice.
func uint256ToDiffBits(n *uint256.Uint256, isNegative bool) uint32 {
	// No need to do any work if it's zero.
	if n.IsZero() {
		return 0
	}

	// Since the base for the exponent is 256, the exponent can be treated as
	// the number of bytes it takes to represent the value.  So, shift the
	// number right or left accordingly.  This is equivalent to:
	// mantissa = n / 256^(exponent-3)
	var mantissa uint32
	exponent := uint32((n.BitLen() + 7) / 8)
	if exponent <= 3 {
		mantissa = n.Uint32() << (8 * (3 - exponent))
	} else {
		// Use a copy to avoid modifying the caller's original value.
		mantissa = new(uint256.Uint256).RshVal(n, 8*(exponent-3)).Uint32()
	}

	// When the mantissa already has the sign bit set, the number is too large
	// to fit into the available 23-bits, so divide the number by 256 and
	// increment the exponent accordingly.
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	// Pack the exponent, sign bit, and mantissa into an unsigned 32-bit int and
	// return it.
	//
	// Note that the sign bit is conditionally set based on the provided flag
	// since uint256s can never be negative.
	bits := exponent<<24 | mantissa
	if isNegative {
		bits |= 0x00800000
	}
	return bits
}

// Uint256ToDiffBits converts a uint256 to a compact representation using an
// unsigned 32-bit integer.  The compact representation only provides 23 bits of
// precision, so values larger than (2^23 - 1) only encode the most significant
// digits of the number.  See DiffBitsToUint256 for details.
func Uint256ToDiffBits(n *uint256.Uint256) uint32 {
	const isNegative = false
	return uint256ToDiffBits(n, isNegative)
}

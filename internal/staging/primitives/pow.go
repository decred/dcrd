// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/math/uint256"
)

// DiffBitsToUint256 converts the compact representation used to encode
// difficulty targets to an unsigned 256-bit integer.  The representation is
// similar to IEEE754 floating point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
//  1. the most significant 8 bits represent the unsigned base 256 exponent
//  2. zero-based bit 23 (the 24th bit) represents the sign bit
//  3. the least significant 23 bits represent the mantissa
//
// Diagram:
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	|-----------------------------------------------|
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
//
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

// CalcWork calculates a work value from difficulty bits.  Decred increases the
// difficulty for generating a block by decreasing the value which the generated
// hash must be less than.  This difficulty target is stored in each block
// header using a compact representation as described in the documentation for
// DiffBitsToUint256.  The main chain is selected by choosing the chain that has
// the most proof of work (highest difficulty).  Since a lower target difficulty
// value equates to higher actual difficulty, the work value which will be
// accumulated must be the inverse of the difficulty.  For legacy reasons, the
// result is zero when the difficulty is zero.  Finally, to avoid really small
// floating point numbers, the result multiplies the numerator by 2^256 and adds
// 1 to the denominator.
func CalcWork(diffBits uint32) uint256.Uint256 {
	// Return a work value of zero if the passed difficulty bits represent a
	// negative number, a number that overflows a uint256, or zero. Note this
	// should not happen in practice with valid blocks, but an invalid block
	// could trigger it.
	diff, isNegative, overflows := DiffBitsToUint256(diffBits)
	if isNegative || overflows || diff.IsZero() {
		return uint256.Uint256{}
	}

	// The goal is to calculate 2^256 / (diff+1), where diff > 0 using a
	// fixed-precision uint256.
	//
	// Since 2^256 can't be represented by a uint256, the calc is performed as
	// follows:
	//
	// Notice:
	//    work = (2^256 / (diff+1))
	// => work = ((2^256-diff-1) / (diff+1))+1
	//
	// Next, observe that 2^256-diff-1 is the one's complement of diff as a
	// uint256 which is equivalent to the bitwise not.  Also, of special note is
	// the case when diff = 2^256-1 because (2^256-1)+1 ≡ 0 (mod 2^256) and
	// thus would result in division by zero when working with a uint256.  The
	// original calculation would produce 1 in that case, so the resulting
	// piecewise function is:
	//
	// {work = 1                   , where diff = 2^256-1
	// {work = (^diff / (diff+1))+1, where 0 < diff < 2^256-1
	//
	// However, a difficulty target of 2^256 - 1 is impossible to encode in the
	// difficulty bits, so it is safe to ignore that case.
	divisor := new(uint256.Uint256).SetUint64(1).Add(&diff)
	return *diff.Not().Div(divisor).AddUint64(1)
}

// HashToUint256 converts the provided hash to an unsigned 256-bit integer that
// can be used to perform math comparisons.
func HashToUint256(hash *chainhash.Hash) uint256.Uint256 {
	// Hashes are a stream of bytes that do not have any inherent endianness to
	// them, so they are interpreted as little endian for the purposes of
	// treating them as a uint256.
	return *new(uint256.Uint256).SetBytesLE((*[32]byte)(hash))
}

// checkProofOfWorkRange ensures the provided target difficulty is in min/max
// range per the provided proof-of-work limit.
func checkProofOfWorkRange(diffBits uint32, powLimit *uint256.Uint256) (uint256.Uint256, error) {
	// The target difficulty must be larger than zero and not overflow and less
	// than the maximum value that can be represented by a uint256.
	target, isNegative, overflows := DiffBitsToUint256(diffBits)
	if isNegative {
		str := fmt.Sprintf("target difficulty bits %08x is a negative value",
			diffBits)
		return uint256.Uint256{}, ruleError(ErrUnexpectedDifficulty, str)
	}
	if overflows {
		str := fmt.Sprintf("target difficulty bits %08x is higher than the "+
			"max limit %064x", diffBits, powLimit)
		return uint256.Uint256{}, ruleError(ErrUnexpectedDifficulty, str)
	}
	if target.IsZero() {
		str := "target difficulty is zero"
		return uint256.Uint256{}, ruleError(ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must not exceed the maximum allowed.
	if target.Gt(powLimit) {
		str := fmt.Sprintf("target difficulty %064x is higher than max %064x",
			target, powLimit)
		return uint256.Uint256{}, ruleError(ErrUnexpectedDifficulty, str)
	}

	return target, nil
}

// CheckProofOfWorkRange ensures the provided target difficulty represented by
// the given header bits is in min/max range per the provided proof-of-work
// limit.
func CheckProofOfWorkRange(diffBits uint32, powLimit *uint256.Uint256) error {
	_, err := checkProofOfWorkRange(diffBits, powLimit)
	return err
}

// CheckProofOfWork ensures the provided hash is less than the target difficulty
// represented by given header bits and that said difficulty is in min/max range
// per the provided proof-of-work limit.
func CheckProofOfWork(powHash *chainhash.Hash, diffBits uint32, powLimit *uint256.Uint256) error {
	target, err := checkProofOfWorkRange(diffBits, powLimit)
	if err != nil {
		return err
	}

	// The block hash must be less than the target difficulty.
	hashNum := HashToUint256(powHash)
	if hashNum.Gt(&target) {
		str := fmt.Sprintf("proof of work hash %064x is higher than expected "+
			"max of %064x", hashNum, target)
		return ruleError(ErrHighHash, str)
	}

	return nil
}

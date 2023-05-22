// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid the
	// overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// oneLsh256 is 1 shifted left 256 bits.  It is defined here to avoid the
	// overhead of creating it multiple times.
	oneLsh256 = new(big.Int).Lsh(bigOne, 256)
)

// HashToBig converts a chainhash.Hash into a big.Int that can be used to
// perform math comparisons.
func HashToBig(hash *chainhash.Hash) *big.Int {
	// A Hash is in little-endian, but the big package wants the bytes in
	// big-endian, so reverse them.
	buf := *hash
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	return new(big.Int).SetBytes(buf[:])
}

// CompactToBig converts a compact representation of a whole number N to an
// unsigned 32-bit number.  The representation is similar to IEEE754 floating
// point numbers.
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
// This compact form is only used in Decred to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with legacy code.
func CompactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated as
	// the number of bytes to represent the full 256-bit number.  So, treat the
	// exponent as the number of bytes and shift the mantissa right or left
	// accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

// BigToCompact converts a whole number N to a compact representation using an
// unsigned 32-bit number.  The compact representation only provides 23 bits of
// precision, so values larger than (2^23 - 1) only encode the most significant
// digits of the number.  See CompactToBig for details.
func BigToCompact(n *big.Int) uint32 {
	// No need to do any work if it's zero.
	if n.Sign() == 0 {
		return 0
	}

	// Since the base for the exponent is 256, the exponent can be treated as
	// the number of bytes.  So, shift the number right or left accordingly.
	// This is equivalent to:
	// mantissa = mantissa / 256^(exponent-3)
	var mantissa uint32
	exponent := uint(len(n.Bytes()))
	if exponent <= 3 {
		mantissa = uint32(n.Bits()[0])
		mantissa <<= 8 * (3 - exponent)
	} else {
		// Use a copy to avoid modifying the caller's original number.
		tn := new(big.Int).Set(n)
		mantissa = uint32(tn.Rsh(tn, 8*(exponent-3)).Bits()[0])
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
	compact := uint32(exponent<<24) | mantissa
	if n.Sign() < 0 {
		compact |= 0x00800000
	}
	return compact
}

// CalcWork calculates a work value from difficulty bits.  Decred increases the
// difficulty for generating a block by decreasing the value which the generated
// hash must be less than.  This difficulty target is stored in each block
// header using a compact representation as described in the documentation for
// CompactToBig.  The main chain is selected by choosing the chain that has the
// most proof of work (highest difficulty).  Since a lower target difficulty
// value equates to higher actual difficulty, the work value which will be
// accumulated must be the inverse of the difficulty.  Also, in order to avoid
// potential division by zero and really small floating point numbers, the
// result adds 1 to the denominator and multiplies the numerator by 2^256.
func CalcWork(bits uint32) *big.Int {
	// Return a work value of zero if the passed difficulty bits represent a
	// negative number. Note this should not happen in practice with valid
	// blocks, but an invalid block could trigger it.
	difficultyNum := CompactToBig(bits)
	if difficultyNum.Sign() <= 0 {
		return big.NewInt(0)
	}

	// (1 << 256) / (difficultyNum + 1)
	denominator := new(big.Int).Add(difficultyNum, bigOne)
	return new(big.Int).Div(oneLsh256, denominator)
}

// checkProofOfWorkRange ensures the provided target difficulty is in min/max
// range per the provided proof-of-work limit.
func checkProofOfWorkRange(target *big.Int, powLimit *big.Int) error {
	// The target difficulty must be larger than zero.
	if target.Sign() <= 0 {
		str := fmt.Sprintf("target difficulty of %064x is too low", target)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(powLimit) > 0 {
		str := fmt.Sprintf("target difficulty of %064x is higher than max of "+
			"%064x", target, powLimit)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	return nil
}

// CheckProofOfWorkRange ensures the provided compact target difficulty is in
// min/max range per the provided proof-of-work limit.
func CheckProofOfWorkRange(difficultyBits uint32, powLimit *big.Int) error {
	target := CompactToBig(difficultyBits)
	return checkProofOfWorkRange(target, powLimit)
}

// checkProofOfWorkHash ensures the provided hash is less than the provided
// target difficulty.
func checkProofOfWorkHash(powHash *chainhash.Hash, target *big.Int) error {
	// The proof of work hash must be less than the target difficulty.
	hashNum := HashToBig(powHash)
	if hashNum.Cmp(target) > 0 {
		str := fmt.Sprintf("proof of work hash %064x is higher than "+
			"expected max of %064x", hashNum, target)
		return ruleError(ErrHighHash, str)
	}

	return nil
}

// CheckProofOfWorkHash ensures the provided hash is less than the provided
// compact target difficulty.
func CheckProofOfWorkHash(powHash *chainhash.Hash, difficultyBits uint32) error {
	target := CompactToBig(difficultyBits)
	return checkProofOfWorkHash(powHash, target)
}

// CheckProofOfWork ensures the provided hash is less than the provided compact
// target difficulty and that the target difficulty is in min/max range per the
// provided proof-of-work limit.
//
// This is semantically equivalent to and slightly more efficient than calling
// CheckProofOfWorkRange followed by CheckProofOfWorkHash.
func CheckProofOfWork(powHash *chainhash.Hash, difficultyBits uint32, powLimit *big.Int) error {
	target := CompactToBig(difficultyBits)
	if err := checkProofOfWorkRange(target, powLimit); err != nil {
		return err
	}

	// The proof of work hash must be less than the target difficulty.
	return checkProofOfWorkHash(powHash, target)
}

// CalcASERTDiff calculates an absolutely scheduled exponentially weighted
// target difficulty for the given set of parameters using the algorithm defined
// in DCP0011.
//
// The Absolutely Scheduled Exponentially weighted Rising Targets (ASERT)
// algorithm defines an ideal schedule for block issuance and calculates the
// difficulty based on how far the most recent block's timestamp is ahead or
// behind that schedule.
//
// The target difficulty is set exponentially such that it is doubled or halved
// for every multiple of the half life the most recent block is ahead or behind
// the ideal schedule.
//
// The starting difficulty bits parameter is the initial target difficulty all
// calculations use as a reference.  This value is defined on a per-chain basis.
// It must be non-zero and less than or equal to the provided proof of work
// limit or the function will panic.
//
// The time delta is the number of seconds that have elapsed between the most
// recent block and an initial reference timestamp.
//
// The height delta is the number of blocks between the most recent block height
// and an initial reference height.  It must be non-negative or the function
// will panic.
//
// NOTE: This only performs the primary target difficulty calculation and does
// not include any additional special network rules such as enforcing a maximum
// allowed test network difficulty.  It is up to the caller to impose any such
// additional restrictions.
//
// This function is safe for concurrent access.
func CalcASERTDiff(startDiffBits uint32, powLimit *big.Int, targetSecsPerBlock,
	timeDelta, heightDelta, halfLife int64) uint32 {

	// Ensure parameter assumptions are not violated.
	//
	// 1. The starting target difficulty must be in the range [1, powLimit]
	// 2. The height to calculate the difficulty for must come after the height
	//    of the reference block
	startDiff := CompactToBig(startDiffBits)
	if startDiff.Sign() <= 0 || startDiff.Cmp(powLimit) > 0 {
		panicf("starting difficulty %064x is not in the valid range [1, %064x]",
			startDiff, powLimit)
	}
	if heightDelta < 0 {
		panicf("provided height delta %d is negative", heightDelta)
	}

	// Calculate the target difficulty by multiplying the provided starting
	// target difficulty by an exponential scaling factor that is determined
	// based on how far ahead or behind the ideal schedule the given time delta
	// is along with a half life that acts as a smoothing factor.
	//
	// Per DCP0011, the goal equation is:
	//
	//   nextDiff = min(max(startDiff * 2^((Δt - Δh*Ib)/halfLife), 1), powLimit)
	//
	// However, in order to avoid the need to perform floating point math which
	// is problematic across languages due to uncertainty in floating point math
	// libs, the formula is implemented using a combination of fixed-point
	// integer arithmetic and a cubic polynomial approximation to the 2^x term.
	//
	// In particular, the goal cubic polynomial approximation over the interval
	// 0 <= x < 1 is:
	//
	//   2^x ~= 1 + 0.695502049712533x + 0.2262697964x^2 + 0.0782318x^3
	//
	// This approximation provides an absolute error margin < 0.013% over the
	// aforementioned interval of [0,1) which is well under the 0.1% error
	// margin needed for good results.  Note that since the input domain is not
	// constrained to that interval, the exponent is decomposed into an integer
	// part, n, and a fractional part, f, such that f is in the desired range of
	// [0,1).  By exponent rules 2^(n + f) = 2^n * 2^f, so the strategy is to
	// calculate the result by applying the cubic polynomial approximation to
	// the fractional part and using the fact that multiplying by 2^n is
	// equivalent to an arithmetic left or right shift depending on the sign.
	//
	// In other words, start by calculating the exponent (x) using 64.16 fixed
	// point and decompose it into integer (n) and fractional (f) parts as
	// follows:
	//
	//       2^16 * (Δt - Δh*Ib)   (Δt - Δh*Ib) << 16
	//   x = ------------------- = ------------------
	//            halfLife              halfLife
	//
	//        x
	//   n = ---- = x >> 16
	//       2^16
	//
	//   f = x (mod 2^16) = x & 0xffff
	//
	// The use of 64.16 fixed point for the exponent means both the integer (n)
	// and fractional (f) parts have an additional factor of 2^16.  Since the
	// fractional part of the exponent is cubed in the polynomial approximation
	// and (2^16)^3 = 2^48, the addition step in the approximation is internally
	// performed using 16.48 fixed point to compensate.
	//
	// In other words, the fixed point formulation of the goal cubic polynomial
	// approximation for the fractional part is:
	//
	//                 195766423245049*f + 971821376*f^2 + 5127*f^3 + 2^47
	//   2^f ~= 2^16 + ---------------------------------------------------
	//                                          2^48
	//
	// Finally, the final target difficulty is calculated using x.16 fixed point
	// and then clamped to the valid range as follows:
	//
	//              startDiff * 2^f * 2^n
	//   nextDiff = ---------------------
	//                       2^16
	//
	//   nextDiff = min(max(nextDiff, 1), powLimit)
	//
	// NOTE: The division by the half life uses Quo instead of Div because it
	// must be truncated division (which is truncated towards zero as Quo
	// implements) as opposed to the Euclidean division that Div implements.
	idealTimeDelta := heightDelta * targetSecsPerBlock
	exponentBig := big.NewInt(timeDelta - idealTimeDelta)
	exponentBig.Lsh(exponentBig, 16)
	exponentBig.Quo(exponentBig, big.NewInt(halfLife))

	// Decompose the exponent into integer and fractional parts.  Since the
	// exponent is using 64.16 fixed point, the bottom 16 bits are the
	// fractional part and the integer part is the exponent arithmetic right
	// shifted by 16.
	frac64 := uint64(exponentBig.Int64() & 0xffff)
	shifts := exponentBig.Rsh(exponentBig, 16).Int64()

	// Calculate 2^16 * 2^(fractional part) of the exponent.
	//
	// Note that a full unsigned 64-bit type is required to avoid overflow in
	// the internal 16.48 fixed point calculation.  Also, the overall result is
	// guaranteed to be positive and a maximum of 17 bits, so it is safe to cast
	// to a uint32.
	const (
		polyCoeff1 uint64 = 195766423245049 // ceil(0.695502049712533 * 2^48)
		polyCoeff2 uint64 = 971821376       // ceil(0.2262697964 * 2^32)
		polyCoeff3 uint64 = 5127            // ceil(0.0782318 * 2^16)
	)
	fracFactor := uint32(1<<16 + (polyCoeff1*frac64+
		polyCoeff2*frac64*frac64+
		polyCoeff3*frac64*frac64*frac64+
		1<<47)>>48)

	// Calculate the target difficulty per the previous discussion:
	//
	//              startDiff * 2^f * 2^n
	//   nextDiff = ---------------------
	//                       2^16
	//
	// Note that by exponent rules 2^n / 2^16 = 2^(n - 16).  This takes
	// advantage of that property to reduce the multiplication by 2^n and
	// division by 2^16 to a single shift.
	//
	// This approach also has the benefit of lowering the maximum magnitude
	// relative to what would be the case when first left shifting by a larger
	// value and then right shifting after.  Since arbitrary precision integers
	// are used for this implementation, it doesn't make any difference from a
	// correctness standpoint, however, it does potentially lower the amount of
	// memory for the arbitrary precision type and can be used to help prevent
	// overflow in implementations that use fixed precision types.
	nextDiff := new(big.Int).Set(startDiff)
	nextDiff.Mul(nextDiff, big.NewInt(int64(fracFactor)))
	shifts -= 16
	if shifts >= 0 {
		nextDiff.Lsh(nextDiff, uint(shifts))
	} else {
		nextDiff.Rsh(nextDiff, uint(-shifts))
	}

	// Limit the target difficulty to the valid hardest and easiest values.
	// The valid range is [1, powLimit].
	if nextDiff.Sign() == 0 {
		// The hardest valid target difficulty is 1 since it would be impossible
		// to find a non-negative integer less than 0.
		nextDiff.SetInt64(1)
	} else if nextDiff.Cmp(powLimit) > 0 {
		nextDiff.Set(powLimit)
	}

	// Convert the difficulty to the compact representation and return it.
	return BigToCompact(nextDiff)
}

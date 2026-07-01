// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

// constantTimeEq returns 1 if a == b or 0 otherwise in constant time.
func constantTimeEq(a, b uint32) uint32 {
	return uint32((uint64(a^b) - 1) >> 63)
}

// constantTimeEq64 returns 1 if a == b or 0 otherwise in constant time.
func constantTimeEq64(a, b uint64) uint32 {
	t := a ^ b
	return uint32(((t | -t) >> 63) ^ 1)
}

// constantTimeNotEq returns 1 if a != b or 0 otherwise in constant time.
func constantTimeNotEq(a, b uint32) uint32 {
	return ^uint32((uint64(a^b)-1)>>63) & 1
}

// constantTimeLess returns 1 if a < b or 0 otherwise in constant time.
func constantTimeLess(a, b uint32) uint32 {
	return uint32((uint64(a) - uint64(b)) >> 63)
}

// constantTimeLessOrEq returns 1 if a <= b or 0 otherwise in constant time.
func constantTimeLessOrEq(a, b uint32) uint32 {
	return uint32((uint64(a) - uint64(b) - 1) >> 63)
}

// constantTimeGreater returns 1 if a > b or 0 otherwise in constant time.
func constantTimeGreater(a, b uint32) uint32 {
	return constantTimeLess(b, a)
}

// constantTimeGreaterOrEq returns 1 if a >= b or 0 otherwise in constant time.
func constantTimeGreaterOrEq(a, b uint32) uint32 {
	return constantTimeLessOrEq(b, a)
}

// constantTimeMin returns min(a,b) in constant time.
func constantTimeMin(a, b uint32) uint32 {
	return b ^ ((a ^ b) & -constantTimeLess(a, b))
}

// constantTimeSelect64 returns a if cond == 1 or b if cond == 0 in constant
// time.
//
// WARNING: The behavior is undefined if cond is anything other than 0 or 1.
func constantTimeSelect64(cond, a, b uint64) uint64 {
	mask := -cond
	return b ^ (a^b)&mask
}

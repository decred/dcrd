// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package arith

// ConstantTimeEq returns 1 if a == b or 0 otherwise in constant time.
func ConstantTimeEq(a, b uint32) uint32 {
	return uint32((uint64(a^b) - 1) >> 63)
}

// ConstantTimeEq64 returns 1 if a == b or 0 otherwise in constant time.
func ConstantTimeEq64(a, b uint64) uint32 {
	t := a ^ b
	return uint32(((t | -t) >> 63) ^ 1)
}

// ConstantTimeNotEq64 returns 1 if a != b or 0 otherwise in constant time.
func ConstantTimeNotEq64(a, b uint64) uint32 {
	t := a ^ b
	return uint32((t | -t) >> 63)
}

// ConstantTimeLess returns 1 if a < b or 0 otherwise in constant time.
func ConstantTimeLess(a, b uint32) uint32 {
	return uint32((uint64(a) - uint64(b)) >> 63)
}

// ConstantTimeLessOrEq returns 1 if a <= b or 0 otherwise in constant time.
func ConstantTimeLessOrEq(a, b uint32) uint32 {
	return uint32((uint64(a) - uint64(b) - 1) >> 63)
}

// ConstantTimeGreater returns 1 if a > b or 0 otherwise in constant time.
func ConstantTimeGreater(a, b uint32) uint32 {
	return ConstantTimeLess(b, a)
}

// ConstantTimeGreaterOrEq returns 1 if a >= b or 0 otherwise in constant time.
func ConstantTimeGreaterOrEq(a, b uint32) uint32 {
	return ConstantTimeLessOrEq(b, a)
}

// ConstantTimeMin returns min(a,b) in constant time.
func ConstantTimeMin(a, b uint32) uint32 {
	return b ^ ((a ^ b) & -ConstantTimeLess(a, b))
}

// ConstantTimeSelect64 returns a when cond == 1 or b when cond == 0 in constant
// time.
//
// WARNING: The behavior is undefined if cond is anything other than 0 or 1.
func ConstantTimeSelect64(cond, a, b uint64) uint64 {
	mask := -cond
	return b ^ (a^b)&mask
}

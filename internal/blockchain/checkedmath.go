// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

// addUnsigned returns the sum of the two unsigned ints of the same size and
// whether or not the result is safe to use (aka no overflow occurred).
func addUnsigned[T ~uint16 | ~uint32 | ~uint64](a, b T) (T, bool) {
	sum := a + b
	return sum, sum >= a
}

// addSigned returns the sum of the two signed ints of the same size and whether
// or not the result is safe to use (aka no overflow or underflow occurred).
func addSigned[T ~int16 | ~int32 | ~int64](a, b T) (T, bool) {
	// Overflow only occurs when adding a positive value when the sum is <= to
	// left summand.  Likewise, underflow only occurs when adding a non-positive
	// value when the sum is > the left summand.  The following is the logical
	// negation of the result of testing both conditions at once so the returned
	// flag indicates their absence.
	sum := a + b
	return sum, (sum > a) == (b > 0)
}

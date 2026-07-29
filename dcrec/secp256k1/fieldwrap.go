// Copyright (c) 2015-2026 The Decred developers
// Copyright (c) 2013-2026 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

// ----------------------------------------------------------------------------
// NOTE: [FieldVal] is intentionally a wrapper as opposed to a direct type
// alias despite it creating some boilerplate.
//
// Some field implementations have more restrictive semantics, so the publicly
// available type must adhere to the most restrictive among all supported ones.
//
// Using a type alias would end up showing the documentation for the
// architecture-specific implementation that is ultimately selected which would
// very likely lead to misleading documentation and incorrect usage.
// ----------------------------------------------------------------------------

// FieldVal implements optimized fixed-precision arithmetic over the
// secp256k1 finite field.  This means all arithmetic is performed modulo
//
//	0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f
//
// WARNING: Since it is so important for the field arithmetic to be extremely
// fast for high performance crypto, this type does not perform any validation
// of documented preconditions where it ordinarily would.  As a result, it is
// IMPERATIVE for callers to understand some key concepts that are described
// below and ensure the methods are called with the necessary preconditions that
// each method is documented with.  For example, some methods only give the
// correct result if the field element is normalized and others require the
// field elements involved to have a maximum magnitude and THERE ARE NO EXPLICIT
// CHECKS TO ENSURE THOSE PRECONDITIONS ARE SATISFIED.  This does,
// unfortunately, make the type more difficult to use correctly and while it is
// typically preferable to ensure all state and input is valid for most code,
// this is a bit of an exception because those extra checks really add up in
// what ends up being critical hot paths.
//
// The first key concept when working with this type is normalization.  In order
// to avoid the need to propagate a ton of carries, the internal representation
// provides additional overflow bits for each limb of the overall 256-bit value.
// This means that there are multiple internal representations for the same
// value and, as a result, any methods that rely on comparison of the value,
// such as equality and oddness determination, require the caller to provide a
// normalized field element.
//
// The second key concept when working with this type is magnitude.  As
// previously mentioned, the internal representation provides additional
// overflow bits which means that the more math operations that are performed on
// the field element between normalizations, the more those overflow bits
// accumulate.  The magnitude is effectively that maximum possible number of
// those overflow bits that could possibly be required as a result of a given
// operation.  Since there are only a limited number of overflow bits available,
// this implies that the max possible magnitude MUST be tracked by the caller
// and the caller MUST normalize the field element if a given operation would
// cause the magnitude of the result to exceed the max allowed value.
//
// IMPORTANT: The max allowed magnitude of a field element is 32.
type FieldVal struct {
	impl fieldImpl
}

// String returns the field element as a normalized human-readable hex string.
//
//	Preconditions: None
//	Output Normalized: Field is not modified -- same as input element
//	Output Max Magnitude: Field is not modified -- same as input element
func (f FieldVal) String() string {
	return f.impl.String()
}

// Zero sets the field element to zero in constant time.  A newly created field
// element is already set to zero.  This function can be useful to clear an
// existing field element for reuse.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (f *FieldVal) Zero() {
	f.impl.Zero()
}

// Set sets the field element equal to the passed one in constant time.  The
// resulting field element will have the same normalization state and magnitude
// as the passed field element.
//
// The field element is returned to support chaining.  This enables syntax like:
// f := new(FieldVal).Set(f2).Add(1) so that f = f2 + 1 where f2 is not
// modified.
//
//	Preconditions: None
//	Output Normalized: Same as input element
//	Output Max Magnitude: Same as input element
func (f *FieldVal) Set(val *FieldVal) *FieldVal {
	f.impl.Set(&val.impl)
	return f
}

// SetInt sets the field element to the passed integer in constant time.  This
// is a convenience function since it is fairly common to perform arithmetic
// with small native integers.
//
// The field element is returned to support chaining.  This enables syntax such
// as f := new(FieldVal).SetInt(2).Mul(f2) so that f = 2 * f2.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (f *FieldVal) SetInt(ui uint16) *FieldVal {
	f.impl.SetInt(ui)
	return f
}

// SetBytes packs the passed 32-byte big-endian value into the internal
// representation in constant time.  It interprets the provided array as a
// 256-bit big-endian unsigned integer, packs it, and returns either 1 if it is
// greater than or equal to the field prime (aka it overflowed) or 0 otherwise
// in constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.
//
//	Preconditions: None
//	Output Normalized: Yes when no overflow, No otherwise
//	Output Max Magnitude: 1
func (f *FieldVal) SetBytes(b *[32]byte) uint32 {
	return f.impl.SetBytes(b)
}

// SetByteSlice interprets the provided slice as a 256-bit big-endian unsigned
// integer (meaning it is truncated to the first 32 bytes), packs it into the
// internal representation, and returns whether or not the resulting truncated
// 256-bit integer is greater than or equal to the field prime (aka it
// overflowed) in constant time.
//
// Note that since passing a slice with more than 32 bytes is truncated, it is
// possible that the truncated value is less than the field prime and hence it
// will not be reported as having overflowed in that case.  It is up to the
// caller to decide whether it needs to provide numbers of the appropriate size
// or it if is acceptable to use this function with the described truncation and
// overflow behavior.
//
//	Preconditions: None
//	Output Normalized: Yes when no overflow, No otherwise
//	Output Max Magnitude: 1
func (f *FieldVal) SetByteSlice(b []byte) bool {
	return f.impl.SetByteSlice(b)
}

// Normalize converts the internal representation into its canonical
// representation and performs modular reduction over the secp256k1 field prime
// in constant time.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (f *FieldVal) Normalize() *FieldVal {
	f.impl.Normalize()
	return f
}

// PutBytesUnchecked unpacks the field element to a 32-byte big-endian value
// directly into the passed byte slice in constant time.  The target slice must
// have at least 32 bytes available or it will panic.
//
// There is a similar function, [FieldVal.PutBytes], which unpacks the field
// element into a 32-byte array directly.  This version is provided since it can
// be useful to write directly into part of a larger buffer without needing a
// separate allocation.
//
//	Preconditions:
//	  - The field element MUST be normalized
//	  - The target slice MUST have at least 32 bytes available
func (f *FieldVal) PutBytesUnchecked(b []byte) {
	f.impl.PutBytesUnchecked(b)
}

// PutBytes unpacks the field element to a 32-byte big-endian value using the
// passed byte array in constant time.
//
// There is a similar function, [FieldVal.PutBytesUnchecked], which unpacks the
// field element into a slice that must have at least 32 bytes available.  This
// version is provided since it can be useful to write directly into an array
// that is type checked.
//
// Alternatively, there is also [FieldVal.Bytes], which unpacks the field
// element into a new array and returns that which can sometimes be more
// ergonomic in applications that aren't concerned about an additional copy.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) PutBytes(b *[32]byte) {
	f.impl.PutBytes(b)
}

// Bytes unpacks the field element to a 32-byte big-endian value in constant
// time.
//
// See [FieldVal.PutBytes] and [FieldVal.PutBytesUnchecked] for variants that
// allow an array or slice to be passed which can be useful to cut down on the
// number of allocations by allowing the caller to reuse a buffer or write
// directly into part of a larger buffer.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) Bytes() *[32]byte {
	return f.impl.Bytes()
}

// IsZeroBit returns 1 when the field element is equal to zero or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [FieldVal.IsZero] for the version
// that returns a bool.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsZeroBit() uint32 {
	return f.impl.IsZeroBit()
}

// IsZero returns whether or not the field element is equal to zero in constant
// time.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsZero() bool {
	return f.impl.IsZero()
}

// IsOneBit returns 1 when the field element is equal to one or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [FieldVal.IsOne] for the version
// that returns a bool.
//
//	Preconditions:
//	   - The field element MUST be normalized
func (f *FieldVal) IsOneBit() uint32 {
	return f.impl.IsOneBit()
}

// IsOne returns whether or not the field element is equal to one in constant
// time.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsOne() bool {
	return f.impl.IsOne()
}

// IsOddBit returns 1 when the field element is an odd number or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [FieldVal.IsOdd] for the version
// that returns a bool.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsOddBit() uint32 {
	return f.impl.IsOddBit()
}

// IsOdd returns whether or not the field element is an odd number in constant
// time.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsOdd() bool {
	return f.impl.IsOdd()
}

// Equals returns whether or not the two field elements are the same in constant
// time.
//
//	Preconditions:
//	 - Both field elements being compared MUST be normalized
func (f *FieldVal) Equals(val *FieldVal) bool {
	return f.impl.Equals(&val.impl)
}

// NegateVal negates the passed element and stores the result in f in constant
// time.  The caller must provide the maximum magnitude of the passed field
// element for a correct result.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.NegateVal(f2).AddInt(1) so that f = -f2 + 1.
//
//	Preconditions:
//	  - The max magnitude of the input field element MUST be 31
//	Output Normalized: No
//	Output Max Magnitude: Input magnitude + 1
func (f *FieldVal) NegateVal(val *FieldVal, magnitude uint32) *FieldVal {
	f.impl.NegateVal(&val.impl, magnitude)
	return f
}

// Negate negates the field element in constant time.  The existing field
// element is modified.  The caller must provide the maximum magnitude of the
// field element for a correct result.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.Negate().AddInt(1) so that f = -f + 1.
//
//	Preconditions:
//	  - The max magnitude MUST be 31
//	Output Normalized: No
//	Output Max Magnitude: Input magnitude + 1
func (f *FieldVal) Negate(magnitude uint32) *FieldVal {
	f.impl.Negate(magnitude)
	return f
}

// AddInt adds the passed integer to the existing field element and stores the
// result in f in constant time.  This is a convenience function since it is
// fairly common to perform some arithmetic with small native integers.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.AddInt(1).Add(f2) so that f = f + 1 + f2.
//
//	Preconditions:
//	  - The field element MUST have a max magnitude of 31
//	  - The integer MUST be at most 32767
//	Output Normalized: No
//	Output Max Magnitude: Existing field magnitude + 1
func (f *FieldVal) AddInt(ui uint16) *FieldVal {
	f.impl.AddInt(ui)
	return f
}

// Add adds the passed element to the existing field element and stores the
// result in f in constant time.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.Add(f2).AddInt(1) so that f = f + f2 + 1.
//
//	Preconditions:
//	  - The sum of the magnitudes of the two field elements MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Sum of the magnitude of the two individual field elements
func (f *FieldVal) Add(val *FieldVal) *FieldVal {
	f.impl.Add(&val.impl)
	return f
}

// Add2 adds the passed two field elements together and stores the result in f
// in constant time.
//
// The field element is returned to support chaining.  This enables syntax like:
// f3.Add2(f, f2).AddInt(1) so that f3 = f + f2 + 1.
//
//	Preconditions:
//	  - The sum of the magnitudes of the two field elements MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Sum of the magnitude of the two field elements
func (f *FieldVal) Add2(val *FieldVal, val2 *FieldVal) *FieldVal {
	f.impl.Add2(&val.impl, &val2.impl)
	return f
}

// MulBy2 multiplies the field element by 2 and stores the result in f in
// constant time.  Note that this function can overflow if multiplying the
// element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.MulBy2().Add(f2) so that f = 2 * f + f2.
//
//	Preconditions:
//	  - The field element magnitude multiplied by 2 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing field magnitude times 2
func (f *FieldVal) MulBy2() *FieldVal {
	f.impl.MulBy2()
	return f
}

// MulBy3 multiplies the field element by 3 and stores the result in f in
// constant time.  Note that this function can overflow if multiplying the
// element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.MulBy3().Add(f2) so that f = 3 * f + f2.
//
//	Preconditions:
//	  - The field element magnitude multiplied by 3 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing field element magnitude times 3
func (f *FieldVal) MulBy3() *FieldVal {
	f.impl.MulBy3()
	return f
}

// MulBy4 multiplies the field element by 4 and stores the result in f in
// constant time.  Note that this function can overflow if multiplying the
// element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.MulBy4().Add(f2) so that f = 4 * f + f2.
//
//	Preconditions:
//	  - The field element magnitude multiplied by 4 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing field element magnitude times 4
func (f *FieldVal) MulBy4() *FieldVal {
	f.impl.MulBy4()
	return f
}

// MulBy8 multiplies the field element by 8 and stores the result in f in
// constant time.  Note that this function can overflow if multiplying the
// element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.MulBy8().Add(f2) so that f = 8 * f + f2.
//
//	Preconditions:
//	  - The field element magnitude multiplied by 8 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing field element magnitude times 8
func (f *FieldVal) MulBy8() *FieldVal {
	f.impl.MulBy8()
	return f
}

// MulInt multiplies the field element by the passed int and stores the result
// in f in constant time.  Note that this function can overflow if multiplying
// the element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// Callers should prefer using the specialized methods for multiplying by 2, 3,
// 4, and 8, as they are commonly used in curve equations.
//
// See [FieldVal.MulBy2], [FieldVal.MulBy3], [FieldVal.MulBy4], and
// [FieldVal.MulBy8] for the aforementioned specialized methods.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.MulInt(2).Add(f2) so that f = 2 * f + f2.
//
//	Preconditions:
//	  - The field element magnitude multiplied by given val MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing field element magnitude times the provided integer
func (f *FieldVal) MulInt(val uint8) *FieldVal {
	f.impl.MulInt(val)
	return f
}

// Mul multiplies the passed element to the existing field element and stores
// the result in f in constant time.  Note that this function can overflow if
// multiplying causes any individual limb to overflow uint32.  In practice, this
// means the magnitude of either element involved in the multiplication must be
// at most 8.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.Mul(f2).AddInt(1) so that f = (f * f2) + 1.
//
//	Preconditions:
//	  - Both field elements MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) Mul(val *FieldVal) *FieldVal {
	f.impl.Mul(&val.impl)
	return f
}

// Mul2 multiplies the passed two field elements together and stores the result
// in f in constant time.  Note that this function can overflow if multiplying
// any of the individual limbs exceeds a max uint32.  In practice, this means
// the magnitude of either element involved in the multiplication must be a max
// of 8.
//
// The field element is returned to support chaining.  This enables syntax like:
// f3.Mul2(f, f2).AddInt(1) so that f3 = (f * f2) + 1.
//
//	Preconditions:
//	  - Both input field elements MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) Mul2(val *FieldVal, val2 *FieldVal) *FieldVal {
	f.impl.Mul2(&val.impl, &val2.impl)
	return f
}

// SquareRootVal either calculates the square root of the passed element when it
// exists or the square root of the negation of the element when it does not
// exist and stores the result in f in constant time.  The return flag is true
// when the calculated square root is for the passed element itself and false
// when it is for its negation.
//
// Note that this function can overflow if multiplying any of the individual
// limbs exceeds a max uint32.  In practice, this means the magnitude of the
// field must be at most 8 to prevent overflow.  The magnitude of the result
// will be 1.
//
//	Preconditions:
//	  - The input field element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) SquareRootVal(val *FieldVal) bool {
	return f.impl.SquareRootVal(&val.impl)
}

// Square squares the field element in constant time.  The existing field
// element is modified.  Note that this function can overflow if multiplying any
// of the individual limbs exceeds a max uint32.  In practice, this means the
// magnitude of the field element must be at most 8 to prevent overflow.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.Square().Mul(f2) so that f = f^2 * f2.
//
//	Preconditions:
//	  - The field element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) Square() *FieldVal {
	f.impl.Square()
	return f
}

// SquareVal squares the passed element and stores the result in f in constant
// time.  Note that this function can overflow if multiplying any of the
// individual limbs exceeds a max uint32.  In practice, this means the magnitude
// of the field element being squared must be at most 8 to prevent overflow.
//
// The field element is returned to support chaining.  This enables syntax like:
// f3.SquareVal(f).Mul(f) so that f3 = f^2 * f = f^3.
//
//	Preconditions:
//	  - The input field element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) SquareVal(val *FieldVal) *FieldVal {
	f.impl.SquareVal(&val.impl)
	return f
}

// Inverse finds the modular multiplicative inverse of the field element in
// constant time.  The existing field element is modified.
//
// The field element is returned to support chaining.  This enables syntax like:
// f.Inverse().Mul(f2) so that f = f^-1 * f2.
//
//	Preconditions:
//	  - The field element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (f *FieldVal) Inverse() *FieldVal {
	f.impl.Inverse()
	return f
}

// IsGtOrEqPrimeMinusOrder returns whether or not the field element is greater
// than or equal to the field prime minus the secp256k1 group order in constant
// time.
//
//	Preconditions:
//	  - The field element MUST be normalized
func (f *FieldVal) IsGtOrEqPrimeMinusOrder() bool {
	return f.impl.IsGtOrEqPrimeMinusOrder()
}

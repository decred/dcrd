uint256
=======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/math/uint256)

## Fixed Precision Unsigned 256-bit Integer Arithmetic

This package implements highly optimized allocation free fixed precision
unsigned 256-bit integer arithmetic.

The following is a brief overview of the main features and benefits:

- Strong focus on performance and correctness
  - Every operation is faster than the stdlib `big.Int` equivalent and most
    operations, including the primary math operations, are **significantly** faster
    ([see performance comparison benchmarks](#uint256-performance-comparison))
- Allocation free
  - All non-formatting operations with the specialized type are allocation free
- Supports boolean comparison, bitwise logic, and bitwise shift operations
- All operations are performed modulo 2^256
- Ergonomic API with unary-style arguments as well as some binary variants
    - eg: `n.SetUint64(1).Lsh(64).SubUint64(1)` is equivalent to `n = 1<<64 - 1`
    - eg: `n.Div2(n1, n2)` is equivalent to `n = n1/n2`
- Conversion-free support for interoperation with native `uint64` integers
- Direct conversion to and from little and big endian byte arrays
- Full support for formatted output and common base conversions
  - Producing formatted output uses fewer allocations than `big.Int`
- 100% test coverage
- Comprehensive benchmarks

The primary `big.Int` operations that are **NOT** currently provided:

- Modular arithmetic with moduli other than 2^256
- Signed arithmetic
  - A caller could fairly easily implement it with 2's complement if desired
    since `Negate` is provided
- Remainders from division

These operations may be implemented in the future if the need arises.

### Note About Standard Library `math/big.Int` Conversions

This package provides convenience methods for converting to and from standard
library `big.Int`s, however, callers should make an active effort to avoid that
unless it is absolutely necessary as conversion to `big.Int` requires an
unavoidable allocation in most circumstances and `big.Int` is also slower for
every operation, often to a significant degree.  Further, `big.Int`s often cause
further allocations while performing arithmetic, notably multiplication and
division.

Regarding the aforementioned allocations, standard library `big.Int`s internally
require heap allocations to operate, so conversion to a new `big.Int`
necessarily causes an allocation.  This means the `ToBig` method will always
incur at least one allocation.

One feature of `big.Int`s is that they attempt to reuse their internal buffer
when possible, so the number of allocations due to conversion can sometimes be
reduced if reusing the same variable is an option.  This package provides a
`PutBig` method which allows callers to reuse an existing `big.Int` for this
purpose.

Do note however that even when reusing a `big.Int`, it will naturally still
require an allocation for the internal buffer unless it has already previously
allocated one large enough to be reused.

## Categorized `Uint256` Summary

This section summarizes the majority of the available operations provided by
category.  See the full documentation for additional details about individual
methods and additional available methods.

### Arithmetic Methods

Operation       | Native Equiv | Methods
----------------|--------------|-------------------
Add Assign      | `n += x`     | `Add`, `AddUint64`
Add             | `n = x + y`  | `Add2`
Subtract Assign | `n -= x`     | `Sub`, `SubUint64`
Subtract        | `n = x - y`  | `Sub2`
Multiply Assign | `n *= x`     | `Mul`, `MulUint64`
Multiply        | `n = x * y`  | `Mul2`
Divide Assign   | `n /= x`     | `Div`
Divide          | `n = x / y`  | `Div2`
Square Assign   | `n *= n`     | `Square`
Square          | `n = x * x`  | `SquareVal`
Negate Assign   | `n = -n`     | `Negate`
Negate          | `n = -x`     | `NegateVal`

### Comparison Methods

Operation        | Native Equiv | Methods
-----------------|--------------|---------------------
Equality         | `x == y`     | `Eq`, `EqUint64`
Less Than        | `x < y`      | `Lt`, `LtUint64`
Less Or Equal    | `x <= y`     | `LtEq`, `LtEqUint64`
Greater Than     | `x > y`      | `Gt`, `GtUint64`
Greater Or Equal | `x >= y`     | `GtEq`, `GtEqUint64`
Comparison       | n/a          | `Cmp`, `CmpUint64`

### Bitwise Methods

Operation          | Native Equiv  | Method
-------------------|---------------|---------
And Assign         | `n &= x`      | `And`
Or Assign          | `n \|= x`     | `Or`
Xor Assign         | `n ^= x`      | `Xor`
Not Assign         | `n = ^x`      | `Not`
Left Shift Assign  | `n <<= x`     | `Lsh`
Left Shift         | `n = x << y`  | `LshVal`
Right Shift Assign | `n >>= x`     | `Rsh`
Right Shift        | `n = x >> y`  | `RshVal`
Bit Length         | n/a           | `BitLen`

### Conversion Methods

Operation          | Methods
-------------------|-----------------------------------------------
From Big Endian    | `SetBytes`, `SetByteSlice`
To Big Endian      | `Bytes`, `PutBytes`, `PutBytesUnchecked`
From Little Endian | `SetBytesLE`, `SetByteSliceLE`
To Little Endian   | `BytesLE`, `PutBytesLE`, `PutBytesUncheckedLE`
From `math/big.Int`| `SetBig`
To `math/big.Int`  | `ToBig`, `PutBig`

### Misc Convenience Methods

Operation                    | Method
-----------------------------|-----------
Can represent with `uint32`? | `IsUint32`
Value modulo 2^32            | `Uint32`
Can represent with `uint64`? | `IsUint64`
Value modulo 2^64            | `Uint64`
Set to 0                     | `Zero`
Is equal to zero?            | `IsZero`
Is the value odd?            | `IsOdd`

### Output Formatting Methods

Operation                                    | Method
---------------------------------------------|---------
Binary/Octal/Decimal/Hex                     | `Text`
Standard Formatted output (`fmt.Formatter`)  | `Format`
Standard Unformatted output (`fmt.Stringer`) | `String`

## Uint256 Performance Comparison

The following benchmark results demonstrate the performance of most operations
as compared to standard library `big.Int`s.  The benchmarks are from a Ryzen 7
1700 processor and are the result of feeding `benchstat` 10 iterations of each.

### Arithmetic Methods

Name            | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
----------------|-------------------|-------------------|-------------------
Add             |     158ns ± 2%    |       2ns ± 1%    | -98.67%
AddUint64       |    44.4ns ± 3%    |     3.4ns ± 2%    | -92.27%
Sub             |    53.9ns ± 1%    |     2.1ns ± 1%    | -96.12%
SubUint64       |    44.8ns ± 1%    |     3.4ns ± 2%    | -92.37%
Mul             |     419ns ± 1%    |      10ns ± 2%    | -97.64%
MulUint64       |     263ns ± 1%    |       4ns ± 1%    | -98.30%
Square          |     418ns ± 0%    |       7ns ± 2%    | -98.39%
Div/num_lt_den  |    75.4ns ± 1%    |     3.4ns ± 1%    | -95.51%
Div/num_eq_den  |     253ns ± 2%    |       4ns ± 3%    | -98.56%
Div/1_by_1_near |    53.8ns ± 2%    |     4.5ns ± 2%    | -91.63%
Div/1_by_1_far  |    31.4ns ± 2%    |    14.6ns ± 2%    | -53.64%
Div/2_by_1_near |    36.9ns ± 1%    |    10.1ns ± 2%    | -72.63%
Div/2_by_1_far  |    49.1ns ± 1%    |    28.8ns ± 1%    | -41.29%
Div/3_by_1_near |    43.2ns ± 1%    |    13.7ns ± 3%    | -68.24%
Div/3_by_1_far  |    57.0ns ± 1%    |    43.6ns ± 1%    | -23.59%
Div/4_by_1_near |    49.7ns ± 4%    |    18.0ns ± 1%    | -63.87%
Div/4_by_1_far  |    65.2ns ± 4%    |    57.8ns ± 2%    | -11.41%
Div/2_by_2_near |     237ns ± 1%    |      22ns ± 3%    | -90.81%
Div/2_by_2_far  |     237ns ± 1%    |      30ns ± 3%    | -87.17%
Div/3_by_2_near |     258ns ± 1%    |      29ns ± 1%    | -88.60%
Div/3_by_2_far  |     257ns ± 1%    |      50ns ± 2%    | -80.42%
Div/4_by_2_near |     312ns ± 2%    |      40ns ± 3%    | -87.27%
Div/4_by_2_far  |     310ns ± 1%    |      71ns ± 3%    | -77.19%
Div/3_by_3_near |     239ns ± 2%    |      21ns ± 2%    | -91.39%
Div/3_by_3_far  |     242ns ± 4%    |      33ns ± 3%    | -86.33%
Div/4_by_3_near |     279ns ± 6%    |      31ns ± 1%    | -89.01%
Div/4_by_3_far  |     271ns ± 1%    |      46ns ± 3%    | -82.99%
Div/4_by_4_near |     252ns ± 3%    |      20ns ± 3%    | -91.99%
Div/4_by_4_far  |     249ns ± 2%    |      36ns ± 2%    | -85.65%
DivRandom       |     202ns ± 1%    |      23ns ± 1%    | -88.43%
DivUint64       |     129ns ± 1%    |      47ns ± 0%    | -63.34%
Negate          |    47.3ns ± 2%    |     1.5ns ± 2%    | -96.91%

### Comparison Methods

Name      | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
----------|-------------------|-------------------|-------------------
Eq        |    12.7ns ± 1%    |     2.1ns ± 1%    | -83.72%
Lt        |    12.6ns ± 1%    |     3.0ns ± 1%    | -75.96%
Gt        |    12.6ns ± 1%    |     3.0ns ± 1%    | -75.91%
Cmp       |    12.6ns ± 1%    |     7.7ns ± 1%    | -39.01%
CmpUint64 |    5.93ns ± 2%    |    3.70ns ± 1%    | -37.60%

### Bitwise Methods

Name            | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
----------------|-------------------|-------------------|-------------------
Lsh/bits_0      |    7.15ns ± 3%    |    2.58ns ± 1%    | -63.94%
Lsh/bits_1      |    14.8ns ± 1%    |     4.2ns ± 1%    | -71.40%
Lsh/bits_64     |    16.7ns ± 1%    |     2.7ns ± 1%    | -84.00%
Lsh/bits_128    |    16.9ns ± 2%    |     2.7ns ± 0%    | -84.21%
Lsh/bits_192    |    16.6ns ± 1%    |     2.6ns ± 1%    | -84.19%
Lsh/bits_255    |    16.3ns ± 2%    |     2.8ns ± 2%    | -83.11%
Lsh/bits_256    |    16.9ns ± 2%    |     2.6ns ± 2%    | -84.77%
Rsh/bits_0      |    8.76ns ± 2%    |    2.57ns ± 1%    | -70.63%
Rsh/bits_1      |    14.4ns ± 2%    |     4.3ns ± 2%    | -70.28%
Rsh/bits_64     |    12.8ns ± 1%    |     2.9ns ± 2%    | -77.31%
Rsh/bits_128    |    11.8ns ± 0%    |     2.9ns ± 2%    | -75.51%
Rsh/bits_192    |    10.5ns ± 2%    |     2.6ns ± 1%    | -75.17%
Rsh/bits_255    |    10.5ns ± 3%    |     2.8ns ± 2%    | -73.89%
Rsh/bits_256    |    5.50ns ± 1%    |    2.58ns ± 2%    | -53.15%
Not             |    25.4ns ± 2%    |     3.3ns ± 2%    | -86.79%
Or              |    17.9ns ± 5%    |     3.4ns ± 6%    | -80.94%
And             |    16.7ns ± 2%    |     3.4ns ± 0%    | -79.93%
Xor             |    17.9ns ± 1%    |     3.4ns ± 2%    | -80.91%
BitLen/bits_64  |    2.24ns ± 1%    |    1.94ns ± 3%    | -13.04%
BitLen/bits_128 |    2.25ns ± 2%    |    1.96ns ± 2%    | -13.17%
BitLen/bits_192 |    2.25ns ± 1%    |    1.60ns ± 1%    | -28.65%
BitLen/bits_255 |    2.26ns ± 2%    |    1.61ns ± 1%    | -29.04%

### Conversion Methods

Name       | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
-----------|-------------------|-------------------|-------------------
SetBytes   |    9.09ns ±13%    |    3.05ns ± 1%    | -66.43%
SetBytesLE |    59.9ns ± 4%    |     3.1ns ± 2%    | -94.76%
Bytes      |    61.3ns ± 1%    |    13.8ns ± 3%    | -77.49%
BytesLE    |    83.5ns ± 2%    |    13.9ns ± 2%    | -83.32%

### Misc Convenience Methods

Name            | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
----------------|-------------------|-------------------|-------------------
Zero            |    2.99ns ± 2%    |    1.29ns ± 1%    | -56.82%
IsZero          |    1.78ns ± 0%    |    1.63ns ± 2%    | -8.23%
IsOdd           |    3.62ns ± 4%    |    1.64ns ± 1%    | -54.65%

### Output Formatting Methods

Name           | `big.Int` Time/Op | `Uint256` Time/Op | Delta vs `big.Int`
---------------|-------------------|-------------------|-------------------
Text/base_2    |     579ns ± 3%    |     496ns ± 2%    | -14.37%
Text/base_8    |     266ns ± 1%    |     227ns ± 1%    | -14.58%
Text/base_10   |     536ns ± 1%    |     458ns ± 2%    | -14.58%
Text/base_16   |     205ns ± 2%    |     180ns ± 4%    | -11.90%
Format/base_2  |     987ns ±15%    |     852ns ± 2%    | -13.64%
Format/base_8  |     620ns ± 6%    |     544ns ± 3%    | -12.31%
Format/base_10 |     888ns ± 1%    |     726ns ± 1%    | -18.25%
Format/base_16 |     565ns ± 1%    |     449ns ± 1%    | -20.41%

## Installation and Updating

This package is part of the `github.com/decred/dcrd/math/uint256` module.  Use
the standard go tooling for working with modules to incorporate it.

## Examples

* [Basic Usage](https://pkg.go.dev/github.com/decred/dcrd/math/uint256#example-package-BasicUsage)  
  Demonstrates calculating the result of a dividing a max unsigned 256-bit
  integer by a max unsigned 128-bit integer and outputting that result in hex
  with leading zeros.

## License

Package uint256 is licensed under the [copyfree](http://copyfree.org) ISC
License.
# Copyright (c) 2026 The Decred developers
# Copyright (c) 2026 Dave Collins
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

from z3_proof_helpers import *

# -------
# Inputs.
# -------

x0 = BitVec('x0', 64)
x1 = BitVec('x1', 64)
x2 = BitVec('x2', 64)
x3 = BitVec('x3', 64)
x4 = BitVec('x4', 64)
x5 = BitVec('x5', 64)
x6 = BitVec('x6', 64)
x7 = BitVec('x7', 64)

field64Prime0 = BitVecVal(0xfffffffefffffc2f, 64)
field64Prime1 = BitVecVal(0xffffffffffffffff, 64)
field64Prime2 = BitVecVal(0xffffffffffffffff, 64)
field64Prime3 = BitVecVal(0xffffffffffffffff, 64)
field64Prime = Concat(field64Prime3, field64Prime2, field64Prime1, field64Prime0)

twicePrime0 = BitVecVal(0xfffffffdfffff85e, 64)
twicePrime1 = BitVecVal(0xffffffffffffffff, 64)
twicePrime2 = BitVecVal(0xffffffffffffffff, 64)
twicePrime3 = BitVecVal(0xffffffffffffffff, 64)
twicePrime4 = ONE
twicePrime = Concat(twicePrime4, twicePrime3, twicePrime2, twicePrime1, twicePrime0)

field64PrimeComplement = BitVecVal(2**32+977, 64)

# ---------------
# Model the code.
# ---------------

discards = []

# first reduction: 512 bits -> 289 bits.
h, t0 = mul64(x4, field64PrimeComplement)

hi, lo = mul64(x5, field64PrimeComplement)
t1, carry = add64(lo, h, ZERO)
h, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

hi, lo = mul64(x6, field64PrimeComplement)
t2, carry = add64(lo, h, ZERO)
h, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

hi, lo = mul64(x7, field64PrimeComplement)
t3, carry = add64(lo, h, ZERO)
t4, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

t0, carry = add64(t0, x0, ZERO)
t1, carry = add64(t1, x1, carry)
t2, carry = add64(t2, x2, carry)
t3, carry = add64(t3, x3, carry)
t4 += carry

# Save for analysis.
t4_saved = t4

# second reduction: 289 bits -> t < 2p
h, t4 = mul64(t4, field64PrimeComplement)

t0, carry = add64(t0, t4, ZERO)
t1, carry = add64(t1, h, carry)
t2, carry = add64(t2, ZERO, carry)
t3, carry = add64(t3, ZERO, carry)

t4 = carry

# final reduction: t < 2p -> t < p
s0, borrow = sub64(t0, field64Prime0, ZERO)
s1, borrow = sub64(t1, field64Prime1, borrow)
s2, borrow = sub64(t2, field64Prime2, borrow)
s3, borrow = sub64(t3, field64Prime3, borrow)
_, borrow = sub64(t4, ZERO, borrow)
r0 = If(borrow == ONE, t0, s0)
r1 = If(borrow == ONE, t1, s1)
r2 = If(borrow == ONE, t2, s2)
r3 = If(borrow == ONE, t3, s3)

# ------
# Proofs.
# -------

# Discarded carries are never set.
prove_no_discarded_carries(discards)

# Top limb never exceeds the field complement after first reduction.
#
# Since the complement is 33 bits, this proves the value does not exceed
# 256 + 33 = 289 bits after the first reduction.
prove(ULE(t4_saved, field64PrimeComplement), "top limb after 1st reduction > complement")

# Value after second reduction is less than twice the prime (t < 2p).
t = Concat(t4, t3, t2, t1, t0)
prove(ULT(t, twicePrime), "value after 2nd reduction >= 2p")

# Fully reduced result is less than the prime (r < p).
r = Concat(r3, r2, r1, r0)
prove(ULT(r, field64Prime), "fully reduced result >= prime")

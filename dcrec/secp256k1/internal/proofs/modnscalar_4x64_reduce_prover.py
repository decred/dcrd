# Copyright (c) 2026 The Decred developers
# Copyright (c) 2026 Dave Collins
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

from z3_proof_helpers import *

# -------
# Inputs.
# -------

x  = BitVecs('x0 x1 x2 x3 x4 x5 x6 x7', 64)

# NOTE: The code in this section reuses the variable names from the Go code
# being proven for easy cross referencing.  However, the comments refer to them
# by shorter mathematical variables for clarity.

# Define N.
orderLimb0 = BitVecVal(0xbfd25e8cd0364141, 64)
orderLimb1 = BitVecVal(0xbaaedce6af48a03b, 64)
orderLimb2 = BitVecVal(0xfffffffffffffffe, 64)
orderLimb3 = BitVecVal(0xffffffffffffffff, 64)
order = Concat(orderLimb3, orderLimb2, orderLimb1, orderLimb0)

# Define 2N.
twiceOrderLimb0 = BitVecVal(0x7fa4bd19a06c8282, 64)
twiceOrderLimb1 = BitVecVal(0x755db9cd5e914077, 64)
twiceOrderLimb2 = BitVecVal(0xfffffffffffffffd, 64)
twiceOrderLimb3 = BitVecVal(0xffffffffffffffff, 64)
twiceOrderLimb4 = ONE
twiceOrder = Concat(
    twiceOrderLimb4, twiceOrderLimb3, twiceOrderLimb2,
    twiceOrderLimb1, twiceOrderLimb0
)

# Define 2**256 - N.
orderComplementLimb0 = BitVecVal(0x402da1732fc9bebf, 64)
orderComplementLimb1 = BitVecVal(0x4551231950b75fc4, 64)
orderComplementLimb2 = ONE
orderComplement = Concat(orderComplementLimb2, orderComplementLimb1, orderComplementLimb0)

# --------------------------------------------------------------------------
# Prove the constant definitions satisfy the required identities before
# trusting anything that uses them.
# --------------------------------------------------------------------------

# Prove twiceOrder = 2N.
order320 = ZeroExt(64, order)
prove(twiceOrder == order320 << 1, "twiceOrder != 2N")

# --------------------------------------------------------------------------
# Lemma R-identity: N + c == 2**256
#
# This lemma justifies replacing multiples of 2**256 with multiples of c when
# working modulo N:
#
# N + c ≡ 2**256 (mod N)
# => 0 + c ≡ 2**256 (mod N)
# => c ≡ 2**256 (mod N)
# --------------------------------------------------------------------------
orderComplement320 = ZeroExt(128, orderComplement)
prove(order320+orderComplement320 == BitVecVal(1, 320)<<256,
   "Lemma R-identity: N+c != 2**256")

# ---------------
# Model the code.
# ---------------

discards = []

# first reduction: 512 bits -> 385 bits.
h0, t0 = mul64(x[4], orderComplementLimb0)
h1, t1 = mul64(x[4], orderComplementLimb1)
t1, carry = add64(t1, h0, ZERO)
t2, carry = add64(x[4], h1, carry)
t3 = carry

h0, l0 = mul64(x[5], orderComplementLimb0)
h1, l1 = mul64(x[5], orderComplementLimb1)
t1, carry = add64(t1, l0, ZERO)
t2, carry = add64(t2, h0, carry)
t3, discarded = add64(t3, h1, carry)
discards.append(discarded)
t2, carry = add64(t2, l1, ZERO)
t3, carry = add64(t3, x[5], carry)
t4 = carry

h0, l0 = mul64(x[6], orderComplementLimb0)
h1, l1 = mul64(x[6], orderComplementLimb1)
t2, carry = add64(t2, l0, ZERO)
t3, carry = add64(t3, h0, carry)
t4, discarded = add64(t4, h1, carry)
discards.append(discarded)
t3, carry = add64(t3, l1, ZERO)
t4, carry = add64(t4, x[6], carry)
t5 = carry

h0, l0 = mul64(x[7], orderComplementLimb0)
h1, l1 = mul64(x[7], orderComplementLimb1)
t3, carry = add64(t3, l0, ZERO)
t4, carry = add64(t4, h0, carry)
t5, discarded = add64(t5, h1, carry)
discards.append(discarded)
t4, carry = add64(t4, l1, ZERO)
t5, carry = add64(t5, x[7], carry)
t6 = carry

t0, carry = add64(t0, x[0], ZERO)
t1, carry = add64(t1, x[1], carry)
t2, carry = add64(t2, x[2], carry)
t3, carry = add64(t3, x[3], carry)
t4, carry = add64(t4, ZERO, carry)
t5, carry = add64(t5, ZERO, carry)
t6, discarded = add64(t6, ZERO, carry)
discards.append(discarded)

# Save for analysis.
t6_saved_1 = t6
t_full_redux_1 = Concat(ZERO, t6, t5, t4, t3, t2, t1, t0)
first_redux_input_high = Concat(x[7], x[6], x[5], x[4])

# second reduction: 385 bits -> 258 bits.
h0, l0 = mul64(t4, orderComplementLimb0)
h1, l1 = mul64(t4, orderComplementLimb1)
l2 = t4
t0, carry = add64(t0, l0, ZERO)
t1, carry = add64(t1, h0, carry)
t2, carry = add64(t2, h1, carry)
t3, carry = add64(t3, ZERO, carry)
t4 = carry
t1, carry = add64(t1, l1, ZERO)
t2, carry = add64(t2, l2, carry)
t3, carry = add64(t3, ZERO, carry)
t4, discarded = add64(t4, ZERO, carry)
discards.append(discarded)

h0, l0 = mul64(t5, orderComplementLimb0)
h1, l1 = mul64(t5, orderComplementLimb1)
t1, carry = add64(t1, l0, ZERO)
t2, carry = add64(t2, h0, carry)
t3, carry = add64(t3, h1, carry)
t4, discarded = add64(t4, ZERO, carry)
discards.append(discarded)
t2, carry = add64(t2, l1, ZERO)
t3, carry = add64(t3, t5, carry)
t4, discarded = add64(t4, ZERO, carry)
discards.append(discarded)

t2, carry = add64(t2, t6*orderComplementLimb0, ZERO)
t3, carry = add64(t3, t6*orderComplementLimb1, carry)
t4, discarded = add64(t4, t6, carry)
discards.append(discarded)

# Save intermediate results for analysis.
t3_carry_2 = carry
t3_saved_2 = t3
t4_saved_2 = t4

# third reduction: max 258 bits -> t < 2N.
t0, carry = add64(t0, t4*orderComplementLimb0, ZERO)
t1, carry = add64(t1, t4*orderComplementLimb1, carry)
t2, carry = add64(t2, t4, carry)
t3, carry = add64(t3, ZERO, carry)
t4 = carry

# final reduction: t < 2N -> t < N
s0, borrow = sub64(t0, orderLimb0, ZERO)
s1, borrow = sub64(t1, orderLimb1, borrow)
s2, borrow = sub64(t2, orderLimb2, borrow)
s3, borrow = sub64(t3, orderLimb3, borrow)
_, borrow = sub64(t4, ZERO, borrow)
r0 = If(borrow == ONE, t0, s0)
r1 = If(borrow == ONE, t1, s1)
r2 = If(borrow == ONE, t2, s2)
r3 = If(borrow == ONE, t3, s3)

# -------
# Proofs.
# -------

# Discarded carries are never set.
prove_no_discarded_carries(discards)

# Top limb after first reduction is at most 1 (t6 ≤ 1).
#
# This along with the proof there is no discarded carry on t6 after the first
# reduction proves the value does not exceed 385 bits (as expected since the
# complement is 129 bits and there were a max of 256 bits in the upper limbs
# before the first reduction, so 256 + 129 = 385).
prove(ULE(t6_saved_1, ONE), "top limb after 1st reduction > 1")

# Top limb after second reduction is at most 3 (t4 ≤ 3).
#
# This along with the proof there is no discarded carry on t4 after the second
# reduction proves the value does not exceed 258 bits (as expected because the
# complement is 129 bits and there were 129 max bits left in the upper limbs
# after the first reduction, so 129 + 129 = 258).
prove(ULE(t4_saved_2, BitVecVal(3, 64)), "top limb after 2nd reduction > 3")

# Top limb after second reduction times complement limb 0 never exceeds uint64.
# ((t4*c0)>>64 == 0)
#
# This proves the regular 64-bit product used in the third reduction does not
# overflow.
t4c0hi, _ = mul64(t4_saved_2, orderComplementLimb0)
prove(
    t4c0hi == ZERO,
   "high word of top limb from 2nd reduction times complement limb 0 != 0",
)

# Top limb after second reduction times complement limb 1 never exceeds uint64.
# ((t4*c1)>>64 == 0)
#
# This proves the regular 64-bit product used in the third reduction does not
# overflow.
t4c1hi, _ = mul64(t4_saved_2, orderComplementLimb1)
prove(
    t4c1hi == ZERO,
   "high word of top limb from 2nd reduction times complement limb 1 != 0",
)

# Top limb after third reduction is at most 1 (t4 ≤ 1).
#
# This proof is not strictly necessary since the bound is actually tigher per
# the next proof which implicitly proves this fact.  However, it is fast to
# prove and including it provides nice symmetry with the previous proofs for the
# bounds of the top limb after each reduction.
prove(ULE(t4, ONE), "top limb after 3rd reduction > 1")

# Value after third reduction is less than twice the group order (t < 2N).
t = Concat(t4, t3, t2, t1, t0)
prove(ULT(t, twiceOrder), "value after 3rd reduction >= 2N")

# Fully reduced result is less than the group order (r < N).
r = Concat(r3, r2, r1, r0)
prove(ULT(r, order), "fully reduced result >= N")

# -----------------------------------------------------------------------------
# Prove functional congruence: r ≡ x (mod N).
#
# The checks above establish bounds and the safety of discarded carries,
# self-consistency of the constants, and the reduction identity (lemma
# R-identity).
#
# The following lemmas establish the arithmetic identity of each transformation
# stage.  Together with the previously proven bounds and reduction identity,
# they imply the overall congruence r ≡ x (mod N).
#
# A direct assertion such as t == x_lo + (x >> 256)*P is intractable because
# it introduces a monolithic 256x256 multiplier that appears nowhere in the
# modeled code, so the SAT solver is forced into bit-level equivalence
# checking between two structurally unrelated multiplier circuits.
#
# Instead, because every accumulation block in the implementation is a fixed
# carry-propagation network over the *outputs* of bits.Mul64, whether or not
# those 128-bit inputs are actually products is irrelevant to the network.  Each
# block satisfies its per-block identity for ALL 128-bit values.  So each block
# identity is proven here as a universal lemma over unconstrained variables,
# which contains no multipliers at all and reduces to adder-network equivalence
# that the solver dispatches quickly.
#
# The Go function, scalar64Reduce512, computes r in four stages:
#
# 1) First reduction: Fold the high 256 bits of x (x4..x7) into the low 256 bits
#    (x0..x3).  This produces t_1 = x_lo + x_hi*c which fits in 385 bits (7
#    64-bit limbs with the 7th limb small).
# 2) Second reduction: Fold t_1's 5th, 6th, and 7th limbs back in the same way.
#    This produces t_2 = t_1_lo + t_1_hi*c which fits in 258 bits (5 64-bit
#    limbs with the 7th limb small)
# 3) Third reduction: Fold t_2's 5th limb back in the same way.  This produces
#    t_3 = t_2_lo + t_1_hi*c, where t_3 < 2N.
# 4) Final reduction: One conditional subtraction of N which is valid because
#    t_3 < 2N.
#
# The lemmas below chain together to prove r ≡ t3 ≡ t2 ≡ t1 ≡ x (mod N) and 0 ≤
# r < N which is exactly the definition of x ≡ r (mod N).
def prove_functional_congruence_lemmas():
    """
    Prove r == x (mod N) for scalar64Reduce512.

    x is the 512-bit input (8 limbs x0..x7) and r is the fully-reduced 256-bit
    output (4 limbs).
    """

    # Use a wide enough congruence width that nothing in a 512-bit input ever
    # wraps around.  This is comfortably wider than the largest quantity
    # of x_hi*c < 2**385.
    CONGRUENCE_WIDTH = 512

    def cwidth(v):
        """Zero-extend v to CONGRUENCE_WIDTH bits."""
        return ZeroExt(CONGRUENCE_WIDTH - v.size(), v)

    def pack(*limbs):
        """
        Concatenate the provided big-endian 64-bit limbs into a single
        CONGRUENCE_WIDTH-bit value:

        limb0*2**(64*num_limbs) + limb1*2**(64*(num_limbs-1)) + ...
        """
        return cwidth(Concat(*limbs))

    def pack_limbs(limbs):
        """
        Concatenate the provided iterable little-endian 64-bit limbs into a
        single CONGRUENCE_WIDTH-bit value:

        limbs[0] + limbs[1]*2**64 + limbs[2]*2**128 + ...
        """
        return cwidth(Concat(*reversed(limbs)))

    def compose_mul_by_c(p0, p1, xj):
        """
        Compose the multi-limb product xj*c from the two 128-bit partial
        products produced by multiplying xj with the two complement limbs.

        Since the third complement limb is 1, the full product is:

            p0 + (p1 << 64) + (xj << 128)

        The result is equal to xj*c and is exactly how the implementation
        accumulates it.
        """
        return cwidth(p0) + (cwidth(p1)<<64) + (cwidth(xj)<<128)

    # Universal lemma variables shared by R-open, R-interior, and
    # R-second-interior.
    #
    # x_j models an arbitrary 64-bit input limb.
    #
    # p0 and p1 model the 128-bit products returned by bits.Mul64(x[j], c0) and
    # bits.Mul64(x[j], c1) (the lower two limbs of c) at some point in the real
    # code.  They are NOT tied to any specific x[j] or t[k].
    #
    # h0 and l0 are the upper and lower halves of p0, respectively.  Likewise h1
    # and l1 for p1.
    #
    # x_j_times_c is the composed value of x[j]*c.
    x_j = BitVec("x_j", 64)
    p0, p1 = BitVecs("p0 p1", 128)
    h0, l0 = split128(p0)
    h1, l1 = split128(p1)
    x_j_times_c = compose_mul_by_c(p0, p1, x_j)

    # -------------------------------------------------------------------------
    # Lemma R-open: The opening row of the first reduction (the x4 line).
    #
    #   h0, t0 = bits.Mul64(x[4], orderComplementLimb0)
    #   h1, t1 = bits.Mul64(x[4], orderComplementLimb1)
    #   t1, carry = bits.Add64(t1, h0, 0)
    #   t2, carry = bits.Add64(x[4], h1, carry)
    #   t3 = carry
    #
    # This only writes fresh limbs and has no discarded carries, so no
    # assumptions are needed.
    # -------------------------------------------------------------------------
    t0 = l0
    t1, carry = add64(l1, h0, ZERO)
    t2, carry = add64(x_j, h1, carry)
    t3 = carry
    prove(pack(t3,t2,t1,t0) == x_j_times_c, "R-open: state != x4*c")

    # -------------------------------------------------------------------------
    # Lemma R-interior: The shape shared by the x5, x6, and x7 blocks in the
    # first reduction.
    #
    #   h0, l0 = bits.Mul64(x[j], orderComplementLimb0)
    #   h1, l1 = bits.Mul64(x[j], orderComplementLimb1)
    #   t[j], carry = bits.Add64(t[j], l0, 0)
    #   t[j+1], carry = bits.Add64(t[j+1], h0, carry)
    #   t[j+2], _ = bits.Add64(t[j+2], h1, carry)
    #   t[j+1], carry = bits.Add64(t[j+1], l1, 0)
    #   t[j+2], carry = bits.Add64(t[j+2], x[j], carry)
    #   t[j+3] = carry
    #
    # a0..a3 model the incoming running total in t0..t3.
    #
    # One carry is discarded and assumed zero.  This fact is proven above by the
    # discard checks for the real x5/x6/x7 blocks, so this does not smuggle in a
    # new assumption.
    # -------------------------------------------------------------------------
    a = BitVecs("a0 a1 a2", 64)
    w = [None]*4
    w[0], carry = add64(a[0], l0, ZERO)
    w[1], carry = add64(a[1], h0, carry)
    w[2], discarded = add64(a[2], h1, carry)
    w[1], carry = add64(w[1], l1, ZERO)
    w[2], carry = add64(w[2], x_j, carry)
    w[3] = carry
    prove(
        pack_limbs(w) == pack_limbs(a) + x_j_times_c,
        "R-interior: block != state + xj*c",
        assumptions=[discarded == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-tail: Fold x0..x3 into running total after the blocks above.
    #
    #   t0, carry = bits.Add64(t0, x[0], 0)
    #   t1, carry = bits.Add64(t1, x[1], carry)
    #   ...
    #   t5, carry = bits.Add64(t5, 0, carry)
    #   t6, _ = bits.Add64(t6, 0, carry)
    #
    # b0..b6 model the incoming running total in t0..t6.
    #
    # The final line is modeled here as add64 with its own carry-out assumed
    # zero.  This fact is proven above by the discard checks for the real total,
    # so this does not smuggle in a new assumption.
    # -------------------------------------------------------------------------
    b = BitVecs("b0 b1 b2 b3 b4 b5 b6", 64)
    w = [None]*7
    w[0], carry = add64(b[0], x[0], ZERO)
    w[1], carry = add64(b[1], x[1], carry)
    w[2], carry = add64(b[2], x[2], carry)
    w[3], carry = add64(b[3], x[3], carry)
    w[4], carry = add64(b[4], ZERO, carry)
    w[5], carry = add64(b[5], ZERO, carry)
    w[6], discarded = add64(b[6], ZERO, carry)
    prove(
        pack_limbs(w) == pack_limbs(b) + pack_limbs(x[:4]),
        "R-tail: post-fold total != state + x_lo",
        assumptions=[discarded == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-second-interior: The carry-propagation shape shared by the t4 and
    # t5 blocks of the second reduction.
    #
    # Unlike R-interior, this block propagates carries through an existing fifth
    # accumulator limb rather than terminating with a new carry limb.
    #
    #   h0, l0 = bits.Mul64(t[k], orderComplementLimb0)
    #   h1, l1 = bits.Mul64(t[k], orderComplementLimb1)
    #   // h2, l2 = bits.Mul64(t[k], orderComplementLimb2) => h2=0, l2=t[k]
    #   t[j], carry = bits.Add64(t[j], l0, 0)
    #   t[j+1], carry = bits.Add64(t[j+1], h0, carry)
    #   ...
    #   t[j+4], _ = bits.Add64(t[j+4], 0, carry)
    #
    # d0..d4 model the incoming running total in t0..t4.
    #
    # d4 is zero when instantiated for the t4 block.
    #
    # The t4 additions are modeled as add64 with the carry-out assumed zero.
    # This fact is proven above by the discard checks for the real totals, so
    # this does not smuggle in a new assumption.
    # -------------------------------------------------------------------------
    d = BitVecs("d0 d1 d2 d3 d4", 64)
    w = [None]*5
    w[0], carry = add64(d[0], l0, ZERO)
    w[1], carry = add64(d[1], h0, carry)
    w[2], carry = add64(d[2], h1, carry)
    w[3], carry = add64(d[3], ZERO, carry)
    w[4], discarded1 = add64(d[4], ZERO, carry)
    w[1], carry = add64(w[1], l1, ZERO)
    w[2], carry = add64(w[2], x_j, carry)
    w[3], carry = add64(w[3], ZERO, carry)
    w[4], discarded2 = add64(w[4], ZERO, carry)
    prove(
        pack_limbs(w) == pack_limbs(d) + x_j_times_c,
        "R-second-interior: block != state + xj*c",
        assumptions=[discarded1 == ZERO, discarded2 == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-second-tail: The t6 block of the second reduction.
    #
    #   t2, carry = bits.Add64(t2, t6*orderComplementLimb0, 0)
    #   t3, carry = bits.Add64(t3, t6*orderComplementLimb1, carry)
    #   t4, _ = bits.Add64(t4, t6, carry)
    #
    # f0..f2 model the incoming running total in t2..t4.
    #
    # t6 models the incoming carry limb produced by the preceding block.
    #
    # The real implementation uses plain (truncating) 64-bit products here rather
    # than bits.Mul64.  Since t6 ≤ 1, these products are exact, so modeling them
    # as ordinary 64-bit multiplications is equivalent to the implementation.
    #
    # Assumes t6 ≤ 1 and one discarded carry is 0.  These facts are both proven
    # above, so this is not smuggling in new assumptions.
    # -------------------------------------------------------------------------
    f = BitVecs("f0 f1 f2", 64)
    t6 = BitVec("t6", 64)
    w = [None]*3
    w[0], carry = add64(f[0], t6*orderComplementLimb0, ZERO)
    w[1], carry = add64(f[1], t6*orderComplementLimb1, carry)
    w[2], discarded = add64(f[2], t6, carry)
    t6_times_c = compose_mul_by_c(
        ZeroExt(64, t6) * ZeroExt(64, orderComplementLimb0),
        ZeroExt(64, t6) * ZeroExt(64, orderComplementLimb1),
        t6,
    )
    prove(
        pack_limbs(w) == pack_limbs(f) + t6_times_c,
        "R-second-tail: block != state + t6*c",
        assumptions=[ULE(t6, 1), discarded == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-third: The third reduction.
    #
    #   t0, carry = bits.Add64(t0, t4*orderComplementLimb0, 0)
    #   t1, carry = bits.Add64(t1, t4*orderComplementLimb1, carry)
    #   t2, carry = bits.Add64(t2, t4, carry)
    #   t3, carry = bits.Add64(t3, 0, carry)
    #   t4 = carry
    #
    # g0..g3 model the incoming running total in t0..t3.  Although the product
    # is added only into t0..t2, the carry chain propagates through t3.
    #
    # t4 models the incoming carry limb produced by the preceding block.
    #
    # The real implementation uses plain (truncating) 64-bit products here rather
    # than bits.Mul64.  Since t4 ≤ 3, these products are exact, so modeling them
    # as ordinary 64-bit multiplications is equivalent to the implementation.
    #
    # Assumes t4 ≤ 3.  This fact is proven above, so this is not smuggling in
    # a new assumption.
    # -------------------------------------------------------------------------
    g = BitVecs("g0 g1 g2 g3", 64)
    t4 = BitVec("t4", 64)
    w = [None]*5
    w[0], carry = add64(g[0], t4*orderComplementLimb0, ZERO)
    w[1], carry = add64(g[1], t4*orderComplementLimb1, carry)
    w[2], carry = add64(g[2], t4, carry)
    w[3], carry = add64(g[3], ZERO, carry)
    w[4] = carry
    t4_times_c = compose_mul_by_c(
        ZeroExt(64, t4) * ZeroExt(64, orderComplementLimb0),
        ZeroExt(64, t4) * ZeroExt(64, orderComplementLimb1),
        t4,
    )
    prove(
        pack_limbs(w) == pack_limbs(g) + t4_times_c,
        "R-third: 3rd reduction != state + t4*c",
        assumptions=[ULE(t4, 3)],
    )

    # -------------------------------------------------------------------------
    # Lemma R-final: The conditional-subtraction final reduction.
    #
    #   s0, borrow = bits.Sub64(t0, orderLimb0, 0)
    #   ...
    #   _, borrow = bits.Sub64(t4, 0, borrow)
    #   r[i] = constantTimeSelect64(borrow, t[i], s[i])
    #
    # z0..z4 model t0..t4 going into this stage.
    #
    # This is only valid because t < 2N as otherwise t - N could still exceed N
    # and the 4-limb result wouldn't represent it.  This fact is proven above
    # for the real total, so this does not smuggle in a new assumption.
    # -------------------------------------------------------------------------
    n = cwidth(order)
    z = BitVecs("z0 z1 z2 z3 z4", 64)
    s = [None]*4
    s[0], borrow = sub64(z[0], orderLimb0, ZERO)
    s[1], borrow = sub64(z[1], orderLimb1, borrow)
    s[2], borrow = sub64(z[2], orderLimb2, borrow)
    s[3], borrow = sub64(z[3], orderLimb3, borrow)
    _, borrow = sub64(z[4], ZERO, borrow)
    t = pack_limbs(z)
    r = If(borrow == ONE, t, pack_limbs(s))
    prove(
        r == If(ULT(t, n), t, t - n),
        "R-final: conditional-subtract result != t mod N",
        assumptions=[ULT(t, cwidth(twiceOrder))],
    )

prove_functional_congruence_lemmas()
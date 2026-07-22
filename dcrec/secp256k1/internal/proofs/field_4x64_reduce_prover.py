# Copyright (c) 2026 The Decred developers
# Copyright (c) 2026 Dave Collins
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

from z3_proof_helpers import *

# -------
# Inputs.
# -------

x = BitVecs('x0 x1 x2 x3 x4 x5 x6 x7', 64)

# NOTE: The code in this section reuses the variable names from the Go code
# being proven for easy cross referencing.  However, the comments refer to them
# by shorter mathematical variables for clarity.

# Define P.
field64Prime0 = BitVecVal(0xfffffffefffffc2f, 64)
field64Prime1 = BitVecVal(0xffffffffffffffff, 64)
field64Prime2 = BitVecVal(0xffffffffffffffff, 64)
field64Prime3 = BitVecVal(0xffffffffffffffff, 64)
field64Prime = Concat(field64Prime3, field64Prime2, field64Prime1, field64Prime0)

# Define 2P.
twicePrime0 = BitVecVal(0xfffffffdfffff85e, 64)
twicePrime1 = BitVecVal(0xffffffffffffffff, 64)
twicePrime2 = BitVecVal(0xffffffffffffffff, 64)
twicePrime3 = BitVecVal(0xffffffffffffffff, 64)
twicePrime4 = ONE
twicePrime = Concat(twicePrime4, twicePrime3, twicePrime2, twicePrime1, twicePrime0)

# Define c = 2**256 - P.
field64PrimeComplement = BitVecVal(2**32+977, 64)

# --------------------------------------------------------------------------
# Prove the constant definitions satisfy the required identities before
# trusting anything that uses them.
# --------------------------------------------------------------------------

# Prove twicePrime = 2P.
field64Prime320 = ZeroExt(64, field64Prime)
prove(twicePrime == field64Prime320 << 1, "twicePrime != 2P")

# --------------------------------------------------------------------------
# Lemma R-identity: P + c == 2**256
#
# This lemma justifies replacing factors of 2**256 modulo P by c:
#
# P + c ≡ 2**256 (mod P)
# => 0 + c ≡ 2**256 (mod P)
# => c ≡ 2**256 (mod P)
# --------------------------------------------------------------------------
field64PrimeComplement320 = ZeroExt(256, field64PrimeComplement)
prove(field64Prime320+field64PrimeComplement320 == BitVecVal(1, 320)<<256,
   "Lemma 1: P+c != 2**256")

# ---------------
# Model the code.
# ---------------

discards = []

# first reduction: 512 bits -> 289 bits.
h, t0 = mul64(x[4], field64PrimeComplement)

hi, lo = mul64(x[5], field64PrimeComplement)
t1, carry = add64(lo, h, ZERO)
h, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

hi, lo = mul64(x[6], field64PrimeComplement)
t2, carry = add64(lo, h, ZERO)
h, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

hi, lo = mul64(x[7], field64PrimeComplement)
t3, carry = add64(lo, h, ZERO)
t4, discarded = add64(hi, ZERO, carry)
discards.append(discarded)

t0, carry = add64(t0, x[0], ZERO)
t1, carry = add64(t1, x[1], carry)
t2, carry = add64(t2, x[2], carry)
t3, carry = add64(t3, x[3], carry)
t4, discarded = add64(t4, ZERO, carry)
discards.append(discarded)

# Save for analysis.
t4_saved = t4

# second reduction: 289 bits -> t < 2P
h, t4 = mul64(t4, field64PrimeComplement)

t0, carry = add64(t0, t4, ZERO)
t1, carry = add64(t1, h, carry)
t2, carry = add64(t2, ZERO, carry)
t3, carry = add64(t3, ZERO, carry)

t4 = carry

# final reduction: t < 2P -> t < P
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

# Value after second reduction is less than twice the prime (t < 2P).
t = Concat(t4, t3, t2, t1, t0)
prove(ULT(t, twicePrime), "value after 2nd reduction >= 2P")

# Fully reduced result is less than the prime (r < P).
r = Concat(r3, r2, r1, r0)
prove(ULT(r, field64Prime), "fully reduced result >= P")

# -----------------------------------------------------------------------------
# Prove functional congruence: r ≡ x (mod P).
#
# The checks above establish bounds and the safety of discarded carries,
# self-consistency of the constants, and the reduction identity (lemma
# R-identity).
#
# The following lemmas establish the arithmetic identity of each transformation
# stage.  Together with the previously proven bounds and reduction identity,
# they imply the overall congruence r ≡ x (mod P).
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
# The Go function, field64Reduce512, computes r in three stages:
#
# 1) First reduction: Fold the high 256 bits of x (x4..x7) into the low 256 bits
#    (x0..x3).  This produces t_1 = x_lo + x_hi*c which fits in 289 bits (5
#    64-bit limbs with the 5th limb small).
# 2) Second reduction: Fold t_1's 5th limb back in the same way.  This produces
#    t_2 = t_1_lo + t_1_hi*c, where t_2 < 2P.
# 3) Final reduction: One conditional subtraction of P which is valid because
#    t_2 < 2p.
#
# The lemmas below chain together to prove r ≡ t2 ≡ t1 ≡ x (mod P) and 0 ≤ r <
# P which is exactly the definition of x ≡ r (mod P).
def prove_functional_congruence_lemmas():
    """
    Prove r == x (mod P) for field64Reduce512.

    x is the 512-bit input (8 limbs x0..x7) and r is the fully-reduced 256-bit
    output (4 limbs).
    """

    # Use a wide enough congruence width that nothing in a 512-bit input ever
    # wraps around.  This is comfortably wider than the largest quantity
    # of x_hi*c < 2**289.
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

    # Universal lemma variable shared by R-open, R-interior, and R-second.
    #
    # prod models the 128-bit product return by bits.Mul64(_, c) at some point
    # in the real code.  It is NOT tied to any specific x[j] or t4.
    #
    # hi and lo are its two halves.
    prod = BitVec("prod", 128)
    hi, lo = split128(prod)

    # -------------------------------------------------------------------------
    # Lemma R-open: The opening block of the first reduction (the x4 line).
    #
    #   h, t0 = bits.Mul64(x[4], c)
    #
    # No addition happens on this line at all.  It just splits the product.
    #
    # This lemma is really just confirming that packing lo and hi back together
    # recreates the original product, which sounds trivial, but it matters
    # because it's the base case that R-interior's telescoping argument (below)
    # builds on.
    #
    # It establishes that after processing x4, the running total (h, t0) exactly
    # equals x4*c with nothing lost.
    # -------------------------------------------------------------------------
    prove(pack(hi,lo) == cwidth(prod), "R-open: (h,t0) != x4*c")

    # -------------------------------------------------------------------------
    # Lemma R-interior: The shape shared by the x5, x6, and x7 blocks.
    #
    # The x7 block simply names its final output t4 instead of h.
    #
    #   hi, lo = bits.Mul64(x[j], c)
    #   t_j, carry = bits.Add64(lo, h, 0)
    #   h, _ = bits.Add64(hi, 0, carry)
    #
    # h_in models the incoming running-total limb carried forward from the
    # previous block (R-open's h, or a previous R-interior's h_out).
    #
    # One carry is discarded and assumed zero.  This fact is proven above by the
    # discard checks for the real x5/x6/x7 blocks, so this does not smuggle in a
    # new assumption.
    # -------------------------------------------------------------------------
    h_in = BitVec("h_in", 64)
    t_j, carry = add64(lo, h_in, ZERO)
    h_out, discarded = add64(hi, ZERO, carry)
    prove(
        pack(h_out, t_j) == cwidth(h_in) + cwidth(prod),
        "R-interior: (t_j,h_out) != h_in + xj*c",
        assumptions=[discarded == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-tail: Fold x0..x3 into running total after the four blocks above.
    #
    #   t0, carry = bits.Add64(t0, x[0], 0)
    #   t1, carry = bits.Add64(t1, x[1], carry)
    #   t2, carry = bits.Add64(t2, x[2], carry)
    #   t3, carry = bits.Add64(t3, x[3], carry)
    #   t4 += carry
    #
    # b0..b4 model the incoming running total in t0..t4.
    #
    # The final line is modeled here as add64 with its own carry-out assumed
    # zero.  This fact is proven above by the discard checks for the real total,
    # so this does not smuggle in a new assumption.
    # -------------------------------------------------------------------------
    b = BitVecs("b0 b1 b2 b3 b4", 64)
    v = [None]*5
    v[0], carry = add64(b[0], x[0], ZERO)
    v[1], carry = add64(b[1], x[1], carry)
    v[2], carry = add64(b[2], x[2], carry)
    v[3], carry = add64(b[3], x[3], carry)
    v[4], discarded = add64(b[4], ZERO, carry)
    prove(
        pack_limbs(v) == pack_limbs(b) + pack_limbs(x[:4]),
        "R-tail: post-fold total != state + x_lo",
        assumptions=[discarded == ZERO],
    )

    # -------------------------------------------------------------------------
    # Lemma R-second: The second reduction.
    #
    #   h, t4 = bits.Mul64(t4, c)
    #   t0, carry = bits.Add64(t0, t4, 0)
    #   t1, carry = bits.Add64(t1, h, carry)
    #   t2, carry = bits.Add64(t2, 0, carry)
    #   t3, carry = bits.Add64(t3, 0, carry)
    #   t4 = carry
    #
    # prod is reused here for t4_old*c since it's still just a free 128-bit
    # variable and nothing ties it to being x4*c specifically.
    #
    # d0..d3 model t0..t3 going into this stage.
    #
    # No carries are discarded and the final carry becomes the new t4, so no
    # assumption is needed.
    # -------------------------------------------------------------------------
    d = BitVecs("d0 d1 d2 d3", 64)
    w = [None] * 5
    w[0], carry = add64(d[0], lo, ZERO)
    w[1], carry = add64(d[1], hi, carry)
    w[2], carry = add64(d[2], ZERO, carry)
    w[3], carry = add64(d[3], ZERO, carry)
    w[4] = carry
    prove(
        pack_limbs(w) == pack_limbs(d) + cwidth(prod),
        "R-second: 2nd reduction != state + t4*c",
    )

    # -----------------------------------------------------------------
    # Lemma R-final: The conditional-subtraction final reduction.
    #
    #   s0, borrow = bits.Sub64(t0, p0, 0)
    #   ...
    #   _, borrow = bits.Sub64(t4, 0, borrow)
    #   r[i] = constantTimeSelect64(borrow, t[i], s[i])
    #
    # z0..z4 model t0..t4 going into this stage.
    #
    # This is only valid because t < 2P as otherwise t - P could still exceed P
    # and the 4-limb result wouldn't represent it.  This fact is proven above
    # for the real total, so this does not smuggle in a new assumption.
    # -----------------------------------------------------------------
    p = cwidth(field64Prime)
    twop = cwidth(twicePrime)
    z = BitVecs("z0 z1 z2 z3 z4", 64)
    s = [None]*4
    s[0], borrow = sub64(z[0], field64Prime0, ZERO)
    s[1], borrow = sub64(z[1], field64Prime1, borrow)
    s[2], borrow = sub64(z[2], field64Prime2, borrow)
    s[3], borrow = sub64(z[3], field64Prime3, borrow)
    _, borrow = sub64(z[4], ZERO, borrow)
    t = pack_limbs(z)
    r = If(borrow == ONE, t, pack_limbs(s))
    prove(
        r == If(ULT(t, p), t, t - p),
        "R-final: conditional-subtract result != t mod P",
        assumptions=[ULT(t, twop)],
    )

prove_functional_congruence_lemmas()

# Copyright (c) 2026 The Decred developers
# Copyright (c) 2026 Dave Collins
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

from z3_proof_helpers import *

# -------
# Inputs.
# -------

a0 = BitVec('a0', 64)
a1 = BitVec('a1', 64)
a2 = BitVec('a2', 64)
a3 = BitVec('a3', 64)
b0 = BitVec('b0', 64)
b1 = BitVec('b1', 64)
b2 = BitVec('b2', 64)
b3 = BitVec('b3', 64)

# ---------------
# Model the code.
# ---------------

discards = []

# Row 0: p0..p4 = a * b0.
h0, p0 = mul64(a0, b0)
h1, p1 = mul64(a1, b0)
h2, p2 = mul64(a2, b0)
h3, p3 = mul64(a3, b0)
p1, c = add64(p1, h0, ZERO)
p2, c = add64(p2, h1, c)
p3, c = add64(p3, h2, c)
p4, discarded = add64(h3, ZERO, c)
discards.append(discarded)

p4_saved_0 = p4

# Row 1: p1..p5 += a * b1.
h0, q0 = mul64(a0, b1)
h1, q1 = mul64(a1, b1)
h2, q2 = mul64(a2, b1)
h3, q3 = mul64(a3, b1)
q1, c = add64(q1, h0, ZERO)
q2, c = add64(q2, h1, c)
q3, c = add64(q3, h2, c)
q4, discarded = add64(h3, ZERO, c)
discards.append(discarded)
p1, c = add64(p1, q0, ZERO)
p2, c = add64(p2, q1, c)
p3, c = add64(p3, q2, c)
p4, c = add64(p4, q3, c)
p5, discarded = add64(q4, ZERO, c)
discards.append(discarded)

q4_saved_1 = q4

# Row 2: p2..p6 += a * b2.
h0, q0 = mul64(a0, b2)
h1, q1 = mul64(a1, b2)
h2, q2 = mul64(a2, b2)
h3, q3 = mul64(a3, b2)
q1, c = add64(q1, h0, ZERO)
q2, c = add64(q2, h1, c)
q3, c = add64(q3, h2, c)
q4, discarded = add64(h3, ZERO, c)
discards.append(discarded)
p2, c = add64(p2, q0, ZERO)
p3, c = add64(p3, q1, c)
p4, c = add64(p4, q2, c)
p5, c = add64(p5, q3, c)
p6, discarded = add64(q4, ZERO, c)
discards.append(discarded)

q4_saved_2 = q4

# Row 3: p3..p7 += a * b3.
h0, q0 = mul64(a0, b3)
h1, q1 = mul64(a1, b3)
h2, q2 = mul64(a2, b3)
h3, q3 = mul64(a3, b3)
q1, c = add64(q1, h0, ZERO)
q2, c = add64(q2, h1, c)
q3, c = add64(q3, h2, c)
q4, discarded = add64(h3, ZERO, c)
discards.append(discarded)
p3, c = add64(p3, q0, ZERO)
p4, c = add64(p4, q1, c)
p5, c = add64(p5, q2, c)
p6, c = add64(p6, q3, c)
p7, discarded = add64(q4, ZERO, c)
discards.append(discarded)

q4_saved_3 = q4

# ------
# Proofs.
# -------

# Discarded carries are never set.
for idx, discarded in enumerate(discards):
    s = Solver()
    s.add(discarded != ZERO)
    check(s, f"discarded carry {idx} != 0")

# Top limb after row 0 is max 2**64 - 2 (p4 ≤ 2**64 - 2).
s = Solver()
s.add(UGT(p4_saved_0, (1<<64) - 2))
check(s, "top limb after row 0 > 2**64-2")

# Limb 5 after row 1 is max 2**64 - 2 (q4 ≤ 2**64 - 2).
s = Solver()
s.add(UGT(q4_saved_1, (1<<64) - 2))
check(s, "limb 5 after row 1 > 2**64-2")

# Limb 5 after row 2 is max 2**64 - 2 (q4 ≤ 2**64 - 2).
s = Solver()
s.add(UGT(q4_saved_2, (1<<64) - 2))
check(s, "limb 5 after row 2 > 2**64-2")

# Limb 5 after row 3 is max 2**64 - 2 (q4 ≤ 2**64 - 2).
s = Solver()
s.add(UGT(q4_saved_3, (1<<64) - 2))
check(s, "limb 5 after row 3 > 2**64-2")

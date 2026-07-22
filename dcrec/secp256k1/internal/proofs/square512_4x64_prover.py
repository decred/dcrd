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

# ---------------
# Model the code.
# ---------------

discards = []

# Off-diagonal upper-triangle products.
p2, p1 = mul64(a0, a1)
h02, l02 = mul64(a0, a2)
h03, l03 = mul64(a0, a3)
p2, c = add64(p2, l02, ZERO)
p3, c = add64(h02, l03, c)
p4, discarded = add64(h03, ZERO, c)
discards.append(discarded)

h12, l12 = mul64(a1, a2)
p3, c = add64(p3, l12, ZERO)
p4, c = add64(p4, h12, c)
p5 = c

h13, l13 = mul64(a1, a3)
p4, c = add64(p4, l13, ZERO)
p5, discarded = add64(p5, h13, c)
discards.append(discarded)

h23, l23 = mul64(a2, a3)
p5, c = add64(p5, l23, ZERO)
p6, discarded = add64(h23, ZERO, c)
discards.append(discarded)

# Double p1..p6, capturing the top carry into p7.
p1, c = add64(p1, p1, ZERO)
p2, c = add64(p2, p2, c)
p3, c = add64(p3, p3, c)
p4, c = add64(p4, p4, c)
p5, c = add64(p5, p5, c)
p6, c = add64(p6, p6, c)
p7 = c

# Add the diagonal squares a[i]^2 at columns 0,2,4,6 in one carry chain.
h0, p0 = mul64(a0, a0)
h1, l1 = mul64(a1, a1)
h2, l2 = mul64(a2, a2)
h3, l3 = mul64(a3, a3)
p1, c = add64(p1, h0, ZERO)
p2, c = add64(p2, l1, c)
p3, c = add64(p3, h1, c)
p4, c = add64(p4, l2, c)
p5, c = add64(p5, h2, c)
p6, c = add64(p6, l3, c)
p7, discarded = add64(p7, h3, c)
discards.append(discarded)

# ------
# Proofs.
# -------

# Discarded carries are never set.
prove_no_discarded_carries(discards)

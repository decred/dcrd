# Copyright (c) 2026 The Decred developers
# Copyright (c) 2026 Dave Collins
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.

from z3 import *

# -------
# Inputs.
# -------

ZERO = BitVecVal(0, 64)
ONE  = BitVecVal(1, 64)

# --------
# Helpers.
# --------

def mul64(x, y):
    """Return (hi, lo) exactly like bits.Mul64."""
    assert x.size() == 64
    assert y.size() == 64
    p = ZeroExt(64, x) * ZeroExt(64, y)
    lo = Extract(63, 0, p)
    hi = Extract(127, 64, p)
    return hi, lo

def add64(x, y, cin):
    """
    Return (sum, carry) exactly like bits.Add64.

    x,y : 64-bit bitvectors
    cin : 0/1 as a 64-bit bitvector
    """
    assert x.size() == 64
    assert y.size() == 64
    assert cin.size() == 64
    s = ZeroExt(1, x) + ZeroExt(1, y) + ZeroExt(1, cin)
    lo = Extract(63, 0, s)
    carry = ZeroExt(63, Extract(64, 64, s))
    return lo, carry

def sub64(x, y, bin):
    """
    Return (difference, borrow) exactly like bits.Sub64.

    x,y : 64-bit bitvectors
    bin : 0/1 as a 64-bit bitvector
    """
    assert x.size() == 64
    assert y.size() == 64
    assert bin.size() == 64
    d = ZeroExt(1, x) - ZeroExt(1, y) - ZeroExt(1, bin)
    lo = Extract(63, 0, d)
    borrow = If(ULT(ZeroExt(1, x), ZeroExt(1, y) + ZeroExt(1, bin)),
                ONE, ZERO)
    return lo, borrow

def check(s, reason):
    check_result = s.check()
    print(f"{reason}: {check_result}")

    if check_result == sat:
        m = s.model()

        for v in [x0,x1,x2,x3,x4,x5,x6,x7]:
            if m[v] != None:
                print(f"{v} = 0x{m[v].as_long():016x}")
            else:
                print(f"{v} = {m[v]}")

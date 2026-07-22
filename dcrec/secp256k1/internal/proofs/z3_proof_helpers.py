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
    cin : 0 or 1 as a 64-bit bitvector

    Note:
        Although cin is a 64-bit bitvector, it should semantically represent a
        single carry bit (0 or 1).  Z3 handles arbitrary values correctly
        (extracting only bit 0), but callers should maintain this invariant.
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
    bin : 0 or 1 as a 64-bit bitvector

    Note:
        Although bin is a 64-bit bitvector, it should semantically represent a
        single borrow bit (0 or 1).  Z3 handles arbitrary values correctly
        (extracting only bit 0), but callers should maintain this invariant.
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
    """Fail unless the proof succeeds."""
    result = s.check()
    if result != unsat:
        m = s.model() if result == sat else None
        details = ""
        if m is not None:
            details = "\n  " + ", ".join(
                f"{d.name()}=0x{m[d].as_long():016x}" for d in m.decls()
            )
        raise AssertionError(f"{reason}: {result}{details}")
    print(f"{reason}: {result}")

def prove(proposition, reason, assumptions=()):
    """
    Prove the proposition is valid by showing its negation is unsatisfiable.

    The assumptions encode additional assumptions proven elsewhere.
    """
    s = Solver()
    for a in assumptions:
        s.add(a)
    s.add(Not(proposition))
    check(s, reason)

def prove_no_discarded_carries(discards, label="discarded carry"):
    """
    Prove every carry in discards is always zero.  This is to say, discarding it
    never loses information.
    """
    for idx, discarded in enumerate(discards):
        prove(discarded == ZERO, f"{label} {idx} != 0")

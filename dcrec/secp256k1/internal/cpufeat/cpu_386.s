// Copyright (c) 2024-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Feature detection originally written by Dave Collins Feb 2019.  Modifications
// for secp256k1 made in Jul 2026.

//go:build !purego

#include "textflag.h"

// func supportsCPUID() bool
TEXT ·supportsCPUID(SB), NOSPLIT, $0-1
	// Per the Intel 64 and IA-32 Architectures Software Developer's Manual,
	// CPUID is supported if bit 21 of the EFLAGS register can be modified.
	//
	// To that end, this works as follows:
	//
	// 1. Get the current value of EFLAGS by pushing it and popping it into AX.
	// 2. Make a copy into BX for later comparison.
	// 3. Toggle bit 21 (the EFLAGS ID bit) of AX.
	// 4. Put the modified value back into EFLAGS by pushing it and popping it
	//    into EFLAGS.  The CPU will either update bit 21 of the EFLAGS with the
	//    modified value when it supports CPUID or leave it unmodified when it
	//    does not.
	// 5. Get the potentially modified value of EFLAGS by pushing it and popping
	//    it into AX.
	// 6. Put the original value back into EFLAGS to avoid any observable side
	//    effects by pushing it and popping it into EFLAGS.
	// 7. Compare the original and potentially modified EFLAGS (aka AX vs BX)
	// 8. CPUID is supported when they do not match since bit 21 was able to be
	//    modified.
	PUSHFL
	POPL AX
	MOVL AX, BX
	XORL $0x200000, AX
	PUSHL AX
	POPFL
	PUSHFL
	POPL AX
	PUSHL BX
	POPFL
	CMPL AX, BX
	JE nocpuid
	MOVB $1, ret+0(FP)
	RET
nocpuid:
	MOVB $0, ret+0(FP)
	RET

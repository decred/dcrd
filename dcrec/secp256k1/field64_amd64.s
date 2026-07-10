// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

#include "textflag.h"

#define REDUCE() \
	MOVQ  $0x1000003D1, DX        \
	XORQ  CX, CX                  \
	\
	\ // First fold: t = (p0..p3) + c*(p4..p7) as 5 limbs in R12..R15,SI.
	MULXQ R12, R12, AX            \
	MULXQ R13, R13, BX            \
	ADCXQ AX,  R13                \
	MULXQ R14, R14, AX            \
	ADCXQ BX,  R14                \
	MULXQ R15, R15, SI            \
	ADCXQ AX,  R15                \
	ADCXQ CX,  SI                 \
	\
	ADOXQ R8,  R12                \
	ADOXQ R9,  R13                \
	ADOXQ R10, R14                \
	ADOXQ R11, R15                \
	ADOXQ CX,  SI                 \
	\
	\ // Second fold: t4*c back into the low limbs, carry kept in SI.
	MULXQ SI, AX, CX              \
	ADDQ  AX, R12                 \
	ADCQ  CX, R13                 \
	ADCQ  $0, R14                 \
	ADCQ  $0, R15                 \
	MOVQ  $0, SI                  \
	ADCQ  $0, SI                  \
	\
	\ // Constant-time conditional subtract of p.
	MOVQ  $0xFFFFFFFEFFFFFC2F, AX \
	MOVQ  R12, R8                 \
	SUBQ  AX,  R8                 \
	MOVQ  R13, R9                 \
	SBBQ  $-1, R9                 \
	MOVQ  R14, R10                \
	SBBQ  $-1, R10                \
	MOVQ  R15, R11                \
	SBBQ  $-1, R11                \
	MOVQ  SI,  AX                 \
	SBBQ  $0,  AX                 \
	CMOVQCC R8,  R12              \
	CMOVQCC R9,  R13              \
	CMOVQCC R10, R14              \
	CMOVQCC R11, R15              \
	\
	MOVQ  r+0(FP), AX             \
	MOVQ  R12, 0(AX)              \
	MOVQ  R13, 8(AX)              \
	MOVQ  R14, 16(AX)             \
	MOVQ  R15, 24(AX)

// func field64MulADX(r *[4]uint64, a, b *[4]uint64)
TEXT ·field64MulADX(SB), NOSPLIT, $0-24
	MOVQ  a+8(FP), SI
	MOVQ  b+16(FP), DI
	MOVOU 0(DI), X0
	MOVOU 16(DI), X1
	XORQ  BX, BX

	// Row 0: p0..p4 = a0*b.
	MOVQ   0(SI), DX
	MOVQ   X0, AX
	PEXTRQ $1, X0, R13
	MOVQ   X1, R14
	PEXTRQ $1, X1, R15
	MULXQ  AX, R8, CX
	MULXQ  R13, R9, DI
	ADCXQ  CX, R9
	MULXQ  R14, R10, CX
	ADCXQ  DI, R10
	MULXQ  R15, R11, R12
	ADCXQ  CX, R11
	ADCXQ  BX, R12

	// Row 1: p1..p4 += a1*b; new top p5 in R13.
	MOVQ   8(SI), DX
	MOVQ   X0, R13
	PEXTRQ $1, X0, R14
	PEXTRQ $1, X1, R15
	MULXQ  R13, AX, CX
	ADOXQ  AX, R9
	MOVQ   X1, R13
	MULXQ  R14, AX, DI
	ADCXQ  CX, AX
	ADOXQ  AX, R10
	MULXQ  R13, AX, CX
	ADCXQ  DI, AX
	ADOXQ  AX, R11
	MULXQ  R15, AX, R13
	ADCXQ  CX, AX
	ADOXQ  AX, R12
	ADCXQ  BX, R13
	ADOXQ  BX, R13

	// Row 2: p2..p5 += a2*b; new top p6 in R14.
	MOVQ   16(SI), DX
	MOVQ   X0, R14
	PEXTRQ $1, X0, R15
	MULXQ  R14, AX, CX
	ADOXQ  AX, R10
	MOVQ   X1, R14
	MULXQ  R15, AX, DI
	ADCXQ  CX, AX
	ADOXQ  AX, R11
	PEXTRQ $1, X1, R15
	MULXQ  R14, AX, CX
	ADCXQ  DI, AX
	ADOXQ  AX, R12
	MULXQ  R15, AX, R14
	ADCXQ  CX, AX
	ADOXQ  AX, R13
	ADCXQ  BX, R14
	ADOXQ  BX, R14

	// Row 3: p3..p6 += a3*b; new top p7 in R15.
	MOVQ   24(SI), DX
	MOVQ   X0, AX
	PEXTRQ $1, X0, R15
	MOVQ   X1, SI
	MULXQ  AX, AX, CX
	ADOXQ  AX, R11
	MULXQ  R15, AX, DI
	ADCXQ  CX, AX
	ADOXQ  AX, R12
	PEXTRQ $1, X1, R15
	MULXQ  SI, AX, CX
	ADCXQ  DI, AX
	ADOXQ  AX, R13
	MULXQ  R15, AX, R15
	ADCXQ  CX, AX
	ADOXQ  AX, R14
	ADCXQ  BX, R15
	ADOXQ  BX, R15

	REDUCE()
	RET

// func field64SquareADX(r *[4]uint64, a *[4]uint64)
TEXT ·field64SquareADX(SB), NOSPLIT, $0-16
	MOVQ   a+8(FP), SI
	MOVOU  0(SI), X0
	MOVOU  16(SI), X1
	XORQ   R13, R13
	XORQ   R15, R15
	XORQ   CX, CX

	// Off-diagonal upper-triangle products into p1..p6 (a1->SI, a3->DI reused).
	MOVQ   X0, DX
	PEXTRQ $1, X1, DI
	MOVQ   X1, R8
	PEXTRQ $1, X0, SI
	MULXQ  SI, R9, R10
	MULXQ  R8, BX, R11
	MULXQ  DI, AX, R12
	ADCXQ  BX, R10
	ADCXQ  AX, R11
	ADCXQ  CX, R12

	MOVQ   SI, DX
	MULXQ  R8, BX, AX
	ADOXQ  BX, R11
	ADOXQ  AX, R12
	ADOXQ  CX, R13
	MULXQ  DI, BX, AX
	ADDQ   BX, R12
	ADCXQ  AX, R13

	MOVQ   R8, DX
	MULXQ  DI, BX, R14
	ADDQ   BX, R13
	ADCXQ  CX, R14

	// Double p1..p6, capturing the top carry into p7.
	ADDQ   R9, R9
	ADCXQ  R10, R10
	ADCXQ  R11, R11
	ADCXQ  R12, R12
	ADCXQ  R13, R13
	ADCXQ  R14, R14
	ADCXQ  R15, R15

	// Add the diagonal squares a[i]^2 at columns 0,2,4,6.
	MOVQ   X0, DX
	MULXQ  DX, R8, BX
	ADDQ   BX, R9
	MOVQ   SI, DX
	MULXQ  DX, BX, AX
	ADCXQ  BX, R10
	ADCXQ  AX, R11
	MOVQ   X1, DX
	MULXQ  DX, BX, AX
	ADCXQ  BX, R12
	ADCXQ  AX, R13
	MOVQ   DI, DX
	MULXQ  DX, BX, AX
	ADCXQ  BX, R14
	ADCXQ  AX, R15

	REDUCE()
	RET

// func field64CPUID(eaxIn, ecxIn uint32) (eax, ebx, ecx, edx uint32)
TEXT ·field64CPUID(SB), NOSPLIT, $0-24
	MOVL eaxIn+0(FP), AX
	MOVL ecxIn+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

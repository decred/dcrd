// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// SSE2 assembler optimizations originally written by Dave Collins May 2020.
// Converted to avo and added SSE4.1 and AVX optimizations July 2024.

//go:build ignore

package main

import (
	"github.com/mmcloughlin/avo/attr"
	"github.com/mmcloughlin/avo/build"
	"github.com/mmcloughlin/avo/gotypes"
	"github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

const (
	// blockSize is the block size of the hash algorithm in bytes.
	blockSize = 64

	// blockSizeLog2 is the base-2 log of the block size.  It is used to
	// efficiently perform integer division by the block size.
	blockSizeLog2 = 6

	// blockSizeBits is the block size in bits.
	blockSizeBits = blockSize << 3
)

// blakeConsts are the constants defined in the BLAKE specification used in
// block compression.
var blakeConsts = [16]uint32{
	0x243f6a88, 0x85a308d3, 0x13198a2e, 0x03707344,
	0xa4093822, 0x299f31d0, 0x082efa98, 0xec4e6c89,
	0x452821e6, 0x38d01377, 0xbe5466cf, 0x34e90c6c,
	0xc0ac29b7, 0xc97c50dd, 0x3f84d5b5, 0xb5470917,
}

// roundPermutationSchedule are the permutations applied to each round as
// defined in the BLAKE specification.
var roundPermutationSchedule = [10][16]uint8{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3},
	{11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4},
	{7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8},
	{9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13},
	{2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9},
	{12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11},
	{13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10},
	{6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5},
	{10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0},
}

// globals houses constants in the global data section.
var globals struct {
	// consts houses the first 8 constants defined in the BLAKE specification
	// pre-arranged so they can be copied without modification when initializing
	// the state matrix.
	consts operand.Mem

	// permutedConsts houses the constants defined in the BLAKE specification
	// pre-arranged according to the order of their use throughout the rounds.
	//
	// Having this available offers a speed advantage over directly permuting /
	// inserting the constants while performing the rounds since it means they
	// can be copied to the registers in chunks without modification.
	//
	// Since the round permutation schedule works mod 10, only the first 10
	// rounds are stored and the remaining rounds are reused.
	permutedConsts operand.Mem
}

// globalData generates a data segment to include and sets the fields in the
// 'globals' struct to the relevant memory references.
func globalData() {
	globals.consts = build.GLOBL("first_8_blake_consts", attr.RODATA|attr.NOPTR)
	for i := 0; i < 4; i++ {
		ch, cl := blakeConsts[i*2+1], blakeConsts[i*2]
		build.DATA(8*i, operand.U64(uint64(ch)<<32|uint64(cl)))
	}

	globals.permutedConsts = build.GLOBL("permuted_blake_consts", attr.RODATA|attr.NOPTR)
	sig := &roundPermutationSchedule
	for round := 0; round < 10; round++ {
		ch, cl := blakeConsts[sig[round][3]], blakeConsts[sig[round][1]]
		build.DATA(64*round+8*0, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][7]], blakeConsts[sig[round][5]]
		build.DATA(64*round+8*1, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][2]], blakeConsts[sig[round][0]]
		build.DATA(64*round+8*2, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][6]], blakeConsts[sig[round][4]]
		build.DATA(64*round+8*3, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][11]], blakeConsts[sig[round][9]]
		build.DATA(64*round+8*4, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][15]], blakeConsts[sig[round][13]]
		build.DATA(64*round+8*5, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][10]], blakeConsts[sig[round][8]]
		build.DATA(64*round+8*6, operand.U64(uint64(ch)<<32|uint64(cl)))
		ch, cl = blakeConsts[sig[round][14]], blakeConsts[sig[round][12]]
		build.DATA(64*round+8*7, operand.U64(uint64(ch)<<32|uint64(cl)))
	}
}

// blocksCompressor houses the state and specialized implementations for each
// supported ISA used to generate the BLAKE-224 and BLAKE-256 block compression
// function as well as loop to compress as many blocks as are available in the
// message.
//
// Per the BLAKE spec, compression takes four input values:
//
//   - A 256-bit chaining value h = h0 || ... || h7
//   - A 512-bit data block m = m0 || ... || mf
//   - A 128-bit salt s = s0 || s1 || s2 || s3
//   - A 64-bit counter value t = t0 || t1
//
// It also uses the 16 constants defined by the spec c = c0 || ... || cf.  See
// [blakeConsts].
//
// The output value is a new 256-bit chaining value denoted h'.
//
// Then, the compression function works in three steps:
//
//   - Initalization of an internal state
//   - Iteration of 14 rounds to transform the state
//   - Finalization to return h' from the final state value
//
// The internal state can be thought of as a 4x4 matrix and is initialized as
// follows:
//
// |v0 v1 v2 v3|   |  h0     h1     h2     h3 |
// |v4 v5 v6 v7|   |  h4     h5     h6     h7 |
// |v8 v9 va vb| = |s0^c0  s1^c1  s2^c2  s3^c3|
// |vc vd ve vf|   |t0^c4  t0^c5  t1^c6  t1^c7|
//
// Each round consists of 8 applications of a G function.  The first 4 calls
// operate on distinct columns and is called a column step.  The final 4 calls
// operate on distinct diagonals and is called a diagonal step.  Concretely:
//
// G0(v0,v4,v8,vc)  G1(v1,v5,v9,vd)  G2(v2,v6,va,ve)  G3(v3,v7,vb,vf)
// G4(v0,v5,va,vf)  G5(v1,v6,vb,vc)  G6(v2,v7,v8,vd)  G7(v3,v4,v9,ve)
//
// An important observation that because the first 4 calls (column step) operate
// on distinct columns they can be calculated in parallel.  The same is true of
// the diagonal step.
//
// The implementations take advantage of this by making use of SIMD to perform
// the calculations by treating each row of the matrix as 4x32 lanes.  The rows
// are numbered from 0 to 3.
//
// In other words:
//
// row0 = |v0 v1 v2 v3|
// row1 = |v4 v5 v6 v7|
// row2 = |v8 v9 va vb|
// row3 = |vc vd ve vf|
//
// In order to do the same thing for the diagonal step, the matrix is
// diagonalized as described in the comment for [diagonalizeFn].
//
// The G function is described in the comment for [columnStepFn].
//
// The finalization process is described in the comment for [finalizeFn].
//
// Finally, the resulting chain value is then either used in the next iteration
// when there are still enough bytes remaining to compress another full message
// block or returned to the caller via [outputChainValueFn] when there are not.
type blocksCompressor struct {
	// msgLen is the register that holds the length of the message passed to the
	// compression function.
	msgLen reg.Register

	// msgPtr is the register that holds a pointer to the message passed to the
	// compression function.
	msgPtr reg.Register

	// counter is the register that holds the total number of message bits
	// hashed so far passed to the compression function.
	counter reg.Register

	// initSMChainValueFn initializes the state matrix with the chain value
	// passed to the compression function.  That is:
	//
	// row0 = |v0 v1 v2 v3| = |h0 h1 h2 h3|
	// row1 = |v4 v5 v6 v7| = |h4 h5 h6 h7|
	initSMChainValueFn func()

	// initSMLowerHalfFn initializes the lower half of the state matrix with the
	// salt, constants, and counter.  That is:
	//
	// row2 = |v8 v9 va vb| = |s0^c0  s1^c1  s2^c2  s3^c3|
	// row3 = |vc vd ve vf| = |t0^c4  t0^c5  t1^c6  t1^c7|
	initSMLowerHalfFn func()

	// convertMsgFn converts the current 512-bit message (data block) from
	// little to big endian as required by the spec.
	convertMsgFn func()

	// columnStepFn performs the column/diagonal step for the given round.
	//
	// Per the BLAKE specification, the G (compression) function takes 4 params
	// (a, b, c, d) where each of those are entries from the state matrix, and
	// looks up message words (mx and my here) and constants (cx and cy) per the
	// round permutation schedule.
	//
	// Setting the inputs to rows that consist of packed 4x32 uint32s in the
	// state matrix (a=row0, b=row1, c=row2, d=row3) instead so they can be
	// calculated in parallel and plugging that into the definition in the spec
	// yields:
	//
	//  row0 = row0 + row1 + (mx^cx)
	//  row3 = (row3^row0) >>> 16
	//  row2 = row2 + row3
	//  row1 = (row1^row2) >>> 12
	//  row0 = row0 + row1 + (my^cy)
	//  row3 = (row3^row0) >>> 8
	//  row2 = row2 + row3
	//  row1 = (row1^row2) >>> 7
	columnStepFn func(round uint8, isDiagStep bool)

	// diagonalizeFn shifts each of the rows in order to prepare the state
	// matrix for performing the diagonal step via a column step.
	//
	// As described by the comments on [blocksCompressor], the state matrix is:
	//
	// row0 = |v0 v1 v2 v3|
	// row1 = |v4 v5 v6 v7|
	// row2 = |v8 v9 va vb|
	// row3 = |vc vd ve vf|
	//
	// The diagonal step consists of applying the G function to the diagonals:
	// G4(v0,v5,va,vf)  G5(v1,v6,vb,vc)  G6(v2,v7,v8,vd)  G7(v3,v4,v9,ve)
	//
	// Therefore, the diagonal step can be accomplished by transforming the
	// state such that the diagonals are moved into colums by rotating the i-th
	// row (where i=0..3) to the left by i positions, performing a column step,
	// and then undoing the transformation by rotating the i-th row to the right
	// i positions.
	//
	// Namely, performing the described transformation:
	//
	// |v0  v1  v2  v3|
	// |v5  v6  v7  v4| (rotated left once)
	// |va  vb  v8  v9| (rotated left twice)
	// |vf  vc  vd  ve| (rotated left three times)
	//
	// Note that the columns are precisely the inputs necessary for the final 4
	// calls to the compression function (G4..G7).
	diagonalizeFn func(rows []reg.VecVirtual)

	// undiagonalizeFn undoes the transformation described by [diagonalizeFn].
	undiagonalizeFn func(rows []reg.VecVirtual)

	// finalizeFn finalizes the state matrix to produce the resulting chain
	// value as follows:
	//
	// h'0 = h0^s0^v0^v8
	// h'1 = h1^s1^v1^v9
	// h'2 = h2^s2^v2^va
	// h'3 = h3^s3^v3^vb
	// h'4 = h4^s0^v4^vc
	// h'5 = h5^s1^v5^vd
	// h'6 = h6^s2^v6^ve
	// h'7 = h7^s3^v7^vf
	finalizeFn func()

	// outputChainValueFn outputs the final chain value to the caller.
	outputChainValueFn func()
}

// generate generates a block compression function along with a loop to compress
// as many full blocks as are available using the various state and state
// transformation functions specified in the [blocksCompressor] instance.
func (bcs *blocksCompressor) generate(rows []reg.VecVirtual) {
	build.Comment("Convert message len to number of blocks for loop counter.")
	build.SHRQ(operand.U8(blockSizeLog2), bcs.msgLen)

	// Note that this is done outside of the loop because the registers that
	// hold the chain value are updated to the modified chain value as part of
	// the process and thus are already in place for subsequent iterations of
	// the loop.
	build.Comment("Initialize state matrix.")
	build.Comment("row0 = |v0  v1  v2  v3|   |  h0     h1     h2     h3 |")
	build.Comment("row1 = |v4  v5  v6  v7|   |  h4     h5     h6     h7 |")
	bcs.initSMChainValueFn()

	build.Label("compressLoop")
	build.Comment("row2 = |v8  v9  va  vb| = |s0^c0  s1^c1  s2^c2  s3^c3|")
	build.Comment("row3 = |vc  vd  ve  vf|   |t0^c4  t0^c5  t1^c6  t1^c7|")
	bcs.initSMLowerHalfFn()

	build.Comment("Convert message to big endian.")
	bcs.convertMsgFn()

	// Perform the 14 rounds.
	for round := uint8(0); round < 14; round++ {
		build.Commentf("Round %d column step.", round+1)
		bcs.columnStepFn(round, false) // G0 G1 G2 G3
		build.Commentf("Round %d diagonal step part 1: diagonalize.", round+1)
		bcs.diagonalizeFn(rows)
		build.Commentf("Round %d diagonal step part 2: column step.", round+1)
		bcs.columnStepFn(round, true) // G4 G5 G6 G7
		build.Commentf("Round %d diagonal step part 3: undiagonalize.", round+1)
		bcs.undiagonalizeFn(rows)
	}

	build.Comment("Finally the chain value is defined as:")
	build.Comment("h'0 = h0^s0^v0^v8")
	build.Comment("h'1 = h1^s1^v1^v9")
	build.Comment("h'2 = h2^s2^v2^va")
	build.Comment("h'3 = h3^s3^v3^vb")
	build.Comment("h'4 = h4^s0^v4^vc")
	build.Comment("h'5 = h5^s1^v5^vd")
	build.Comment("h'6 = h6^s2^v6^ve")
	build.Comment("h'7 = h7^s3^v7^vf")
	bcs.finalizeFn()

	build.Comment("Either terminate the loop when there are no more full blocks")
	build.Comment("to compress or move the message pointer to the next block of")
	build.Comment("bytes to compress, increment the message bits counter")
	build.Comment("accordingly, and loop back around to compress it.")
	build.DECQ(bcs.msgLen)
	build.JZ(operand.LabelRef("done"))
	build.LEAQ(memOffset(bcs.msgPtr, blockSize), bcs.msgPtr)
	build.ADDQ(operand.U32(blockSizeBits), bcs.counter)
	build.JMP(operand.LabelRef("compressLoop"))

	build.Label("done")
	build.Comment("Output the resulting chain value.")
	bcs.outputChainValueFn()
}

// determineMsgIdxs returns the needed message word indices per the round
// permutation schedule for the given parameters.
func determineMsgIdxs(round uint8, isDiagStep, isFirstLoad bool) [4]uint8 {
	// Assert max 14 rounds (zero based).
	if round > 13 {
		panic("round must be a max of 13")
	}

	// Determine the needed message word indices per the round permutation
	// schedule.
	gBase, offset := 0, 0
	if isDiagStep {
		gBase = 4
	}
	if !isFirstLoad {
		offset = 1
	}
	return [4]uint8{
		roundPermutationSchedule[round%10][2*(gBase+0)+offset],
		roundPermutationSchedule[round%10][2*(gBase+1)+offset],
		roundPermutationSchedule[round%10][2*(gBase+2)+offset],
		roundPermutationSchedule[round%10][2*(gBase+3)+offset],
	}
}

// rotateRight4x32SSE2 performs a 4-way 32-bit right rotation of the provided
// vector register by the given number of bits using opcodes limited to SSE2.
// SSE2 does not have an instruction to do it directly, so it is implemented
// using 4-way 32-bit SIMD instructions to xor a right shift of 'bits' with a
// left shift of 32 - 'bits'.  The provided tmp register is used for the
// intermediate results.
func rotateRight4x32SSE2(tmp reg.Register, bits uint8, dest reg.Register) {
	build.MOVO(dest, tmp)
	build.PSRLL(operand.U8(bits), tmp)
	build.PSLLL(operand.U8(32-bits), dest)
	build.PXOR(tmp, dest)
}

// shuffle4x32SSE2 performs a 4-way 32-bit shuffle of the given registers using
// the SSE2 PSHUFD opcode.
//
// The arguments specify which zero-based source position (lane) to copy to
// the position (lane) of the argument itself in the destination.  The
// arguments treat the 32-bit lanes as if they were numbered from left to
// right.
//
// For example, consider 4 uint32s:
//
//	x0 = 0xaaaaaaaa, x1 = 0xbbbbbbbb, x2 = 0xcccccccc, x3 = 0xdddddddd
//
// With the packed register laid out as:
//
//	x0 x1 x2 x3 = 0xaaaaaaaa 0xbbbbbbbb 0xcccccccc 0xdddddddd
//
// Then, a source selector of 0 selects x0 = 0xaaaaaaaa, 1 selects
// x1 = 0xbbbbbbbb, etc.
//
// A shuffle of (z,y,x,w) = (2,3,0,1) = lane 2->0, 3->1, 0->2, and 1->3
// would result in:
//
//	  lane:  0  1  2  3
//	source: x0 x1 x2 x3 = 0xaaaaaaaa 0xbbbbbbbb 0xcccccccc 0xdddddddd
//	  dest: x2 x3 x0 x1 = 0xcccccccc 0xdddddddd 0xaaaaaaaa 0xbbbbbbbb
func shuffle4x32SSE2(z, y, x, w uint8, src, dest reg.Register) {
	if z > 3 || y > 3 || x > 3 || w > 3 {
		panic("src position parameters must not exceed 3 for a 4-way " +
			"32-bit shuffle")
	}
	// PSHUFD expects the lanes to be numbered from right to left, so
	// reverse the params and encode accordingly.
	order := uint8(w<<6 | x<<4 | y<<2 | z)
	build.PSHUFD(operand.U8(order), src, dest)
}

// set4x32SSE2 packs the 4 provided 32-bit unsigned integers into the
// destination vector register using opcodes limited to SSE2.
//
// It makes use of the two provided temporary registers in the process.
func set4x32SSE2(tmp0, tmp1 reg.Register, z, y, x, w operand.Op, dest reg.Register) {
	// NOTE: These MOVD really should be MOVL instructions for the Go assembler
	// so it only moves 32 bits instead of 64 bits, but a bug in avo won't allow
	// MOVL to be used with XMM registers.  The logic still works with MOVD here
	// though because the unpacks select the low 32 bits.
	//
	// See https://github.com/mmcloughlin/avo/issues/436.
	build.MOVD(w, dest)
	build.MOVD(x, tmp0)
	build.MOVOA(tmp0, tmp1)
	build.PUNPCKLLQ(dest, tmp1)
	build.MOVD(y, tmp0)
	build.MOVD(z, dest)
	build.PUNPCKLLQ(tmp0, dest)
	build.PUNPCKLQDQ(tmp1, dest)
}

// diagonalizeSSE2 shifts each of the rows in order to prepare the state matrix
// for performing the diagonal step via a column step using opcodes limited to
// SSE2.
//
// See [blocksCompressor.diagonalizeFn] for more details.
func diagonalizeSSE2(rows []reg.VecVirtual) {
	shuffle4x32SSE2(1, 2, 3, 0, rows[1], rows[1])
	shuffle4x32SSE2(2, 3, 0, 1, rows[2], rows[2])
	shuffle4x32SSE2(3, 0, 1, 2, rows[3], rows[3])
}

// undiagonalizeSSE2 undoes the transformation discussed by [diagonalizeSSE2]
// using opcodes limited to SSE2.
func undiagonalizeSSE2(rows []reg.VecVirtual) {
	shuffle4x32SSE2(3, 0, 1, 2, rows[1], rows[1])
	shuffle4x32SSE2(2, 3, 0, 1, rows[2], rows[2])
	shuffle4x32SSE2(1, 2, 3, 0, rows[3], rows[3])
}

// mustAddr is a convenience method to resolve an avo component to a memory
// reference to said component.
func mustAddr(c gotypes.Component) operand.Mem {
	b, err := c.Resolve()
	if err != nil {
		panic(err)
	}
	return b.Addr
}

// memOffset returns a memory reference that is the provided offset number of
// bytes past the start of the location the given register holds.
func memOffset(r reg.Register, offset int) operand.Mem {
	return operand.Mem{Base: r, Disp: offset}
}

// blocksSSE2 generates the BLAKE-224 and BLAKE-256 block compression function
// accelerated by SSE2.
func blocksSSE2() {
	build.TEXT("blocksSSE2", 0, "func(state *State, msg []byte, counter uint64)")
	build.Doc("blocksSSE2 performs BLAKE-224 and BLAKE-256 block compression",
		"using SSE2 extensions.  See [Blocks] in blocksisa_amd64.go for",
		"parameter details.")
	build.Pragma("noescape")

	// Load / dereference function params.
	state := build.Dereference(build.Param("state"))
	h := state.Field("CV")
	s := state.Field("S")
	counter := build.Load(build.Param("counter"), build.GP64())
	msgPtr := build.Load(build.Param("msg").Base(), build.GP64())
	msgLen := build.Load(build.Param("msg").Len(), build.GP64())

	// Allocate stack space for the converted message.
	m := build.AllocLocal(blockSize)

	// XMM registers used for operation.
	//
	//   sr: Holds the salt passed to the function.
	// rows: Holds the state matrix.
	//   hr: Holds the chain value.
	//  tmp: Holds temp results when setting XMM registers.
	//  imr: Holds temp intermediate results.
	sr := build.XMM()
	rows := []reg.VecVirtual{build.XMM(), build.XMM(), build.XMM(), build.XMM()}
	hr := []reg.VecVirtual{build.XMM(), build.XMM()}
	tmp := []reg.VecVirtual{build.XMM(), build.XMM()}
	imr := []reg.VecVirtual{build.XMM(), build.XMM()}

	// General purpose registers used for swapping the message to big endian.
	var swap [8]reg.Register
	for i := 0; i < len(swap); i++ {
		swap[i] = build.GP32()
	}

	// loadMsg4x32 loads the permuted message words for the given parameters
	// into the provided register as 4 packed uint32s.  This loads the relevant
	// message words from the stack since the message is stored there.
	loadMsg4x32 := func(round uint8, isDiagStep, isFirstLoad bool, dest reg.Register) {
		neededMsgIdxs := determineMsgIdxs(round, isDiagStep, isFirstLoad)
		mz := m.Offset(4 * int(neededMsgIdxs[0]))
		my := m.Offset(4 * int(neededMsgIdxs[1]))
		mx := m.Offset(4 * int(neededMsgIdxs[2]))
		mw := m.Offset(4 * int(neededMsgIdxs[3]))
		set4x32SSE2(tmp[0], tmp[1], mz, my, mx, mw, dest)
	}

	// Instantiate an instance of the blocks compression generator with the
	// various state transformation functions implemented such that they only
	// make use of the capabilities (e.g. opcodes and registers) available to
	// SSE2 and perform the generation.
	//
	// See the comments of [blocksCompressor] for an overview of the overall
	// methodology and what each function does.
	compressor := blocksCompressor{
		msgLen:  msgLen,
		msgPtr:  msgPtr,
		counter: counter,
		initSMChainValueFn: func() {
			build.MOVOU(mustAddr(s.Index(0)), sr)      //   sr  = s0 s1 s2 s3
			build.MOVOU(mustAddr(h.Index(0)), rows[0]) // row0  = h0 h1 h2 h3
			build.MOVOU(mustAddr(h.Index(4)), rows[1]) // row1  = h4 h5 h6 h7
		},
		initSMLowerHalfFn: func() {
			build.MOVOU(globals.consts.Offset(0), rows[2]) // row2  = c0 c1 c2 c3
			build.PXOR(sr, rows[2])                        // row2 ^= s0 s1 s2 s3
			build.MOVD(counter, rows[3])                   // row3  = t0 t1 0  0
			shuffle4x32SSE2(0, 0, 1, 1, rows[3], rows[3])  // row3  = t0 t0 t1 t1
			build.PXOR(globals.consts.Offset(16), rows[3]) // row3 ^= c4 c5 c6 c7
			build.MOVO(rows[0], hr[0])                     //  hr0  = h0 h1 h2 h3
			build.MOVO(rows[1], hr[1])                     //  hr1  = h4 h5 h6 h7
		},
		convertMsgFn: func() {
			// Convert the message to big endian.  The message is stored on the
			// stack.
			for i := 0; i < 2; i++ {
				// Use two loops to prevent avo from reusing the same register
				// which in turn ensures the processor avoids false
				// dependencies.
				for j := 0; j < 8; j++ {
					build.MOVL(memOffset(msgPtr, i*32+j*4), swap[j])
				}
				for j := 0; j < 8; j++ {
					build.BSWAPL(swap[j])
					build.MOVL(swap[j], m.Offset(4*(i*8+j)))
				}
			}
		},
		columnStepFn: func(round uint8, isDiagStep bool) {
			// Determine the memory offsets of the permuted constants for this
			// round and column/diagonal step.
			baseConstsOffset := int(round%10) * 64
			if isDiagStep {
				baseConstsOffset += 32
			}
			cxOffset := globals.permutedConsts.Offset(baseConstsOffset)
			cyOffset := globals.permutedConsts.Offset(baseConstsOffset + 16)

			loadMsg4x32(round, isDiagStep, true, imr[0])  // imr0 = mx (for G[i]..G[i+3])
			build.MOVOU(cxOffset, imr[1])                 // imr1 = cx (for G[i]..G[i+3])
			build.PXOR(imr[0], imr[1])                    // imr1 = mx^cx
			build.PADDD(imr[1], rows[0])                  // row0 += imr1 (mx^cx)
			loadMsg4x32(round, isDiagStep, false, imr[0]) // imr0 = my (for G[i]..G[i+3])
			build.MOVOU(cyOffset, imr[1])                 // imr1 = cy (for G[i]..G[i+3])
			build.PXOR(imr[0], imr[1])                    // imr1 = my^cy
			build.PADDD(rows[1], rows[0])                 // row0 += row1
			build.PXOR(rows[0], rows[3])                  // row3 ^= row0
			rotateRight4x32SSE2(tmp[0], 16, rows[3])      // row3 >>>= 16
			build.PADDD(rows[3], rows[2])                 // row2 += row3
			build.PXOR(rows[2], rows[1])                  // row1 ^= row2
			rotateRight4x32SSE2(tmp[0], 12, rows[1])      // row1 >>>= 12
			build.PADDD(imr[1], rows[0])                  // row0 += imr1 (my^cy)
			build.PADDD(rows[1], rows[0])                 // row0 += row1
			build.PXOR(rows[0], rows[3])                  // row3 ^= row0
			rotateRight4x32SSE2(tmp[0], 8, rows[3])       // row3 >>>= 8
			build.PADDD(rows[3], rows[2])                 // row2 += row3
			build.PXOR(rows[2], rows[1])                  // row1 ^= row2
			rotateRight4x32SSE2(tmp[0], 7, rows[1])       // row1 >>>= 7
		},
		diagonalizeFn:   diagonalizeSSE2,
		undiagonalizeFn: undiagonalizeSSE2,
		finalizeFn: func() {
			build.PXOR(hr[0], rows[0])   // row0 ^= h0 h1 h2 h3
			build.PXOR(sr, rows[0])      // row0 ^= s0 s1 s2 s3
			build.PXOR(rows[2], rows[0]) // row0 ^= v8 v9 va vb
			build.PXOR(hr[1], rows[1])   // row1 ^= h4 h5 h6 h7
			build.PXOR(sr, rows[1])      // row1 ^= s0 s1 s2 s3
			build.PXOR(rows[3], rows[1]) // row1 ^= vc vd ve vf
		},
		outputChainValueFn: func() {
			build.MOVOU(rows[0], mustAddr(h.Index(0))) // h0 h1 h2 h3 = row0
			build.MOVOU(rows[1], mustAddr(h.Index(4))) // h4 h5 h6 h7 = row1
		},
	}
	compressor.generate(rows)
	build.RET()
}

func main() {
	// Ideally this would just reference the compress package with the struct
	// definition, but avo doesn't seem to have a way to specify a build tag
	// for this statement and the compress package is unable to build before the
	// code is generated without a tag.  So, just duplicate the struct
	// definition in this package and reference it.
	build.Package("github.com/decred/dcrd/crypto/blake256/internal/_asm")

	build.ConstraintExpr("!purego")
	globalData()
	blocksSSE2()
	build.Generate()
}

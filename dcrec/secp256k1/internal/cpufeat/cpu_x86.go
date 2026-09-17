// Copyright (c) 2024-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Feature detection originally written by Dave Collins Feb 2019 for blake256.
// Additional cleanup and comments added Jul 2024.  Modifications for secp256k1
// made in Jul 2026.

//go:build (386 || amd64) && !purego

package cpufeat

// supportsCPUID returns true when the CPU supports the CPUID opcode.
//
//go:noescape
func supportsCPUID() bool

// cpuid provides access to the CPUID opcode.
//
//go:noescape
func cpuid(eaxIn, ecxIn uint32) (eax, ebx, ecx, edx uint32)

// hasBit returns whether or not the provided bit is set in the given test
// value.
func hasBit(testVal uint32, bit uint) bool {
	return testVal>>bit&1 == 1
}

// detect returns the result of querying the CPU to determine supported
// features.
func detect() Features {
	// Per CPUID—CPU Identification in Chapter 3 of the Intel 64 and IA-32
	// Architectures Software Developer's Manual, Volume 2A:
	//
	// "The ID flag (bit 21) in the EFLAGS register indicates support for the
	// CPUID instruction. If a software procedure can set and clear this flag,
	// the processor executing the procedure supports the CPUID instruction.
	// This instruction operates the same in non-64-bit modes and 64-bit mode.
	//
	// CPUID returns processor identification and feature information in the
	// EAX, EBX, ECX, and EDX registers.  The output is dependent on the
	// contents of the EAX register upon execution (in some cases, ECX as
	// well)."
	//
	// The inputs and outputs for determining various levels of support that are
	// relevant to secp256k1 are:
	//
	// Initial EAX Value | Output
	// ------------------|------------------------------------------------
	// 0x00              | EAX = Maximum Input Value for Basic CPUID Info.
	// -------------------------------------------------------------------
	// 0x07              | EBX = Feature Information
	//                   |  Bit 8 = Bit Manipulation Instruction Set 2 (BMI2)
	//                   |  Bit 19 = Multi-Precision Add-Carry Extensions (ADX)
	const (
		eaxInputQueryMax          = 0x00
		eaxInputQueryExtFeatFlags = 0x07

		ebx7OutputBMI2Bit = 8
		ebx7OutputADXBit  = 19
	)

	// Nothing to do if the CPU somehow does not support CPUID.  Go probably
	// won't even run on such a CPU, but as the Intel manual states, it is
	// technically required to check if CPUID is supported before querying it
	// and it's best to be safe.
	var f Features
	if !supportsCPUID() {
		return f
	}

	// Querying the supported feature info for BMI2 and ADX is only valid if the
	// CPU at least supports querying the Structured Extended Feature
	// Enumeration sub-leaf.
	maxEAXInput, _, _, _ := cpuid(eaxInputQueryMax, 0)
	if maxEAXInput < eaxInputQueryExtFeatFlags {
		return f
	}

	// Query extended feature info to determine BMI2 and ADX support.
	_, ebx, _, _ := cpuid(eaxInputQueryExtFeatFlags, 0)
	f.BMI2 = hasBit(ebx, ebx7OutputBMI2Bit)
	f.ADX = hasBit(ebx, ebx7OutputADXBit)

	return f
}

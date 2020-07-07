// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestBadPC sets the pc to a deliberately bad result then confirms that Step
// and Disasm fail correctly.
func TestBadPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scriptIdx int
	}{
		{scriptIdx: 2},
		{scriptIdx: 3},
	}
	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash: chainhash.Hash([32]byte{
					0xc9, 0x97, 0xa5, 0xe5,
					0x6e, 0x10, 0x41, 0x02,
					0xfa, 0x20, 0x9c, 0x6a,
					0x85, 0x2d, 0xd9, 0x06,
					0x60, 0xa2, 0x0b, 0x2d,
					0x9c, 0x35, 0x24, 0x23,
					0xed, 0xce, 0x25, 0x85,
					0x7f, 0xcd, 0x37, 0x04,
				}),
				Index: 0,
			},
			SignatureScript: mustParseShortForm("NOP"),
			Sequence:        4294967295,
		}},
		TxOut: []*wire.TxOut{{
			Value:    1000000000,
			PkScript: nil,
		}},
		LockTime: 0,
	}
	pkScript := mustParseShortForm("NOP")

	for _, test := range tests {
		vm, err := NewEngine(pkScript, tx, 0, 0, 0, nil)
		if err != nil {
			t.Errorf("Failed to create script: %v", err)
		}

		// Set to after all scripts.
		vm.scriptIdx = test.scriptIdx

		// Ensure attempting to step fails.
		_, err = vm.Step()
		if err == nil {
			t.Errorf("Step with invalid pc (%v) succeeds!", test)
			continue
		}

		// Ensure attempting to disassemble the current program counter fails.
		_, err = vm.DisasmPC()
		if err == nil {
			t.Errorf("DisasmPC with invalid pc (%v) succeeds!", test)
		}
	}
}

// TestCheckErrorCondition tests the execute early test in CheckErrorCondition()
// since most code paths are tested elsewhere.
func TestCheckErrorCondition(t *testing.T) {
	t.Parallel()

	// tx with almost empty scripts.
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash: chainhash.Hash([32]byte{
						0xc9, 0x97, 0xa5, 0xe5,
						0x6e, 0x10, 0x41, 0x02,
						0xfa, 0x20, 0x9c, 0x6a,
						0x85, 0x2d, 0xd9, 0x06,
						0x60, 0xa2, 0x0b, 0x2d,
						0x9c, 0x35, 0x24, 0x23,
						0xed, 0xce, 0x25, 0x85,
						0x7f, 0xcd, 0x37, 0x04,
					}),
					Index: 0,
				},
				SignatureScript: []uint8{},
				Sequence:        4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value:    1000000000,
				PkScript: nil,
			},
		},
		LockTime: 0,
	}
	pkScript := mustParseShortForm("NOP NOP NOP NOP NOP NOP NOP NOP NOP" +
		" NOP TRUE")

	vm, err := NewEngine(pkScript, tx, 0, 0, 0, nil)
	if err != nil {
		t.Errorf("failed to create script: %v", err)
	}

	for i := 0; i < len(pkScript)-1; i++ {
		done, err := vm.Step()
		if err != nil {
			t.Fatalf("failed to step %dth time: %v", i, err)
		}
		if done {
			t.Fatalf("finshed early on %dth time", i)
		}

		err = vm.CheckErrorCondition(false)
		if !errors.Is(err, ErrScriptUnfinished) {
			t.Fatalf("got unexpected error %v on %dth iteration",
				err, i)
		}
	}
	done, err := vm.Step()
	if err != nil {
		t.Fatalf("final step failed %v", err)
	}
	if !done {
		t.Fatalf("final step isn't done!")
	}

	err = vm.CheckErrorCondition(false)
	if err != nil {
		t.Errorf("unexpected error %v on final check", err)
	}
}

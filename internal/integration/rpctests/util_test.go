// Copyright (c) 2022 The Decred developers

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctests

import "github.com/decred/dcrd/wire"

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

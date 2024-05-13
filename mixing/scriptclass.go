// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

// ScriptClass describes the type and format of scripts that can be used for
// mixed outputs.  A mix may only be performed among all participants who agree
// on the same script class.
type ScriptClass string

// Script class descriptors for the mixed outputs.
// Only secp256k1 P2PKH is allowed at this time.
const (
	ScriptClassP2PKHv0 ScriptClass = "P2PKH-secp256k1-v0"
)

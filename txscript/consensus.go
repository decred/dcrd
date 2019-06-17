// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"fmt"
)

const (
	// LockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.
	LockTimeThreshold = 5e8 // Tue Nov 5 00:53:20 1985 UTC

	// maxUniqueCoinbaseNullDataSize is the maximum number of bytes allowed
	// in the pushed data output of the coinbase output that is used to
	// ensure the coinbase has a unique hash.
	//
	// Deprecated: This will be removed in the next major version bump.
	maxUniqueCoinbaseNullDataSize = 256
)

// ExtractCoinbaseNullData ensures the passed script is a nulldata script as
// required by the consensus rules for the coinbase output that is used to
// ensure the coinbase has a unique hash and returns the data it pushes.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
//
// Deprecated: This will be removed in the next major version bump.
func ExtractCoinbaseNullData(pkScript []byte) ([]byte, error) {
	// The nulldata in the coinbase must be a single OP_RETURN followed by a
	// data push up to maxUniqueCoinbaseNullDataSize bytes.
	//
	// NOTE: This is intentionally not using GetScriptClass and the related
	// functions because those are specifically for standardness checks which
	// can change over time and this function is specifically intended to be
	// used by the consensus rules.
	//
	// Also of note is that technically normal nulldata scripts support encoding
	// numbers via small opcodes, however the consensus rules require the block
	// height to be encoded as a 4-byte little-endian uint32 pushed via a normal
	// data push, as opposed to using the normal number handling semantics of
	// scripts, so this is specialized to accommodate that.
	const scriptVersion = 0
	if len(pkScript) == 1 && pkScript[0] == OP_RETURN {
		return nil, nil
	}
	if len(pkScript) > 1 && pkScript[0] == OP_RETURN {
		tokenizer := MakeScriptTokenizer(scriptVersion, pkScript[1:])
		if tokenizer.Next() && tokenizer.Done() &&
			tokenizer.Opcode() <= OP_PUSHDATA4 &&
			len(tokenizer.Data()) <= maxUniqueCoinbaseNullDataSize {

			return tokenizer.Data(), nil
		}
	}

	str := fmt.Sprintf("script %x is not well-formed coinbase nulldata",
		pkScript)
	return nil, scriptError(ErrMalformedCoinbaseNullData, str)
}

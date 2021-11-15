// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package stdscript provides facilities for working with standard scripts.
package stdscript

import "github.com/decred/dcrd/txscript/v4/stdaddr"

// ExtractAddrs analyzes the passed public key script and returns the associated
// script type along with any addresses associated with it when possible.
//
// This function only works for standard script types and any data that fails to
// produce a valid address is omitted from the results.  This means callers must
// not blindly assume the slice will be of a particular length for a given
// returned script type and should always check the length prior to access in
// case the addresses were not able to be created.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return a nonstandard script type and no addresses for other script
// versions.
func ExtractAddrs(scriptVersion uint16, pkScript []byte, params stdaddr.AddressParamsV0) (ScriptType, []stdaddr.Address) {
	switch scriptVersion {
	case 0:
		return ExtractAddrsV0(pkScript, params)
	}

	return STNonStandard, nil
}

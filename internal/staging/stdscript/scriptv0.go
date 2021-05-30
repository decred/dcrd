// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

// DetermineScriptTypeV0 returns the type of the passed version 0 script from
// the known standard types.  This includes both types that are required by
// consensus as well as those which are not.
//
// STNonStandard will be returned when the script does not parse.
func DetermineScriptTypeV0(script []byte) ScriptType {
	return STNonStandard
}

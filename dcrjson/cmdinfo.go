// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import v3 "github.com/decred/dcrd/dcrjson/v3"

// CmdMethod returns the method for the passed command.  The provided command
// type must be a registered type.  All commands provided by this package are
// registered by default.
func CmdMethod(cmd interface{}) (string, error) {
	return v3.CmdMethod(cmd)
}

// MethodUsageFlags returns the usage flags for the passed command method.  The
// provided method must be associated with a registered type.  All commands
// provided by this package are registered by default.
func MethodUsageFlags(method string) (UsageFlag, error) {
	return v3.MethodUsageFlags(method)
}

// MethodUsageText returns a one-line usage string for the provided method.  The
// provided method must be associated with a registered type.  All commands
// provided by this package are registered by default.
func MethodUsageText(method string) (string, error) {
	return v3.MethodUsageText(method)
}

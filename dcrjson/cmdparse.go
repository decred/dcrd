// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import v3 "github.com/decred/dcrd/dcrjson/v3"

// MarshalCmd marshals the passed command to a JSON-RPC request byte slice that
// is suitable for transmission to an RPC server.  The provided command type
// must be a registered type.  All commands provided by this package are
// registered by default.
func MarshalCmd(rpcVersion string, id interface{}, cmd interface{}) ([]byte, error) {
	return v3.MarshalCmd(rpcVersion, id, cmd)
}

// UnmarshalCmd unmarshals a JSON-RPC request into a suitable concrete command
// so long as the method type contained within the marshalled request is
// registered.
func UnmarshalCmd(r *Request) (interface{}, error) {
	return v3.ParseParams(r.Method, r.Params)
}

// NewCmd provides a generic mechanism to create a new command that can marshal
// to a JSON-RPC request while respecting the requirements of the provided
// method.  The method must have been registered with the package already along
// with its type definition.  All methods associated with the commands exported
// by this package are already registered by default.
//
// The arguments are most efficient when they are the exact same type as the
// underlying field in the command struct associated with the the method,
// however this function also will perform a variety of conversions to make it
// more flexible.  This allows, for example, command line args which are strings
// to be passed unaltered.  In particular, the following conversions are
// supported:
//
//   - Conversion between any size signed or unsigned integer so long as the
//     value does not overflow the destination type
//   - Conversion between float32 and float64 so long as the value does not
//     overflow the destination type
//   - Conversion from string to boolean for everything strconv.ParseBool
//     recognizes
//   - Conversion from string to any size integer for everything
//     strconv.ParseInt and strconv.ParseUint recognizes
//   - Conversion from string to any size float for everything
//     strconv.ParseFloat recognizes
//   - Conversion from string to arrays, slices, structs, and maps by treating
//     the string as marshalled JSON and calling json.Unmarshal into the
//     destination field
func NewCmd(method string, args ...interface{}) (interface{}, error) {
	return v3.NewCmd(method, args...)
}

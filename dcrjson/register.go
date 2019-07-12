// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import v3 "github.com/decred/dcrd/dcrjson/v3"

// UsageFlag define flags that specify additional properties about the
// circumstances under which a command can be used.
type UsageFlag = v3.UsageFlag

const (
	// UFWalletOnly indicates that the command can only be used with an RPC
	// server that supports wallet commands.
	//
	// Deprecated: This flag is not used in the v3 API and is masked away
	// when presenting formatted UsageFlag strings.
	UFWalletOnly UsageFlag = 1 << iota

	// UFWebsocketOnly indicates that the command can only be used when
	// communicating with an RPC server over websockets.  This typically
	// applies to notifications and notification registration functions
	// since neiher makes since when using a single-shot HTTP-POST request.
	UFWebsocketOnly

	// UFNotification indicates that the command is actually a notification.
	// This means when it is marshalled, the ID must be nil.
	UFNotification

	// highestUsageFlagBit is the maximum usage flag bit and is used in the
	// stringer and tests to ensure all of the above constants have been
	// tested.
	highestUsageFlagBit
)

// RegisterCmd registers a new command that will automatically marshal to and
// from JSON-RPC with full type checking and positional parameter support.  It
// also accepts usage flags which identify the circumstances under which the
// command can be used.
//
// This package automatically registers all of the exported commands by default
// using this function, however it is also exported so callers can easily
// register custom types.
//
// The type format is very strict since it needs to be able to automatically
// marshal to and from JSON-RPC 1.0.  The following enumerates the requirements:
//
//   - The provided command must be a single pointer to a struct
//   - All fields must be exported
//   - The order of the positional parameters in the marshalled JSON will be in
//     the same order as declared in the struct definition
//   - Struct embedding is not supported
//   - Struct fields may NOT be channels, functions, complex, or interface
//   - A field in the provided struct with a pointer is treated as optional
//   - Multiple indirections (i.e **int) are not supported
//   - Once the first optional field (pointer) is encountered, the remaining
//     fields must also be optional fields (pointers) as required by positional
//     params
//   - A field that has a 'jsonrpcdefault' struct tag must be an optional field
//     (pointer)
//
// NOTE: This function only needs to be able to examine the structure of the
// passed struct, so it does not need to be an actual instance.  Therefore, it
// is recommended to simply pass a nil pointer cast to the appropriate type.
// For example, (*FooCmd)(nil).
func RegisterCmd(method string, cmd interface{}, flags UsageFlag) error {
	return v3.Register(method, cmd, flags)
}

// MustRegisterCmd performs the same function as RegisterCmd except it panics
// if there is an error.  This should only be called from package init
// functions.
func MustRegisterCmd(method string, cmd interface{}, flags UsageFlag) {
	v3.MustRegister(method, cmd, flags)
}

// RegisteredCmdMethods returns a sorted list of methods for all registered
// commands.
func RegisteredCmdMethods() []string {
	return v3.RegisteredMethods("")
}

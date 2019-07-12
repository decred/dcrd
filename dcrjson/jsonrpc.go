// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import v3 "github.com/decred/dcrd/dcrjson/v3"

// RPCErrorCode represents an error code to be used as a part of an RPCError
// which is in turn used in a JSON-RPC Response object.
//
// A specific type is used to help ensure the wrong errors aren't used.
type RPCErrorCode = v3.RPCErrorCode

// RPCError represents an error that is used as a part of a JSON-RPC Response
// object.
type RPCError = v3.RPCError

// NewRPCError constructs and returns a new JSON-RPC error that is suitable
// for use in a JSON-RPC Response object.
func NewRPCError(code RPCErrorCode, message string) *RPCError {
	return v3.NewRPCError(code, message)
}

// IsValidIDType checks that the ID field (which can go in any of the JSON-RPC
// requests, responses, or notifications) is valid.  JSON-RPC 1.0 allows any
// valid JSON type.  JSON-RPC 2.0 (which bitcoind follows for some parts) only
// allows string, number, or null, so this function restricts the allowed types
// to that list.  This function is only provided in case the caller is manually
// marshalling for some reason.    The functions which accept an ID in this
// package already call this function to ensure the provided id is valid.
func IsValidIDType(id interface{}) bool {
	return v3.IsValidIDType(id)
}

// Request represents raw JSON-RPC requests.  The Method field identifies the
// specific command type which in turn leads to different parameters. Callers
// typically will not use this directly since this package provides a statically
// typed command infrastructure which handles creation of these requests,
// however this struct is being exported in case the caller wants to construct
// raw requests for some reason.
type Request = v3.Request

// NewRequest returns a new JSON-RPC request object given the provided rpc
// version, id, method, and parameters.  The parameters are marshalled into a
// json.RawMessage for the Params field of the returned request object. This
// function is only provided in case the caller wants to construct raw requests
// for some reason. Typically callers will instead want to create a registered
// concrete command type with the NewCmd or New<Foo>Cmd functions and call the
// MarshalCmd function with that command to generate the marshalled JSON-RPC
// request.
func NewRequest(rpcVersion string, id interface{}, method string, params []interface{}) (*Request, error) {
	return v3.NewRequest(rpcVersion, id, method, params)
}

// Response is the general form of a JSON-RPC response.  The type of the
// Result field varies from one command to the next, so it is implemented as an
// interface.  The ID field has to be a pointer to allow for a nil value when
// empty.
type Response = v3.Response

// NewResponse returns a new JSON-RPC response object given the provided rpc
// version, id, marshalled result, and RPC error.  This function is only
// provided in case the caller wants to construct raw responses for some reason.
// Typically callers will instead want to create the fully marshalled JSON-RPC
// response to send over the wire with the MarshalResponse function.
func NewResponse(rpcVersion string, id interface{}, marshalledResult []byte, rpcErr *RPCError) (*Response, error) {
	return v3.NewResponse(rpcVersion, id, marshalledResult, rpcErr)
}

// MarshalResponse marshals the passed rpc version, id, result, and RPCError to
// a JSON-RPC response byte slice that is suitable for transmission to a
// JSON-RPC client.
func MarshalResponse(rpcVersion string, id interface{}, result interface{}, rpcErr *RPCError) ([]byte, error) {
	return v3.MarshalResponse(rpcVersion, id, result, rpcErr)
}

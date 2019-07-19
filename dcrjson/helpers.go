// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import v3 "github.com/decred/dcrd/dcrjson/v3"

// Bool is a helper routine that allocates a new bool value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Bool(v bool) *bool {
	return v3.Bool(v)
}

// Int is a helper routine that allocates a new int value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Int(v int) *int {
	return v3.Int(v)
}

// Uint is a helper routine that allocates a new uint value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Uint(v uint) *uint {
	return v3.Uint(v)
}

// Uint16 is a helper routine that allocates a new uint16 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Uint16(v uint16) *uint16 {
	return v3.Uint16(v)
}

// Int32 is a helper routine that allocates a new int32 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Int32(v int32) *int32 {
	return v3.Int32(v)
}

// Uint32 is a helper routine that allocates a new uint32 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Uint32(v uint32) *uint32 {
	return v3.Uint32(v)
}

// Int64 is a helper routine that allocates a new int64 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Int64(v int64) *int64 {
	return v3.Int64(v)
}

// Uint64 is a helper routine that allocates a new uint64 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Uint64(v uint64) *uint64 {
	return v3.Uint64(v)
}

// Float64 is a helper routine that allocates a new float64 value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func Float64(v float64) *float64 {
	return v3.Float64(v)
}

// String is a helper routine that allocates a new string value to store v and
// returns a pointer to it.  This is useful when assigning optional parameters.
func String(v string) *string {
	return v3.String(v)
}

// EstimateSmartFeeModeAddr is a helper routine that allocates a new
// EstimateSmartFeeMode value to store v and returns a pointer to it. This is
// useful when assigning optional parameters.
func EstimateSmartFeeModeAddr(v EstimateSmartFeeMode) *EstimateSmartFeeMode {
	p := new(EstimateSmartFeeMode)
	*p = v
	return p
}

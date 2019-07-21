// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// EstimateSmartFeeModeAddr is a helper routine that allocates a new
// EstimateSmartFeeMode value to store v and returns a pointer to it. This is
// useful when assigning optional parameters.
func EstimateSmartFeeModeAddr(v EstimateSmartFeeMode) *EstimateSmartFeeMode {
	p := new(EstimateSmartFeeMode)
	*p = v
	return p
}

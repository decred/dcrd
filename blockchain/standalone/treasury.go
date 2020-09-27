// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import "fmt"

// CalculateTSpendExpiry returns the only valid value relative to the next
// block height where the transaction will expire. We add two blocks in the end
// because transaction expiry is inclusive (>=) relative to blockheight.
// We try very hard to use the "natural" types but this is a giant mess.
func CalculateTSpendExpiry(nextBlockHeight int64, tvi, multiplier uint64) uint32 {
	nbh := uint64(nextBlockHeight)
	nextTVI := nbh + (tvi - (nbh % tvi)) // Round up to next TVI
	maxTVI := nextTVI + tvi*multiplier   // Max TVI allowed.

	return uint32(maxTVI + 2) // + 2 to deal with Expiry handling in mempool.
}

// IsTreasuryVoteInterval returns true if the passed height is on a Treasury
// Vote Interval.
func IsTreasuryVoteInterval(height, tvi uint64) bool {
	return height%tvi == 0 && height != 0
}

// CalcTSpendWindow calculates the start and end of a treasury voting window
// based on the parameters that are passed.  An error will be returned if the
// provided expiry is not two more than a treasury vote interval (TVI) or before
// a single voting window is possible.
func CalcTSpendWindow(expiry uint32, tvi, multiplier uint64) (uint32, uint32, error) {
	// Ensure the provided expiry is at least higher than a single voting
	// window.
	minReqExpiry := tvi*multiplier + 2
	if uint64(expiry) < minReqExpiry {
		str := fmt.Sprintf("expiry %d must be at least %d for the voting "+
			"window defined by a TVI of %d with a multiplier of %d", expiry,
			minReqExpiry, tvi, multiplier)
		return 0, 0, ruleError(ErrInvalidTSpendExpiry, str)
	}

	// Ensure the provided expiry is two more than a TVI.
	if !IsTreasuryVoteInterval(uint64(expiry-2), tvi) {
		str := fmt.Sprintf("expiry %d must be two more than a multiple of the "+
			"treasury vote interval %d", expiry, tvi)
		return 0, 0, ruleError(ErrInvalidTSpendExpiry, str)
	}

	return expiry - uint32(tvi*multiplier) - 2, expiry - 2, nil
}

// InsideTSpendWindow returns true if the provided block height is inside the
// treasury vote window of the provided expiry.  This function should only be
// called with an expiry that is on a TVI. Proper care must be taken to call
// this function with the correct blockheight. It is incumbent on the caller to
// determine if the blockheight is for the previous, current or next block.
//
// Note: The end is INCLUSIVE in order to determine if a TSPEND is allowed in a
// block despite the fact that voting window is EXCLUSIVE.
func InsideTSpendWindow(blockHeight int64, expiry uint32, tvi, multiplier uint64) bool {
	start, end, err := CalcTSpendWindow(expiry, tvi, multiplier)
	if err != nil {
		return false
	}

	return uint32(blockHeight) >= start && uint32(blockHeight) <= end
}

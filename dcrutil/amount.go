// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import v2 "github.com/decred/dcrd/dcrutil/v2"

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a coin.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in coins to an amount counted in atomic units.
type AmountUnit = v2.AmountUnit

// These constants define various units used when describing a coin
// monetary amount.
const (
	AmountMegaCoin  = v2.AmountMegaCoin
	AmountKiloCoin  = v2.AmountKiloCoin
	AmountCoin      = v2.AmountCoin
	AmountMilliCoin = v2.AmountMilliCoin
	AmountMicroCoin = v2.AmountMicroCoin
	AmountAtom      = v2.AmountAtom
)

// Amount represents the base coin monetary unit (colloquially referred
// to as an `Atom').  A single Amount is equal to 1e-8 of a coin.
type Amount = v2.Amount

// NewAmount creates an Amount from a floating point value representing
// some value in the currency.  NewAmount errors if f is NaN or +-Infinity,
// but does not check that the amount is within the total amount of coins
// producible as f may not refer to an amount at a single moment in time.
//
// NewAmount is for specifically for converting DCR to Atoms (atomic units).
// For creating a new Amount with an int64 value which denotes a quantity of
// Atoms, do a simple type conversion from type int64 to Amount.
// See GoDoc for example: http://godoc.org/github.com/decred/dcrd/dcrutil#example-Amount
func NewAmount(f float64) (Amount, error) {
	return v2.NewAmount(f)
}

// AmountSorter implements sort.Interface to allow a slice of Amounts to
// be sorted.
type AmountSorter = v2.AmountSorter

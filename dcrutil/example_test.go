// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil_test

import (
	"fmt"
	"math"

	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/dcrec/v2"
	"github.com/decred/dcrd/dcrutil/v3"
)

func ExampleAmount() {

	a := dcrutil.Amount(0)
	fmt.Println("Zero Atom:", a)

	a = dcrutil.Amount(1e8)
	fmt.Println("100,000,000 Atoms:", a)

	a = dcrutil.Amount(1e5)
	fmt.Println("100,000 Atoms:", a)
	// Output:
	// Zero Atom: 0 DCR
	// 100,000,000 Atoms: 1 DCR
	// 100,000 Atoms: 0.001 DCR
}

func ExampleNewAmount() {
	amountOne, err := dcrutil.NewAmount(1)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountOne) //Output 1

	amountFraction, err := dcrutil.NewAmount(0.01234567)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountFraction) //Output 2

	amountZero, err := dcrutil.NewAmount(0)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountZero) //Output 3

	amountNaN, err := dcrutil.NewAmount(math.NaN())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountNaN) //Output 4

	// Output: 1 DCR
	// 0.01234567 DCR
	// 0 DCR
	// invalid coin amount
}

func ExampleAmount_unitConversions() {
	amount := dcrutil.Amount(44433322211100)

	fmt.Println("Atom to kCoin:", amount.Format(dcrutil.AmountKiloCoin))
	fmt.Println("Atom to Coin:", amount)
	fmt.Println("Atom to MilliCoin:", amount.Format(dcrutil.AmountMilliCoin))
	fmt.Println("Atom to MicroCoin:", amount.Format(dcrutil.AmountMicroCoin))
	fmt.Println("Atom to Atom:", amount.Format(dcrutil.AmountAtom))

	// Output:
	// Atom to kCoin: 444.333222111 kDCR
	// Atom to Coin: 444333.222111 DCR
	// Atom to MilliCoin: 444333222.111 mDCR
	// Atom to MicroCoin: 444333222111 μDCR
	// Atom to Atom: 44433322211100 Atom
}

// This example demonstrates decoding addresses, determining their underlying
// type, and displaying their associated underlying hash160 and digital
// signature algorithm.
func ExampleDecodeAddress() {
	// Ordinarily addresses would be read from the user or the result of a
	// derivation, but they are hard coded here for the purposes of this
	// example.
	mainNetParmas := chaincfg.MainNetParams()
	addrsToDecode := []string{
		"DsRUvfCwTMrKz29dDiQBJhZii9GDN3bVx6Q", // pay-to-pubkey-hash ecdsa
		"DSpf9Sru9MarMKQQnuzTiQ9tjWVJA3KSm2d", // pay-to-pubkey-hash schnorr
	}
	for idx, encodedAddr := range addrsToDecode {
		addr, err := dcrutil.DecodeAddress(encodedAddr, mainNetParmas)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("addr%d hash160: %x\n", idx, *addr.Hash160())

		// The example addresses are pay-to-pubkey-hash with different signature
		// algorithms, so this code is limited to that type
		switch a := addr.(type) {
		case *dcrutil.AddressPubKeyHash:
			// Determine and display the digital signature algorithm.
			algo := "unknown"
			switch a.DSA() {
			case dcrec.STEcdsaSecp256k1:
				algo = "ECDSA"
			case dcrec.STSchnorrSecp256k1:
				algo = "Schnorr"
			}
			fmt.Printf("addr%d DSA: %v\n", idx, algo)

		default:
			fmt.Println("Unexpected test address type")
			return
		}
	}

	// Output:
	// addr0 hash160: 05ad744deacf5334671d3e62db86230af1891f71
	// addr0 DSA: ECDSA
	// addr1 hash160: e280cb6e66b96679aec288b1fbdbd4db08077a1b
	// addr1 DSA: Schnorr
}

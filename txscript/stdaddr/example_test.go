// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdaddr_test

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// This example demonstrates decoding addresses, generating their payment
// scripts and associated script versions, determining supported capabilities by
// checking if interfaces are implemented, obtaining the associated underlying
// hash160 for addresses that support it, converting public key addresses to
// their public key hash variant, and generating stake-related scripts for
// addresses that can be used in the staking system.
func ExampleDecodeAddress() {
	// Ordinarily addresses would be read from the user or the result of a
	// derivation, but they are hard coded here for the purposes of this
	// example.
	simNetParams := chaincfg.SimNetParams()
	addrsToDecode := []string{
		// v0 pay-to-pubkey ecdsa
		"SkLUJQxtYoVrewN6fwqsU6JQjxLs5a6xfcTsGfUYiLr2AUY6HuLMN",

		// v0 pay-to-pubkey-hash ecdsa
		"Sspzuh5xuvqxccYLWJDJjCtqp166NRxcaPB",

		// v0 pay-to-pubkey schnorr
		"SkLYBuKMSsCi1PdjqMf2i6D3wWB6K1QNMN39Qsxr68qLBFXMTwcpG",

		// v0 pay-to-pubkey-hash schnorr
		"SSt3WeMV3ufEHufh8nCey97y2yp7tNdPyES",

		// v0 pay-to-script-hash
		"ScrkZMau4jj7JUHUvU4YMMRRi4w1o3Wp1vY",
	}
	for idx, encodedAddr := range addrsToDecode {
		addr, err := stdaddr.DecodeAddress(encodedAddr, simNetParams)
		if err != nil {
			fmt.Println(err)
			return
		}

		// Obtain the payment script and associated script version that would
		// ordinarily by used in a transaction output to send funds to the
		// address.
		scriptVer, script := addr.PaymentScript()
		fmt.Printf("addr%d: %s\n", idx, addr)
		fmt.Printf("  payment script version: %d\n", scriptVer)
		fmt.Printf("  payment script: %x\n", script)

		// Access the RIPEMD-160 hash from addresses that involve it.
		if h160er, ok := addr.(stdaddr.Hash160er); ok {
			fmt.Printf("  hash160: %x\n", *h160er.Hash160())
		}

		// Demonstrate converting public key addresses to the public key hash
		// variant when supported.  This is primarily provided for convenience
		// when the caller already happens to have the public key address handy
		// such as in cases where public keys are shared through some protocol.
		if pkHasher, ok := addr.(stdaddr.AddressPubKeyHasher); ok {
			fmt.Printf("  p2pkh addr: %s\n", pkHasher.AddressPubKeyHash())
		}

		// Obtain stake-related scripts and associated script versions that
		// would ordinarily be used in stake transactions such as ticket
		// purchases and votes for supported addresses.
		//
		// Note that only very specific addresses can be used as destinations in
		// the staking system and this approach provides a capabilities based
		// mechanism to determine support.
		if stakeAddr, ok := addr.(stdaddr.StakeAddress); ok {
			// Obtain the voting rights script and associated script version
			// that would ordinarily by used in a ticket purchase transaction to
			// give voting rights to the address.
			voteScriptVer, voteScript := stakeAddr.VotingRightsScript()
			fmt.Printf("  voting rights script version: %d\n", voteScriptVer)
			fmt.Printf("  voting rights script: %x\n", voteScript)

			// Obtain the rewards commitment script and associated script
			// version that would ordinarily by used in a ticket purchase
			// transaction to commit the original funds locked plus the reward
			// to the address.
			//
			// Ordinarily the reward amount and fee limits would need to be
			// calculated correctly, but they are hard coded here for the
			// purposes of this example.
			const rewardAmount = 1e8
			const feeLimit = 0x5800
			rewardScriptVer, rewardScript := stakeAddr.RewardCommitmentScript(
				rewardAmount, feeLimit)
			fmt.Printf("  reward script version: %d\n", rewardScriptVer)
			fmt.Printf("  reward script: %x\n", rewardScript)
		}
	}

	// Output:
	// addr0: SkLUJQxtYoVrewN6fwqsU6JQjxLs5a6xfcTsGfUYiLr2AUY6HuLMN
	//   payment script version: 0
	//   payment script: 210279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798ac
	//   p2pkh addr: Sspzuh5xuvqxccYLWJDJjCtqp166NRxcaPB
	// addr1: Sspzuh5xuvqxccYLWJDJjCtqp166NRxcaPB
	//   payment script version: 0
	//   payment script: 76a914e280cb6e66b96679aec288b1fbdbd4db08077a1b88ac
	//   hash160: e280cb6e66b96679aec288b1fbdbd4db08077a1b
	//   voting rights script version: 0
	//   voting rights script: ba76a914e280cb6e66b96679aec288b1fbdbd4db08077a1b88ac
	//   reward script version: 0
	//   reward script: 6a1ee280cb6e66b96679aec288b1fbdbd4db08077a1b00e1f505000000000058
	// addr2: SkLYBuKMSsCi1PdjqMf2i6D3wWB6K1QNMN39Qsxr68qLBFXMTwcpG
	//   payment script version: 0
	//   payment script: 210279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179852be
	//   p2pkh addr: SSt3WeMV3ufEHufh8nCey97y2yp7tNdPyES
	// addr3: SSt3WeMV3ufEHufh8nCey97y2yp7tNdPyES
	//   payment script version: 0
	//   payment script: 76a914e280cb6e66b96679aec288b1fbdbd4db08077a1b8852be
	//   hash160: e280cb6e66b96679aec288b1fbdbd4db08077a1b
	// addr4: ScrkZMau4jj7JUHUvU4YMMRRi4w1o3Wp1vY
	//   payment script version: 0
	//   payment script: a914ae7cd0a69b915796aa9318e1ad74f3579bfcb36587
	//   hash160: ae7cd0a69b915796aa9318e1ad74f3579bfcb365
	//   voting rights script version: 0
	//   voting rights script: baa914ae7cd0a69b915796aa9318e1ad74f3579bfcb36587
	//   reward script version: 0
	//   reward script: 6a1eae7cd0a69b915796aa9318e1ad74f3579bfcb36500e1f505000000800058
}

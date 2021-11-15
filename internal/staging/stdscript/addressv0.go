// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// addrToSlice is a convenience function that returns a slice containing the
// passed address if the given error is nil and the address is NOT nil.
func addrToSlice(addr stdaddr.Address, err error) []stdaddr.Address {
	if err != nil || addr == nil {
		return nil
	}
	return []stdaddr.Address{addr}
}

// ExtractAddrsV0 analyzes the passed version 0 public key script and returns
// the associated script type along with any addresses associated with it when
// possible.
//
// This function only works for standard script types and any data that fails to
// produce a valid address is omitted from the results.  This means callers must
// not blindly assume the slice will be of a particular length for a given
// returned script type and should always check the length prior to access in
// case the addresses were not able to be created.
func ExtractAddrsV0(pkScript []byte, params stdaddr.AddressParamsV0) (ScriptType, []stdaddr.Address) {
	// Check for pay-to-pubkey-hash-ecdsa-secp256k1 script.
	if h := ExtractPubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STPubKeyHashEcdsaSecp256k1, addrToSlice(addr, err)
	}

	// Check for pay-to-script-hash.
	if h := ExtractScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STScriptHash, addrToSlice(addr, err)
	}

	// Check for pay-to-pubkey-hash-ed25519 script.
	if data := ExtractPubKeyHashEd25519V0(pkScript); data != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEd25519V0(data, params)
		return STPubKeyHashEd25519, addrToSlice(addr, err)
	}

	// Check for pay-to-pubkey-hash-schnorr-secp256k1 script.
	if data := ExtractPubKeyHashSchnorrSecp256k1V0(pkScript); data != nil {
		addr, err := stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(data, params)
		return STPubKeyHashSchnorrSecp256k1, addrToSlice(addr, err)
	}

	// Check for pay-to-pubkey script.
	if data := ExtractPubKeyV0(pkScript); data != nil {
		// Note that this parse is done because the address is intentionally
		// limited to compressed pubkeys, but consensus technically allows both
		// compressed and uncompressed pubkeys for the underlying script.
		var addrs []stdaddr.Address
		pk, err := secp256k1.ParsePubKey(data)
		if err == nil {
			addr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk, params)
			addrs = addrToSlice(addr, err)
		}
		return STPubKeyEcdsaSecp256k1, addrs
	}

	// Check for pay-to-pubkey-ed25519 script.
	if data := ExtractPubKeyEd25519V0(pkScript); data != nil {
		addr, err := stdaddr.NewAddressPubKeyEd25519V0Raw(data, params)
		return STPubKeyEd25519, addrToSlice(addr, err)
	}

	// Check for pay-to-pubkey-schnorr-secp256k1 script.
	if data := ExtractPubKeySchnorrSecp256k1V0(pkScript); data != nil {
		addr, err := stdaddr.NewAddressPubKeySchnorrSecp256k1V0Raw(data, params)
		return STPubKeySchnorrSecp256k1, addrToSlice(addr, err)
	}

	// Check for multi-signature script.
	details := ExtractMultiSigScriptDetailsV0(pkScript, true)
	if details.Valid {
		// Convert the public keys while skipping any that are invalid.  Also,
		// only allocate the slice of addresses if at least one valid address is
		// found to avoid an unnecessary heap alloc that would otherwise happen
		// when there are no valid addresses because the slice is returned.
		var addrs []stdaddr.Address
		for i := uint16(0); i < details.NumPubKeys; i++ {
			pubkey, err := secp256k1.ParsePubKey(details.PubKeys[i])
			if err == nil {
				addr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pubkey, params)
				if err == nil {
					if addrs == nil {
						addrs = make([]stdaddr.Address, 0, details.NumPubKeys-i)
					}
					addrs = append(addrs, addr)
				}
			}
		}
		return STMultiSig, addrs
	}

	// Check for stake submission script.  Only stake-submission-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if h := ExtractStakeSubmissionPubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STStakeSubmissionPubKeyHash, addrToSlice(addr, err)
	}
	if h := ExtractStakeSubmissionScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STStakeSubmissionScriptHash, addrToSlice(addr, err)
	}

	// Check for stake generation script.  Only stake-generation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if h := ExtractStakeGenPubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STStakeGenPubKeyHash, addrToSlice(addr, err)
	}
	if h := ExtractStakeGenScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STStakeGenScriptHash, addrToSlice(addr, err)
	}

	// Check for stake revocation script.  Only stake-revocation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if h := ExtractStakeRevocationPubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STStakeRevocationPubKeyHash, addrToSlice(addr, err)
	}
	if h := ExtractStakeRevocationScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STStakeRevocationScriptHash, addrToSlice(addr, err)
	}

	// Check for stake change script.  Only stake-change-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if h := ExtractStakeChangePubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STStakeChangePubKeyHash, addrToSlice(addr, err)
	}
	if h := ExtractStakeChangeScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STStakeChangeScriptHash, addrToSlice(addr, err)
	}

	// Check for null data script.
	if IsNullDataScriptV0(pkScript) {
		// Null data scripts do not have an associated address.
		return STNullData, nil
	}

	// Check for treasury add.
	if IsTreasuryAddScriptV0(pkScript) {
		return STTreasuryAdd, nil
	}

	// Check for treasury generation script.  Only treasury-gen-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if h := ExtractTreasuryGenPubKeyHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h, params)
		return STTreasuryGenPubKeyHash, addrToSlice(addr, err)
	}
	if h := ExtractTreasuryGenScriptHashV0(pkScript); h != nil {
		addr, err := stdaddr.NewAddressScriptHashV0FromHash(h, params)
		return STTreasuryGenScriptHash, addrToSlice(addr, err)
	}

	// Don't attempt to extract addresses for nonstandard transactions.
	return STNonStandard, nil
}

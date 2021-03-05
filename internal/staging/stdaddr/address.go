// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package stdaddr provides facilities for working with human-readable Decred
// payment addresses.
package stdaddr

import (
	"fmt"

	"github.com/decred/dcrd/crypto/ripemd160"
)

// AddressParams defines an interface that is used to provide the parameters
// required when encoding and decoding addresses.  These values are typically
// well-defined and unique per network.
type AddressParams interface {
	AddressParamsV0
}

// Address represents any type of destination a transaction output may spend to.
// Some examples include pay-to-pubkey (P2PK), pay-to-pubkey-hash (P2PKH), and
// pay-to-script-hash (P2SH).  Address is designed to be generic enough that
// other kinds of addresses may be added in the future without changing the
// decoding and encoding API.
type Address interface {
	// Address returns the string encoding of the payment address for the
	// associated script version and payment script.
	Address() string

	// PaymentScript returns the script version associated with the address
	// along with a script to pay a transaction output to the address.
	PaymentScript() (uint16, []byte)
}

// StakeAddress is an interface for generating the specialized scripts that are
// used in the staking system.  Only specific address types are supported by the
// staking system and therefore only those address types will implement this
// interface.
//
// Version 1 and 3 staking transactions only support version 0 scripts and
// address types of AddressPubKeyHashEcdsaSecp256k1V0 and AddressScriptHashV0.
//
// Callers can programmatically assert a specific address implements this
// interface to access the methods.
type StakeAddress interface {
	Address

	// VotingRightsScript returns the script version associated with the address
	// along with a script to give voting rights to the address.  It is only
	// valid when used in stake ticket purchase transactions.
	VotingRightsScript() (uint16, []byte)

	// RewardCommitmentScript returns the script version associated with the
	// address along with a script that commits the original funds locked to
	// purchase a ticket plus the reward to the address along with limits to
	// impose on any fees.
	RewardCommitmentScript(amount int64, limits uint16) (uint16, []byte)

	// StakeChangeScript returns the script version associated with the address
	// along with a script to pay change to the address.  It is only valid when
	// used in stake ticket purchase and treasury add transactions.
	StakeChangeScript() (uint16, []byte)

	// PayVoteCommitmentScript returns the script version associated with the
	// address along with a script to pay the original funds locked to purchase
	// a ticket plus the reward to the address.  The address must have
	// previously been committed to by the ticket purchase.  The script is only
	// valid when used in stake vote transactions whose associated tickets are
	// eligible to vote.
	PayVoteCommitmentScript() (uint16, []byte)

	// PayRevokeCommitmentScript returns the script version associated with the
	// address along with a script to revoke an expired or missed ticket which
	// pays the original funds locked to purchase a ticket to the address.  The
	// address must have previously been committed to by the ticket purchase.
	// The script is only valid when used in stake revocation transactions whose
	// associated tickets have been missed or have expired.
	PayRevokeCommitmentScript() (uint16, []byte)

	// PayFromTreasuryScript returns the script version associated with the
	// address along with a script that pays funds from the treasury to the
	// address.  The script is only valid when used in treasury spend
	// transactions.
	PayFromTreasuryScript() (uint16, []byte)
}

// AddressPubKeyHasher is an interface for public key addresses that can be
// converted to an address that imposes an encumbrance that requires the public
// key that hashes to a given public key hash along with a valid signature for
// that public key.
//
// The specific public key and signature types are dependent on the original
// address.
type AddressPubKeyHasher interface {
	AddressPubKeyHash() Address
}

// Hash160er is an interface that allows the RIPEMD-160 hash to be obtained from
// addresses that involve them.
type Hash160er interface {
	Hash160() *[ripemd160.Size]byte
}

// Secp256k1PublicKey is an interface that represents a secp256k1 public key for
// use in creating pay-to-pubkey addresses that involve them.
type Secp256k1PublicKey interface {
	// SerializeCompressed serializes a public key in the 33-byte compressed
	// format.
	SerializeCompressed() []byte
}

// NewAddressPubKeyEcdsaSecp256k1Raw returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key.
//
// The provided public key MUST be a valid secp256k1 public key serialized in
// the _compressed_ format or an error will be returned.
//
// See NewAddressPubKeyEcdsaSecp256k1 for a variant that accepts the public
// key as a concrete type instance instead.
//
// This function can be useful to callers who already need the serialized public
// key for other purposes to avoid the need to serialize it multiple times.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyEcdsaSecp256k1Raw(scriptVersion uint16,
	serializedPubKey []byte,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyEcdsaSecp256k1V0Raw(serializedPubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeyEcdsaSecp256k1 returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key.
//
// See NewAddressPubKeyEcdsaSecp256k1Raw for a variant that accepts the public
// key already serialized in the _compressed_ format instead of a concrete type.
// It can be useful to callers who already need the serialized public key for
// other purposes to avoid the need to serialize it multiple times.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyEcdsaSecp256k1(scriptVersion uint16,
	pubKey Secp256k1PublicKey,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyEcdsaSecp256k1V0(pubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeyEd25519Raw returns an address that represents a payment
// destination which imposes an encumbrance that requires a valid Ed25519
// signature for a specific Ed25519 public key.
//
// See NewAddressPubKeyEd25519 for a variant that accepts the public key as a
// concrete type instance instead.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyEd25519Raw(scriptVersion uint16, serializedPubKey []byte,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyEd25519V0Raw(serializedPubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// Ed25519PublicKey is an interface type that represents an Ed25519 public key
// for use in creating pay-to-pubkey addresses that involve them.
type Ed25519PublicKey interface {
	// Serialize serializes the public key in a 32-byte compressed little endian
	// format.
	Serialize() []byte
}

// NewAddressPubKeyEd25519 returns an address that represents a payment
// destination which imposes an encumbrance that requires a valid Ed25519
// signature for a specific Ed25519 public key.
//
// See NewAddressPubKeyEd25519Raw for a variant that accepts the public key
// already serialized instead of a concrete type.  It can be useful to callers
// who already need the serialized public key for other purposes to avoid the
// need to serialize it multiple times.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyEd25519(scriptVersion uint16, pubKey Ed25519PublicKey,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyEd25519V0(pubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeySchnorrSecp256k1Raw returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid
// EC-Schnorr-DCR signature for a specific secp256k1 public key.
//
// The provided public key MUST be a valid secp256k1 public key serialized in
// the _compressed_ format or an error will be returned.
//
// See NewAddressPubKeySchnorrSecp256k1 for a variant that accepts the public
// key as a concrete type instance instead.
//
// This function can be useful to callers who already need the serialized public
// key for other purposes to avoid the need to serialize it multiple times.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeySchnorrSecp256k1Raw(scriptVersion uint16,
	serializedPubKey []byte,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeySchnorrSecp256k1V0Raw(serializedPubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeySchnorrSecp256k1 returns an address that represents a payment
// destination which imposes an encumbrance that requires a valid EC-Schnorr-DCR
// signature for a specific secp256k1 public key.
//
// See NewAddressPubKeySchnorrSecp256k1Raw for a variant that accepts the public
// key already serialized in the _compressed_ format instead of a concrete type.
// It can be useful to callers who already need the serialized public key for
// other purposes to avoid the need to serialize it multiple times.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeySchnorrSecp256k1(scriptVersion uint16,
	pubKey Secp256k1PublicKey,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeySchnorrSecp256k1V0(pubKey, params)
	}

	str := fmt.Sprintf("pubkey addresses for version %d are not supported",
		scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeyHashEcdsaSecp256k1 returns an address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key that hashes to the provided public key hash along with a valid
// ECDSA signature for that public key.
//
// For version 0 scripts, the provided public key hash must be 20 bytes and is
// expected to be the Hash160 of the associated secp256k1 public key serialized
// in the _compressed_ format.
//
// It is important to note that while it is technically possible for legacy
// reasons to create this specific type of address based on the hash of a public
// key in the uncompressed format, so long as it is also redeemed with that same
// public key in uncompressed format, it is *HIGHLY* recommended to use the
// compressed format since it occupies less space on the chain and is more
// consistent with other address formats where uncompressed public keys are NOT
// supported.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyHashEcdsaSecp256k1(scriptVersion uint16, pkHash []byte,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyHashEcdsaSecp256k1V0(pkHash, params)
	}

	str := fmt.Sprintf("pubkey hash addresses for version %d are not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeyHashEd25519 returns an address that represents a a payment
// destination which imposes an encumbrance that requires an Ed25519 public key
// that hashes to the provided public key hash along with a valid Ed25519
// signature for that public key.
//
// For version 0 scripts, the provided public key hash must be 20 bytes and be
// the Hash160 of the correct public key or it will not be redeemable with the
// expected public key because it would hash to a different value than the
// payment script generated for the provided incorrect public key hash expects.
//
// NOTE: Version 0 scripts are the only currently supported version.
func NewAddressPubKeyHashEd25519(scriptVersion uint16, pkHash []byte,
	params AddressParams) (Address, error) {

	switch scriptVersion {
	case 0:
		return NewAddressPubKeyHashEd25519V0(pkHash, params)
	}

	str := fmt.Sprintf("pubkey hash addresses for version %d are not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressPubKeyHashSchnorrSecp256k1 returns an address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key in the _compressed_ format that hashes to the provided public key
// hash along with a valid EC-Schnorr-DCR signature for that public key.
//
// For version 0 scripts, the provided public key hash must be 20 bytes and is
// expected to be the Hash160 of the associated secp256k1 public key serialized
// in the _compressed_ format.
//
// WARNING: It is important to note that, unlike in the case of the ECDSA
// variant of this type of address, redemption via a public key in the
// uncompressed format is NOT supported by the consensus rules for this type, so
// it is *EXTREMELY* important to ensure the provided hash is of the serialized
// public key in the compressed format or the associated coins will NOT be
// redeemable.
func NewAddressPubKeyHashSchnorrSecp256k1(scriptVersion uint16, pkHash []byte,
	params AddressParams) (Address, error) {

	str := fmt.Sprintf("pubkey hash addresses for version %d are not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressScriptHashFromHash returns an address that represents a payment
// destination which imposes an encumbrance that requires a script that hashes
// to the provided script hash along with all of the encumbrances that script
// itself imposes.  The script is commonly referred to as a redeem script.
//
// For version 0 scripts, the provided script hash must be 20 bytes and is
// expected to be the Hash160 of the associated redeem script.
//
// See NewAddressScriptHash for a variant that accepts the redeem script instead
// of its hash.  It can be used as a convenience for callers that have the
// redeem script available.
func NewAddressScriptHashFromHash(scriptVersion uint16, scriptHash []byte,
	params AddressParams) (Address, error) {

	str := fmt.Sprintf("script hash addresses for version %d are not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// NewAddressScriptHash returns an address that represents a payment destination
// which imposes an encumbrance that requires a script that hashes to the same
// value as the provided script along with all of the encumbrances that script
// itself imposes.  The script is commonly referred to as a redeem script.
//
// See NewAddressScriptHashFromHash for a variant that accepts the hash of the
// script directly instead of the script.  It can be useful to callers that
// either already have the script hash available or do not know the associated
// script.
func NewAddressScriptHash(scriptVersion uint16, redeemScript []byte,
	params AddressParams) (Address, error) {

	str := fmt.Sprintf("script hash addresses for version %d are not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// probablyV0Base58Addr returns true when the provided string looks like a
// version 0 base58 address as determined by their length and only containing
// runes in the base58 alphabet used by Decred for version 0 addresses.
func probablyV0Base58Addr(s string) bool {
	// Ensure the length is one of the possible values for supported version 0
	// addresses.
	if len(s) != 35 && len(s) != 53 {
		return false
	}

	// The modified base58 alphabet used by Decred for version 0 addresses is:
	//   123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz
	for _, r := range s {
		if r < '1' || r > 'z' ||
			r == 'I' || r == 'O' || r == 'l' ||
			(r > '9' && r < 'A') || (r > 'Z' && r < 'a') {

			return false
		}
	}

	return true
}

// DecodeAddress decodes the string encoding of an address and returns the
// relevant Address if it is a valid encoding for a known address type and is
// for the provided network.
func DecodeAddress(addr string, params AddressParams) (Address, error) {
	// Parsing code for future address/script versions should be added as the
	// most recent case in the switch statement.  The expectation is that newer
	// version addresses will become more common, so they should be checked
	// first.
	switch {
	case probablyV0Base58Addr(addr):
		return DecodeAddressV0(addr, params)
	}

	str := fmt.Sprintf("address %q is not a supported type", addr)
	return nil, makeError(ErrUnsupportedAddress, str)
}

// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdaddr

import (
	"errors"
	"fmt"

	"github.com/decred/base58"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// These are redefinitions of version 0 opcodes in the txscript package that are
// used in this package to generate payment scripts.  Ultimately, this should be
// using the constant definitions from txscript instead, but it would currently
// create a cyclic dependency since txscript will need to depend on this package
// for signing.
const (
	opData33   = 0x21
	opCheckSig = 0xac
)

const (
	// sigTypeSecp256k1PubKeyCompOddFlag specifies the bitmask to apply to the
	// pubkey address signature type byte for those that deal with compressed
	// secp256k1 pubkeys to specify the omitted y coordinate is odd.
	sigTypeSecp256k1PubKeyCompOddFlag = uint8(1 << 7)
)

// AddressParamsV0 defines an interface that is used to provide the parameters
// required when encoding and decoding addresses for version 0 scripts.  These
// values are typically well-defined and unique per network.
type AddressParamsV0 interface {
	// AddrIDPubKeyV0 returns the magic prefix bytes for version 0 pay-to-pubkey
	// addresses.
	AddrIDPubKeyV0() [2]byte

	// AddrIDPubKeyHashECDSAV0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey is secp256k1 and
	// the signature algorithm is ECDSA.
	AddrIDPubKeyHashECDSAV0() [2]byte

	// AddrIDPubKeyHashEd25519V0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey and signature
	// algorithm are Ed25519.
	AddrIDPubKeyHashEd25519V0() [2]byte

	// AddrIDPubKeyHashSchnorrV0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey is secp256k1 and
	// the signature algorithm is Schnorr.
	AddrIDPubKeyHashSchnorrV0() [2]byte

	// AddrIDScriptHashV0 returns the magic prefix bytes for version 0
	// pay-to-script-hash addresses.
	AddrIDScriptHashV0() [2]byte
}

// encodeAddressV0 returns a human-readable payment address for the data and
// netID which encodes the network and address type using the format for version
// 0 scripts.
func encodeAddressV0(data []byte, netID [2]byte) string {
	// The overall format for an address for version 0 scripts is the base58
	// check encoding of data which varies by address type.  In other words, it
	// is:
	//
	//   2-byte network and address type || data || 4-byte checksum
	return base58.CheckEncode(data, netID)
}

// AddressPubKeyEcdsaSecp256k1V0 specifies an address that represents a payment
// destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key.
//
// This is commonly referred to as pay-to-pubkey (P2PK) for legacy reasons,
// however, since it is possible to support multiple algorithm and signature
// scheme combinations, it is technically more accurate to refer to it as
// pay-to-pubkey-ecdsa-secp256k1.
type AddressPubKeyEcdsaSecp256k1V0 struct {
	pubKeyID         [2]byte
	pubKeyHashID     [2]byte
	serializedPubKey []byte
}

// Ensure AddressPubKeyEcdsaSecp256k1V0 implements the Address interface.
var _ Address = (*AddressPubKeyEcdsaSecp256k1V0)(nil)

// NewAddressPubKeyEcdsaSecp256k1V0Raw returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key using version 0 scripts.
//
// The provided public key MUST be a valid secp256k1 public key serialized in
// the _compressed_ format or an error will be returned.
//
// See NewAddressPubKeyEcdsaSecp256k1V0 for a variant that accepts the public
// key as a concrete type instance instead.
//
// This function can be useful to callers who already need the serialized public
// key for other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeyEcdsaSecp256k1V0Raw(serializedPubKey []byte,
	params AddressParamsV0) (*AddressPubKeyEcdsaSecp256k1V0, error) {

	// Attempt to parse the provided public key to ensure it is both a valid
	// serialization and that it is a valid point on the secp256k1 curve.
	_, err := secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		str := fmt.Sprintf("failed to parse public key: %v", err)
		return nil, makeError(ErrInvalidPubKey, str)
	}

	// Ensure the provided serialized public key is in the compressed format.
	// This probably should be returned from secp256k1, but do it here to avoid
	// API churn.  The pubkey is known to be valid since it parsed above, so
	// it's safe to simply examine the leading byte to get the format.
	//
	// Notice that both the uncompressed and hybrid forms are intentionally not
	// supported.
	switch serializedPubKey[0] {
	case secp256k1.PubKeyFormatCompressedEven:
	case secp256k1.PubKeyFormatCompressedOdd:
	default:
		str := fmt.Sprintf("serialized public key %x is not a valid format",
			serializedPubKey)
		return nil, makeError(ErrInvalidPubKeyFormat, str)
	}

	return &AddressPubKeyEcdsaSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashECDSAV0(),
		serializedPubKey: serializedPubKey,
	}, nil
}

// NewAddressPubKeyEcdsaSecp256k1V0 returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key using version 0 scripts.
//
// See NewAddressPubKeyEcdsaSecp256k1V0Raw for a variant that accepts the public
// key already serialized in the _compressed_ format instead of a concrete type.
// It can be useful to callers who already need the serialized public key for
// other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeyEcdsaSecp256k1V0(pubKey Secp256k1PublicKey,
	params AddressParamsV0) (*AddressPubKeyEcdsaSecp256k1V0, error) {

	return &AddressPubKeyEcdsaSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashECDSAV0(),
		serializedPubKey: pubKey.SerializeCompressed(),
	}, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEcdsaSecp256k1V0) Address() string {
	// The format for the data portion of a public key address used with
	// elliptic curves is:
	//   identifier byte || 32-byte X coordinate
	//
	// The identifier byte specifies the curve and signature scheme combination
	// as well as encoding the oddness of the Y coordinate for secp256k1 public
	// keys in the high bit.
	var data [33]byte
	data[0] = byte(dcrec.STEcdsaSecp256k1)
	if addr.serializedPubKey[0] == secp256k1.PubKeyFormatCompressedOdd {
		data[0] |= sigTypeSecp256k1PubKeyCompOddFlag
	}
	copy(data[1:], addr.serializedPubKey[1:])
	return encodeAddressV0(data[:], addr.pubKeyID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEcdsaSecp256k1V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-ecdsa-secp256k1 script is one of the following forms:
	//  <33-byte compressed pubkey> CHECKSIG
	//  <65-byte uncompressed pubkey> CHECKSIG
	//
	// However, this address type intentionally only supports the compressed
	// form.
	var script [35]byte
	script[0] = opData33
	copy(script[1:34], addr.serializedPubKey)
	script[34] = opCheckSig
	return 0, script[:]
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyEcdsaSecp256k1V0) String() string {
	return addr.Address()
}

// DecodeAddressV0 decodes the string encoding of an address and returns the
// relevant Address if it is a valid encoding for a known version 0 address type
// and is for the network identified by the provided parameters.
func DecodeAddressV0(addr string, params AddressParamsV0) (Address, error) {
	// Attempt to decode the address and address type.
	decoded, addrID, err := base58.CheckDecode(addr)
	if err != nil {
		kind := ErrMalformedAddress
		if errors.Is(err, base58.ErrChecksum) {
			kind = ErrBadAddressChecksum
		}
		str := fmt.Sprintf("failed to decoded address %q: %v", addr, err)
		return nil, makeError(kind, str)
	}

	// Decode the address according to the address type.
	switch addrID {
	case params.AddrIDScriptHashV0():
		return NewAddressScriptHashFromHash(0, decoded, params)

	case params.AddrIDPubKeyHashECDSAV0():
		return NewAddressPubKeyHashEcdsaSecp256k1(0, decoded, params)

	case params.AddrIDPubKeyHashSchnorrV0():
		return NewAddressPubKeyHashSchnorrSecp256k1(0, decoded, params)

	case params.AddrIDPubKeyHashEd25519V0():
		return NewAddressPubKeyHashEd25519(0, decoded, params)

	case params.AddrIDPubKeyV0():
		// Ensure the decoded data has the expected signature type identifier
		// byte.
		if len(decoded) < 1 {
			str := fmt.Sprintf("address %q decoded data is empty", addr)
			return nil, makeError(ErrMalformedAddressData, str)
		}

		// Decode according to the crypto algorithm and signature scheme.
		sigType := decoded[0] & ^sigTypeSecp256k1PubKeyCompOddFlag
		switch dcrec.SignatureType(sigType) {
		case dcrec.STEcdsaSecp256k1:
			// The encoded data for this case is the 32-byte X coordinate for a
			// secp256k1 public key along with the oddness of the Y coordinate
			// encoded via the high bit of the first byte.
			//
			// Reconstruct the standard compressed serialized public key format
			// by choosing the correct prefix byte depending on the encoded
			// Y-coordinate oddness pass it along to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			const reqPubKeyLen = 33
			if len(decoded) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(decoded), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}
			isOddY := decoded[0]&sigTypeSecp256k1PubKeyCompOddFlag != 0
			prefix := secp256k1.PubKeyFormatCompressedEven
			if isOddY {
				prefix = secp256k1.PubKeyFormatCompressedOdd
			}
			decoded[0] = prefix
			return NewAddressPubKeyEcdsaSecp256k1Raw(0, decoded, params)

		case dcrec.STEd25519:
			const reqPubKeyLen = 32
			pubKey := decoded[1:]
			if len(pubKey) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(pubKey), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}

			// The encoded data for this case is the actual Ed25519 public key,
			// so just pass it along unaltered to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			return NewAddressPubKeyEd25519Raw(0, pubKey, params)

		case dcrec.STSchnorrSecp256k1:
			// The encoded data for this case is the 32-byte X coordinate for a
			// secp256k1 public key along with the oddness of the Y coordinate
			// encoded via the high bit of the first byte.
			//
			// Reconstruct the standard compressed serialized public key format
			// by choosing the correct prefix byte depending on the encoded
			// Y-coordinate oddness pass it along to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			const reqPubKeyLen = 33
			if len(decoded) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(decoded), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}
			isOddY := decoded[0]&sigTypeSecp256k1PubKeyCompOddFlag != 0
			prefix := secp256k1.PubKeyFormatCompressedEven
			if isOddY {
				prefix = secp256k1.PubKeyFormatCompressedOdd
			}
			decoded[0] = prefix
			return NewAddressPubKeySchnorrSecp256k1Raw(0, decoded, params)
		}
	}

	str := fmt.Sprintf("address %q is not a supported type", addr)
	return nil, makeError(ErrUnsupportedAddress, str)
}

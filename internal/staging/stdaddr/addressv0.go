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

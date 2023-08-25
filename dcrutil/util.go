// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// AddressParams defines an interface that is used to provide the parameters
// required when encoding and decoding addresses.  These values are typically
// well-defined and unique per network.
type AddressParams interface {
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

// VerifyMessage verifies that signature is a valid signature of message and was created
// using the secp256k1 private key for address.
func VerifyMessage(address string, signature string, message string, params AddressParams) error {
	// Decode the provided address.  This also ensures the network encoded with
	// the address matches the network the server is currently on.
	addr, err := stdaddr.DecodeAddress(address, params)
	if err != nil {
		return err
	}

	// Only P2PKH addresses are valid for signing.
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		return fmt.Errorf("address is not a pay-to-pubkey-hash address")
	}

	// Decode base64 signature.
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("malformed base64 encoding: %w", err)
	}

	// Validate the signature - this just shows that it was valid for any pubkey
	// at all. Whether the pubkey matches is checked below.
	var buf bytes.Buffer
	wire.WriteVarString(&buf, 0, "Decred Signed Message:\n")
	wire.WriteVarString(&buf, 0, message)
	expectedMessageHash := chainhash.HashB(buf.Bytes())
	pk, wasCompressed, err := ecdsa.RecoverCompact(sig, expectedMessageHash)
	if err != nil {
		return err
	}

	// Reconstruct the address from the recovered pubkey.
	var pkHash []byte
	if wasCompressed {
		pkHash = stdaddr.Hash160(pk.SerializeCompressed())
	} else {
		pkHash = stdaddr.Hash160(pk.SerializeUncompressed())
	}
	recAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pkHash, params)
	if err != nil {
		return err
	}

	// Check whether addresses match.
	if recAddr.String() != addr.String() {
		return fmt.Errorf("message not signed by address")
	}

	return nil
}

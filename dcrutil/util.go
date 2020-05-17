// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v3/ecdsa"
	"github.com/decred/dcrd/wire"
)

// VerifyMessage verifies that signature is a valid signature of message and was created
// using the secp256k1 private key for address.
func VerifyMessage(address string, signature string, message string, params AddressParams) error {
	// Decode the provided address.  This also ensures the network encoded with
	// the address matches the network the server is currently on.
	addr, err := DecodeAddress(address, params)
	if err != nil {
		return err
	}

	// Only P2PKH addresses are valid for signing.
	if _, ok := addr.(*AddressPubKeyHash); !ok {
		return fmt.Errorf("address is not a pay-to-pubkey-hash address")
	}

	// Decode base64 signature.
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("malformed base64 encoding: %v", err)
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
	var serializedPK []byte
	if wasCompressed {
		serializedPK = pk.SerializeCompressed()
	} else {
		serializedPK = pk.SerializeUncompressed()
	}
	recoveredAddr, err := NewAddressSecpPubKey(serializedPK, params)
	if err != nil {
		return err
	}

	// Check whether addresses match.
	if recoveredAddr.Address() != addr.Address() {
		return fmt.Errorf("message not signed by address")
	}

	return nil
}

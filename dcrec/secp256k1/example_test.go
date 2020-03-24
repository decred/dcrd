// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1_test

import (
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// This example demonstrates encrypting a message for a public key that is first
// parsed from raw bytes, then decrypting it using the corresponding private key.
func Example_encryptMessage() {
	// Decode the hex-encoded pubkey of the recipient.
	pubKeyBytes, err := hex.DecodeString("04115c42e757b2efb7671c578530ec191a1" +
		"359381e6a71127a9d37c486fd30dae57e76dc58f693bd7e7010358ce6b165e483a29" +
		"21010db67ac11b1b51b651953d2") // uncompressed pubkey
	if err != nil {
		fmt.Println(err)
		return
	}
	pubKey, err := secp256k1.ParsePubKey(pubKeyBytes)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Encrypt a message decryptable by the private key corresponding to pubKey
	message := "test message"
	ciphertext, err := secp256k1.Encrypt(pubKey, []byte(message))
	if err != nil {
		fmt.Println(err)
		return
	}

	// Decode the hex-encoded private key.
	pkBytes, err := hex.DecodeString("a11b0a4e1a132305652ee7a8eb7848f6ad" +
		"5ea381e3ce20a2c086a2e388230811")
	if err != nil {
		fmt.Println(err)
		return
	}
	privKey := secp256k1.PrivKeyFromBytes(pkBytes)

	// Try decrypting and verify if it's the same message.
	plaintext, err := secp256k1.Decrypt(privKey, ciphertext)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(plaintext))

	// Output:
	// test message
}

// This example demonstrates decrypting a message using a private key that is
// first parsed from raw bytes.
func Example_decryptMessage() {
	// Decode the hex-encoded private key.
	pkBytes, err := hex.DecodeString("a11b0a4e1a132305652ee7a8eb7848f6ad" +
		"5ea381e3ce20a2c086a2e388230811")
	if err != nil {
		fmt.Println(err)
		return
	}
	privKey := secp256k1.PrivKeyFromBytes(pkBytes)

	ciphertext, err := hex.DecodeString("35f644fbfb208bc71e57684c3c8b437402ca" +
		"002047a2f1b38aa1a8f1d5121778378414f708fe13ebf7b4a7bb74407288c1958969" +
		"00207cf4ac6057406e40f79961c973309a892732ae7a74ee96cd89823913b8b8d650" +
		"a44166dc61ea1c419d47077b748a9c06b8d57af72deb2819d98a9d503efc59fc8307" +
		"d14174f8b83354fac3ff56075162")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Try decrypting the message.
	plaintext, err := secp256k1.Decrypt(privKey, ciphertext)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(plaintext))

	// Output:
	// test message
}

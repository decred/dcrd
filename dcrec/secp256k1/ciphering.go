// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	v2 "github.com/decred/dcrd/dcrec/secp256k1/v2"
)

var (
	// ErrInvalidMAC occurs when Message Authentication Check (MAC) fails
	// during decryption. This happens because of either invalid private key or
	// corrupt ciphertext.
	ErrInvalidMAC = v2.ErrInvalidMAC

	// 0x02CA = 714
	ciphCurveBytes = [2]byte{0x02, 0xCA}
	// 0x20 = 32
	ciphCoordLength = [2]byte{0x00, 0x20}
)

// GenerateSharedSecret generates a shared secret based on a private key and a
// public key using Diffie-Hellman key exchange (ECDH) (RFC 4753).
// RFC5903 Section 9 states we should only return x.
func GenerateSharedSecret(privkey *PrivateKey, pubkey *PublicKey) []byte {
	return v2.GenerateSharedSecret(privkey, pubkey)
}

// Encrypt encrypts data for the target public key using AES-256-CBC. It also
// generates a private key (the pubkey of which is also in the output). The only
// supported curve is secp256k1. The `structure' that it encodes everything into
// is:
//
//	struct {
//		// Initialization Vector used for AES-256-CBC
//		IV [16]byte
//		// Public Key: curve(2) + len_of_pubkeyX(2) + pubkeyX +
//		// len_of_pubkeyY(2) + pubkeyY (curve = 714)
//		PublicKey [70]byte
//		// Cipher text
//		Data []byte
//		// HMAC-SHA-256 Message Authentication Code
//		HMAC [32]byte
//	}
//
// The primary aim is to ensure byte compatibility with Pyelliptic.  Also, refer
// to section 5.8.1 of ANSI X9.63 for rationale on this format.
func Encrypt(pubkey *PublicKey, in []byte) ([]byte, error) {
	return v2.Encrypt(pubkey, in)
}

// Decrypt decrypts data that was encrypted using the Encrypt function.
func Decrypt(priv *PrivateKey, in []byte) ([]byte, error) {
	return v2.Decrypt(priv, in)
}

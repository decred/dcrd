// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utxoproof

import (
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
)

// Tags and schemes describing the message being signed.
//
// These strings must not contain the comma, which is reserved as a separator
// character.
const (
	tag = "mixpr-utxoproof"

	// schemes
	secp256k1P2PKH = "P2PKH(EC-Schnorr-DCRv0)"
)

const sep = ","

// The signature hash is created from the serialization of:
//   tag , scheme , expiry pubkey
// No separator is written after expiry; it is fixed length.

// Secp256k1KeyPair provides access to the serialized public key and parsed
// private key of a secp256k1 key pair.
type Secp256k1KeyPair struct {
	Pub  []byte
	Priv *secp256k1.PrivateKey
}

// SignUtxoProof returns the UTXO proof of ownership over an output controlled
// by the keypair.  The UTXO proof is only valid for the provided expiry
// height to prevent its inclusion in other PR messages signed by an unrelated
// identity.
func (k *Secp256k1KeyPair) SignUtxoProof(expires uint32) ([]byte, error) {
	const scheme = secp256k1P2PKH

	h := blake256.NewHasher256()
	const preamble = tag + sep + scheme + sep
	h.WriteBytes([]byte(preamble))
	h.WriteUint32BE(expires)
	h.WriteBytes(k.Pub)
	hash := h.Sum256()

	sig, err := schnorr.Sign(k.Priv, hash[:])
	if err != nil {
		return nil, err
	}

	return sig.Serialize(), nil
}

// ValidateSecp256k1P2PKH validates the UTXO proof of an output controlled by
// a secp256k1 keypair for the given expiry height.  Returns true only if the
// proof is valid.
func ValidateSecp256k1P2PKH(pubkey, proof []byte, expires uint32) bool {
	const scheme = secp256k1P2PKH

	pubkeyParsed, err := secp256k1.ParsePubKey(pubkey)
	if err != nil {
		return false
	}
	proofParsed, err := schnorr.ParseSignature(proof)
	if err != nil {
		return false
	}

	h := blake256.NewHasher256()
	const preamble = tag + sep + scheme + sep
	h.WriteBytes([]byte(preamble))
	h.WriteUint32BE(expires)
	h.Write(pubkey)
	hash := h.Sum256()

	return proofParsed.Verify(hash[:], pubkeyParsed)
}

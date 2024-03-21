// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"hash"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
)

type Signed interface {
	Pub() []byte
	Sig() []byte
	WriteSignedData(hash.Hash)
}

func SignMessage(m Signed, priv *secp256k1.PrivateKey) error {
	sig, err := sign(priv, m)
	if err != nil {
		return err
	}
	copy(m.Sig(), sig)
	return nil
}

func VerifyMessageSignature(m Signed) bool {
	return verify(m.Pub(), m, m.Sig())
}

func sign(priv *secp256k1.PrivateKey, m Signed) ([]byte, error) {
	h := blake256.New()
	m.WriteSignedData(h)
	sig, err := schnorr.Sign(priv, h.Sum(nil))
	if err != nil {
		return nil, err
	}
	return sig.Serialize(), nil
}

func verify(pk []byte, m Signed, sig []byte) bool {
	pkParsed, err := secp256k1.ParsePubKey(pk)
	if err != nil {
		return false
	}
	sigParsed, err := schnorr.ParseSignature(sig)
	if err != nil {
		return false
	}
	h := blake256.New()
	m.WriteSignedData(h)
	return sigParsed.Verify(h.Sum(nil), pkParsed)
}

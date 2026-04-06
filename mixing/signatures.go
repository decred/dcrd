// Copyright (c) 2023-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"encoding/hex"
	"hash"
	"strconv"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
)

const tag = "decred-mix-signature"

// Signed is an interface describing a signed mixing message.
type Signed interface {
	Pub() []byte
	Sig() []byte
	Sid() []byte
	GetRun() uint32
	Command() string
	WriteSignedData(hash.Hash)
}

// SignMessage creates a signature for the message m and writes the signature
// into the message.
func SignMessage(m Signed, priv *secp256k1.PrivateKey) error {
	sig, err := sign(priv, m)
	if err != nil {
		return err
	}
	// XXX: A SetSig method or similar would be less janky.
	copy(m.Sig(), sig)
	return nil
}

// VerifySignedMessage verifies that a signed message carries a valid
// signature for the represented identity.
func VerifySignedMessage(m Signed) bool {
	h := blake256.NewHasher256()
	m.WriteSignedData(h)
	sigHash := h.Sum256()

	command := m.Command()
	sid := m.Sid()
	run := m.GetRun()
	if len(sid) != 32 {
		sid = zeroSID[:]
		run = 0
	}

	h.Reset()
	return verify(h, m.Pub(), m.Sig(), sigHash[:], command, sid, run)
}

// VerifySignature verifies a message signature from its signature hash and
// information describing the message type and its place in the protocol.
// Multiple messages of the same command, sid, and run should not be signed by
// the same public key, and demonstrating this can be used to prove malicious
// behavior by sending different versions of messages through the network.
func VerifySignature(pub, sig, sigHash []byte, command string, sid []byte, run uint32) bool {
	h := blake256.NewHasher256()
	return verify(h, pub, sig, sigHash, command, sid, run)
}

var zeroSID [32]byte

func sign(priv *secp256k1.PrivateKey, m Signed) ([]byte, error) {
	h := blake256.NewHasher256()
	m.WriteSignedData(h)
	sigHash := h.Sum256()

	sid := m.Sid()
	run := m.GetRun()
	if len(sid) != 32 {
		sid = zeroSID[:]
		run = 0
	}

	h.Reset()
	hash := schnorrHash(h, m.Command(), sid, run, sigHash[:])
	sig, err := schnorr.Sign(priv, hash[:])
	if err != nil {
		return nil, err
	}
	return sig.Serialize(), nil
}

func verify(h *blake256.Hasher256, pk []byte, sig []byte, sigHash []byte, command string, sid []byte, run uint32) bool {
	if len(pk) != secp256k1.PubKeyBytesLenCompressed {
		return false
	}
	pkParsed, err := secp256k1.ParsePubKey(pk)
	if err != nil {
		return false
	}
	sigParsed, err := schnorr.ParseSignature(sig)
	if err != nil {
		return false
	}

	hash := schnorrHash(h, command, sid, run, sigHash)
	return sigParsed.Verify(hash[:], pkParsed)
}

func schnorrHash(h *blake256.Hasher256, command string, sid []byte, run uint32, sigHash []byte) [32]byte {
	buf := make([]byte, 64)

	h.WriteBytes([]byte(tag))
	h.WriteByte(',')
	h.WriteString(command)
	h.WriteByte(',')
	h.WriteBytes(hex.AppendEncode(buf[:0], sid))
	h.WriteByte(',')
	h.WriteBytes(strconv.AppendUint(buf[:0], uint64(run), 10))
	h.WriteByte(',')
	h.WriteBytes(hex.AppendEncode(buf[:0], sigHash))
	return h.Sum256()
}

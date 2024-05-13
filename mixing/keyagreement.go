// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/companyzero/sntrup4591761"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/mixing/internal/chacha20prng"
	"github.com/decred/dcrd/wire"
)

// Aliases for sntrup4591761 types
type (
	PQPublicKey  = [sntrup4591761.PublicKeySize]byte
	PQPrivateKey = [sntrup4591761.PrivateKeySize]byte
	PQCiphertext = [sntrup4591761.CiphertextSize]byte
	PQSharedKey  = [sntrup4591761.SharedKeySize]byte
)

func generateSecp256k1(rand io.Reader) (*secp256k1.PublicKey, *secp256k1.PrivateKey, error) {
	if rand == nil {
		rand = cryptorand.Reader
	}

	privateKey, err := secp256k1.GeneratePrivateKeyFromRand(rand)
	if err != nil {
		return nil, nil, err
	}

	publicKey := privateKey.PubKey()

	return publicKey, privateKey, nil
}

// KX contains the client public and private keys to perform shared key exchange
// with other peers.
type KX struct {
	ECDHPublicKey  *secp256k1.PublicKey
	ECDHPrivateKey *secp256k1.PrivateKey
	PQPublicKey    *PQPublicKey
	PQPrivateKey   *PQPrivateKey
	PQCleartexts   []PQSharedKey
}

// NewKX generates a mixing identity's public and private keys for a interactive
// key exchange, with randomness read from a run's CSPRNG.
func NewKX(csprng io.Reader) (*KX, error) {
	ecdhPublic, ecdhPrivate, err := generateSecp256k1(csprng)
	if err != nil {
		return nil, err
	}

	pqPublic, pqPrivate, err := sntrup4591761.GenerateKey(csprng)
	if err != nil {
		return nil, err
	}

	kx := &KX{
		ECDHPublicKey:  ecdhPublic,
		ECDHPrivateKey: ecdhPrivate,
		PQPublicKey:    pqPublic,
		PQPrivateKey:   pqPrivate,
	}
	return kx, nil
}

func (kx *KX) ecdhSharedKey(pub *secp256k1.PublicKey) []byte {
	secret := secp256k1.GenerateSharedSecret(kx.ECDHPrivateKey, pub)
	hash := blake256.Sum256(secret)
	return hash[:]
}

func (kx *KX) pqSharedKey(ciphertext *PQCiphertext) ([]byte, error) {
	pqSharedKey, ok := sntrup4591761.Decapsulate(ciphertext, kx.PQPrivateKey)
	if ok != 1 {
		return nil, errors.New("sntrup4591761: decapsulate failure")
	}
	return pqSharedKey[:], nil
}

// Encapsulate performs encapsulation for sntrup4591761 key exchanges with each
// other peer in the DC-net.  It populates the PQCleartexts field of kx and
// returns encrypted cyphertexts of these shared keys.
//
// Encapsulation in the DC-net requires randomness from a CSPRNG seeded by a
// committed secret; blame assignment is not possible otherwise.
func (kx *KX) Encapsulate(prng io.Reader, pubkeys []*PQPublicKey, my int) ([]PQCiphertext, error) {
	cts := make([][sntrup4591761.CiphertextSize]byte, len(pubkeys))
	kx.PQCleartexts = make([][32]byte, len(pubkeys))

	for i, pk := range pubkeys {
		ciphertext, cleartext, err := sntrup4591761.Encapsulate(prng, pk)
		if err != nil {
			return nil, err
		}
		cts[i] = *ciphertext
		kx.PQCleartexts[i] = *cleartext
	}

	return cts, nil
}

// RevealedKeys records the revealed ECDH public keys of every peer and the
// ciphertexts created for a single peer at MyIndex.
type RevealedKeys struct {
	ECDHPublicKeys []*secp256k1.PublicKey
	Ciphertexts    []PQCiphertext
	MyIndex        uint32
}

// SharedSecrets is a return value for the KX.SharedSecrets method, housing
// the slot reservation and XOR DC-Net shared secrets between two peers.
type SharedSecrets struct {
	SRSecrets [][][]byte
	DCSecrets [][]wire.MixVect
}

// SharedKeys creates the pairwise SR and DC shared secret keys for
// mcounts[k.MyIndex] mixes.  ecdhPubs, cts, and mcounts must all share the same
// slice length.
func (kx *KX) SharedSecrets(k *RevealedKeys, sid []byte, run uint32, mcounts []uint32) (SharedSecrets, error) {
	var s SharedSecrets

	if len(k.ECDHPublicKeys) != len(mcounts) {
		err := fmt.Errorf("ECDH public key count (%d) must match peer count (%d)",
			len(k.ECDHPublicKeys), len(mcounts))
		return s, err
	}
	if len(k.Ciphertexts) != len(mcounts) {
		err := fmt.Errorf("ciphertext count (%d) must match peer count (%d)",
			len(k.Ciphertexts), len(mcounts))
		return s, err
	}

	mcount := mcounts[k.MyIndex]
	var mtot uint32
	for i := range mcounts {
		mtot += mcounts[i]
	}

	s.SRSecrets = make([][][]byte, mcount)
	s.DCSecrets = make([][]wire.MixVect, mcount)

	for i := uint32(0); i < mcount; i++ {
		s.SRSecrets[i] = make([][]byte, mtot)
		s.DCSecrets[i] = make([]wire.MixVect, mtot)
		var m int
		for peer := uint32(0); int(peer) < len(mcounts); peer++ {
			if peer == k.MyIndex && mcount == 1 {
				m++
				continue
			}

			sharedKey := kx.ecdhSharedKey(k.ECDHPublicKeys[peer])
			pqSharedKey, err := kx.pqSharedKey(&k.Ciphertexts[peer])
			if err != nil {
				err := &DecapsulateError{
					SubmittingIndex: peer,
				}
				return s, err
			}

			// XOR ECDH and both sntrup4591761 keys into a single
			// shared key. If sntrup4591761 is discovered to be
			// broken in the future, the security only reduces to
			// that of x25519.
			// If the message belongs to our own peer, only XOR
			// the sntrup4591761 key once.  The decapsulated and
			// cleartext keys are equal in this case, and would
			// cancel each other out otherwise.
			xor := func(dst, src []byte) {
				if len(dst) != len(src) {
					panic("dcnet: different lengths in xor")
				}
				for i := range dst {
					dst[i] ^= src[i]
				}
			}
			xor(sharedKey, pqSharedKey[:])
			if peer != k.MyIndex {
				xor(sharedKey, kx.PQCleartexts[peer][:])
			}

			// Create the prefix of a PRNG seed preimage.  A counter
			// will be appended before creating each PRNG, one for
			// each message pair.
			prngSeedPreimage := make([]byte, len(sid)+len(sharedKey)+4)
			l := copy(prngSeedPreimage, sid)
			l += copy(prngSeedPreimage[l:], sharedKey)
			seedCounterBytes := prngSeedPreimage[l:]

			// Read from the PRNG to create shared keys for each
			// message the peer is mixing.
			for j := uint32(0); j < mcounts[peer]; j++ {
				if k.MyIndex == peer && j == i {
					m++
					continue
				}

				// Create the PRNG seed using the combined shared key.
				// A unique seed is generated for each message pair,
				// determined using the message index of the peer with
				// the lower peer index.  The PRNG nonce is the message
				// number of the peer with the higher peer index.
				// When creating shared keys with our own peer, the PRNG
				// seed counter and nonce must be reversed for the second
				// half of our generated keys.
				seedCounter := i
				nonce := j
				if k.MyIndex > peer || (k.MyIndex == peer && j > i) {
					seedCounter = j
					nonce = i
				}
				binary.LittleEndian.PutUint32(seedCounterBytes, seedCounter)

				prngSeed := blake256.Sum256(prngSeedPreimage)
				prng := chacha20prng.New(prngSeed[:], nonce)

				s.SRSecrets[i][m] = prng.Next(32)
				s.DCSecrets[i][m] = wire.MixVect(randVec(mtot, prng))

				m++
			}
		}
	}

	return s, nil
}

// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sign

import (
	"crypto/rand"
	"errors"
	"fmt"
	mrand "math/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/edwards/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// testingParams defines the chain params to use throughout these tests so it
// can more easily be changed if desired.
var testingParams = chaincfg.RegNetParams()

const (
	testValueIn = 12345

	// noTreasury signifies the treasury agenda should be treated as though
	// it is inactive.  It is used to increase the readability of the
	// tests.
	noTreasury = false

	// withTreasury signifies the treasury agenda should be treated as
	// though it is active.  It is used to increase the readability of
	// the tests.
	withTreasury = true
)

type addressToKey struct {
	key        []byte
	sigType    dcrec.SignatureType
	compressed bool
}

func mkGetKey(keys map[string]addressToKey) KeyDB {
	if keys == nil {
		return KeyClosure(func(addr stdaddr.Address) ([]byte,
			dcrec.SignatureType, bool, error) {
			return nil, 0, false, errors.New("nope 1")
		})
	}
	return KeyClosure(func(addr stdaddr.Address) ([]byte,
		dcrec.SignatureType, bool, error) {
		a2k, ok := keys[addr.String()]
		if !ok {
			return nil, 0, false, errors.New("nope 2")
		}
		return a2k.key, a2k.sigType, a2k.compressed, nil
	})
}

func mkGetKeyPub(keys map[string]addressToKey) KeyDB {
	if keys == nil {
		return KeyClosure(func(addr stdaddr.Address) ([]byte,
			dcrec.SignatureType, bool, error) {
			return nil, 0, false, errors.New("nope 1")
		})
	}
	return KeyClosure(func(addr stdaddr.Address) ([]byte,
		dcrec.SignatureType, bool, error) {
		a2k, ok := keys[addr.String()]
		if !ok {
			return nil, 0, false, errors.New("nope 2")
		}
		return a2k.key, a2k.sigType, a2k.compressed, nil
	})
}

func mkGetScript(scripts map[string][]byte) ScriptDB {
	if scripts == nil {
		return ScriptClosure(func(addr stdaddr.Address) (
			[]byte, error) {
			return nil, errors.New("nope 3")
		})
	}
	return ScriptClosure(func(addr stdaddr.Address) ([]byte,
		error) {
		script, ok := scripts[addr.String()]
		if !ok {
			return nil, errors.New("nope 4")
		}
		return script, nil
	})
}

func checkScripts(msg string, tx *wire.MsgTx, idx int, sigScript, pkScript []byte) error {
	tx.TxIn[idx].SignatureScript = sigScript
	var scriptFlags txscript.ScriptFlags
	vm, err := txscript.NewEngine(pkScript, tx, idx, scriptFlags, 0, nil)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = vm.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

func signAndCheck(msg string, tx *wire.MsgTx, idx int, pkScript []byte,
	hashType txscript.SigHashType, kdb KeyDB, sdb ScriptDB,
	isTreasuryEnabled bool) error {

	sigScript, err := SignTxOutput(testingParams, tx, idx, pkScript,
		hashType, kdb, sdb, nil, isTreasuryEnabled)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	return checkScripts(msg, tx, idx, sigScript, pkScript)
}

func signBadAndCheck(msg string, tx *wire.MsgTx, idx int, pkScript []byte,
	hashType txscript.SigHashType, kdb KeyDB, sdb ScriptDB,
	isTreasuryEnabled bool) error {

	// Setup a PRNG.
	randScriptHash := chainhash.HashB(pkScript)
	tRand := mrand.New(mrand.NewSource(int64(randScriptHash[0])))

	sigScript, err := SignTxOutput(testingParams, tx,
		idx, pkScript, hashType, kdb, sdb, nil, isTreasuryEnabled)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	// Be sure to reset the value in when we're done creating the
	// corrupted signature for that flag.
	tx.TxIn[0].ValueIn = testValueIn

	// Corrupt a random bit in the signature.
	pos := tRand.Intn(len(sigScript) - 1)
	bitPos := tRand.Intn(7)
	sigScript[pos] ^= 1 << uint8(bitPos)

	return checkScripts(msg, tx, idx, sigScript, pkScript)
}

func TestSignTxOutput(t *testing.T) {
	t.Parallel()

	// make key
	// make script based on key.
	// sign with magic pixie dust.
	hashTypes := []txscript.SigHashType{
		txscript.SigHashAll,
		txscript.SigHashNone,
		txscript.SigHashSingle,
		txscript.SigHashAll | txscript.SigHashAnyOneCanPay,
		txscript.SigHashNone | txscript.SigHashAnyOneCanPay,
		txscript.SigHashSingle | txscript.SigHashAnyOneCanPay,
	}
	signatureSuites := []dcrec.SignatureType{
		dcrec.STEcdsaSecp256k1,
		dcrec.STEd25519,
		dcrec.STSchnorrSecp256k1,
	}
	tx := &wire.MsgTx{
		SerType: wire.TxSerializeFull,
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0,
					Tree:  0,
				},
				Sequence:    4294967295,
				ValueIn:     testValueIn,
				BlockHeight: 78901,
				BlockIndex:  23456,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 1,
					Tree:  0,
				},
				Sequence:    4294967295,
				ValueIn:     testValueIn,
				BlockHeight: 78901,
				BlockIndex:  23456,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 2,
					Tree:  0,
				},
				Sequence:    4294967295,
				ValueIn:     testValueIn,
				BlockHeight: 78901,
				BlockIndex:  23456,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Version: wire.DefaultPkScriptVersion,
				Value:   1,
			},
			{
				Version: wire.DefaultPkScriptVersion,
				Value:   2,
			},
			{
				Version: wire.DefaultPkScriptVersion,
				Value:   3,
			},
		},
		LockTime: 0,
		Expiry:   0,
	}

	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) (merging with correct)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				sigScript, err := SignTxOutput(
					testingParams, tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(
					testingParams, tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), sigScript, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				sigScript, err = SignTxOutput(
					testingParams, tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(
					testingParams, tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), sigScript, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), sigScript, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript,
					hashType, mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), sigScript, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash for a ticket(SStx) (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB := privKey.Serialize()
			pkBytes := privKey.PubKey().SerializeCompressed()

			address, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				stdaddr.Hash160(pkBytes), testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
			}

			_, pkScript := address.VotingRightsScript()

			// Without treasury agenda.
			suite := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Pay to Pubkey Hash for a ticket change (SStx change) (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB := privKey.Serialize()
			pkBytes := privKey.PubKey().SerializeCompressed()

			address, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				stdaddr.Hash160(pkBytes), testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			_, pkScript := address.StakeChangeScript()

			// Without treasury agenda.
			suite := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Pay to Pubkey Hash for a ticket spending (SSGen) (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB := privKey.Serialize()
			pkBytes := privKey.PubKey().SerializeCompressed()

			address, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				stdaddr.Hash160(pkBytes), testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			_, pkScript := address.PayVoteCommitmentScript()

			// Without treasury agenda.
			suite := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Pay to Pubkey Hash for a ticket revocation (SSRtx) (compressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB := privKey.Serialize()
			pkBytes := privKey.PubKey().SerializeCompressed()

			address, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				stdaddr.Hash160(pkBytes), testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v",
					msg, err)
				break
			}

			_, pkScript := address.PayRevokeCommitmentScript()

			// Without treasury agenda.
			suite := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, true},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB := privKey.Serialize()
			pk := privKey.PubKey()
			address, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			_, pkScript := address.PaymentScript()

			// Without treasury agenda.
			suite := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, false},
				}), mkGetScript(nil), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, dcrec.STEcdsaSecp256k1, false},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, suite, false},
				}), mkGetScript(nil), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address.String(): {keyDB, dcrec.STEcdsaSecp256k1, false},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), sigScript, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
				}

				// With treasury agenda.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), sigScript, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
				}
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKeyPub(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to PubKey (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
					if err != nil {
						t.Errorf("failed to make address for %s: %v", msg, err)
					}
				}

				_, pkScript := address.PaymentScript()

				// Without treasury agenda.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), sigScript, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), sigScript, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, pkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// As before, but with p2sh now.
	// Pay to Pubkey Hash (uncompressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				_, err = SignTxOutput(testingParams, tx, i,
					scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// Wit treasury agenda.
				_, err = SignTxOutput(testingParams, tx, i,
					scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (compressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to Pubkey Hash (compressed) with duplicate merge
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
						h160, testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashEd25519V0(h160,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pkBytes := privKey.PubKey().SerializeCompressed()
					h160 := stdaddr.Hash160(pkBytes)
					address, err = stdaddr.NewAddressPubKeyHashSchnorrSecp256k1V0(
						h160, testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Pay to PubKey (uncompressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), noTreasury); err != nil {
					t.Error(err)
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), withTreasury); err != nil {
					t.Error(err)
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to PubKey (uncompressed) with duplicate merge
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeUncompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), noTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), noTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}

				// With treasury agenda.
				if err := signAndCheck(msg, tx, i, scriptPkScript,
					hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), withTreasury); err != nil {
					t.Error(err)
					break
				}

				if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, false},
					}), mkGetScript(nil), withTreasury); err == nil {
					t.Errorf("corrupted signature validated: %s", msg)
					break
				}
			}
		}
	}

	// Pay to PubKey (compressed)
	for _, hashType := range hashTypes {
		for _, suite := range signatureSuites {
			for i := range tx.TxIn {
				var keyDB []byte

				msg := fmt.Sprintf("%d:%d:%d", hashType, i, suite)
				var address stdaddr.Address
				var err error
				switch suite {
				case dcrec.STEcdsaSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk,
						testingParams)

				case dcrec.STEd25519:
					keyDB, _, _, _ = edwards.GenerateKey(rand.Reader)
					_, pk := edwards.PrivKeyFromBytes(keyDB)
					pkBytes := pk.SerializeCompressed()
					address, err = stdaddr.NewAddressPubKeyEd25519V0Raw(pkBytes,
						testingParams)

				case dcrec.STSchnorrSecp256k1:
					privKey, _ := secp256k1.GeneratePrivateKey()
					keyDB = privKey.Serialize()
					pk := privKey.PubKey()
					address, err = stdaddr.NewAddressPubKeySchnorrSecp256k1V0(pk,
						testingParams)
				}
				if err != nil {
					t.Errorf("failed to make address for %s: %v", msg, err)
					break
				}

				_, pkScript := address.PaymentScript()

				scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
					testingParams)
				if err != nil {
					t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
					break
				}

				_, scriptPkScript := scriptAddr.PaymentScript()

				// Without treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err := SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, noTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}

				// With treasury agenda.
				_, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s: %v", msg,
						err)
					break
				}

				// by the above loop, this should be valid, now sign
				// again and merge.
				sigScript, err = SignTxOutput(testingParams,
					tx, i, scriptPkScript, hashType,
					mkGetKey(map[string]addressToKey{
						address.String(): {keyDB, suite, true},
					}), mkGetScript(map[string][]byte{
						scriptAddr.String(): pkScript,
					}), nil, withTreasury)
				if err != nil {
					t.Errorf("failed to sign output %s a "+
						"second time: %v", msg, err)
					break
				}

				err = checkScripts(msg, tx, i, sigScript, scriptPkScript)
				if err != nil {
					t.Errorf("twice signed script invalid for "+
						"%s: %v", msg, err)
					break
				}
			}
		}
	}

	// Basic Multisig
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey1, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB1 := privKey1.Serialize()
			pk1 := privKey1.PubKey()

			address1, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk1,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			privKey2, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB2 := privKey2.Serialize()
			pk2 := privKey2.PubKey()

			address2, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk2,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v", msg, err)
				break
			}

			pkScript, err := txscript.MultiSigScript(2,
				pk1.SerializeCompressed(), pk2.SerializeCompressed())
			if err != nil {
				t.Errorf("failed to make pkscript for %s: %v", msg, err)
			}

			scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
				testingParams)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
				break
			}

			_, scriptPkScript := scriptAddr.PaymentScript()

			// Without treasury agenda.
			suite1 := dcrec.STEcdsaSecp256k1
			suite2 := dcrec.STEcdsaSecp256k1
			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), noTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(nil), noTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}

			// With treasury agenda.
			if err := signAndCheck(msg, tx, i, scriptPkScript,
				hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), withTreasury); err != nil {
				t.Error(err)
				break
			}

			if err := signBadAndCheck(msg, tx, i, pkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(nil), withTreasury); err == nil {
				t.Errorf("corrupted signature validated: %s", msg)
				break
			}
		}
	}

	// Two part multisig, sign with one key then the other.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey1, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB1 := privKey1.Serialize()
			pk1 := privKey1.PubKey()

			address1, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk1,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			privKey2, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB2 := privKey2.Serialize()
			pk2 := privKey2.PubKey()

			address2, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk2,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v", msg, err)
				break
			}

			pkScript, err := txscript.MultiSigScript(2,
				pk1.SerializeCompressed(), pk2.SerializeCompressed())
			if err != nil {
				t.Errorf("failed to make pkscript "+
					"for %s: %v", msg, err)
			}

			scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
				testingParams)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
				break
			}

			_, scriptPkScript := scriptAddr.PaymentScript()

			// Without treasury agenda.
			suite1 := dcrec.STEcdsaSecp256k1
			suite2 := dcrec.STEcdsaSecp256k1
			sigScript, err := SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), nil, noTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), sigScript, noTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}

			// With treasury agenda.
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), nil, withTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), sigScript, withTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}

	// Two part multisig, sign with one key then both, check key dedup
	// correctly.
	for _, hashType := range hashTypes {
		for i := range tx.TxIn {
			msg := fmt.Sprintf("%d:%d", hashType, i)

			privKey1, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB1 := privKey1.Serialize()
			pk1 := privKey1.PubKey()

			address1, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk1,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address for %s: %v", msg, err)
				break
			}

			privKey2, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				t.Errorf("failed to generate key: %v", err)
				break
			}
			keyDB2 := privKey2.Serialize()
			pk2 := privKey2.PubKey()
			address2, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0(pk2,
				testingParams)
			if err != nil {
				t.Errorf("failed to make address 2 for %s: %v", msg, err)
				break
			}

			pkScript, err := txscript.MultiSigScript(2,
				pk1.SerializeCompressed(), pk2.SerializeCompressed())
			if err != nil {
				t.Errorf("failed to make pkscript for %s: %v", msg, err)
			}

			scriptAddr, err := stdaddr.NewAddressScriptHashV0(pkScript,
				testingParams)
			if err != nil {
				t.Errorf("failed to make p2sh addr for %s: %v", msg, err)
				break
			}

			_, scriptPkScript := scriptAddr.PaymentScript()
			if err != nil {
				t.Errorf("failed to make script pkscript for %s: %v", msg, err)
				break
			}

			// Without treasury agenda.
			suite1 := dcrec.STEcdsaSecp256k1
			suite2 := dcrec.STEcdsaSecp256k1
			sigScript, err := SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), nil, noTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), sigScript, noTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}

			// With treasury agenda.
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), nil, withTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg,
					err)
				break
			}

			// Only 1 out of 2 signed, this *should* fail.
			if checkScripts(msg, tx, i, sigScript,
				scriptPkScript) == nil {
				t.Errorf("part signed script valid for %s", msg)
				break
			}

			// Sign with the other key and merge
			sigScript, err = SignTxOutput(testingParams, tx, i,
				scriptPkScript, hashType,
				mkGetKey(map[string]addressToKey{
					address1.String(): {keyDB1, suite1, true},
					address2.String(): {keyDB2, suite2, true},
				}), mkGetScript(map[string][]byte{
					scriptAddr.String(): pkScript,
				}), sigScript, withTreasury)
			if err != nil {
				t.Errorf("failed to sign output %s: %v", msg, err)
				break
			}

			// Now we should pass.
			err = checkScripts(msg, tx, i, sigScript,
				scriptPkScript)
			if err != nil {
				t.Errorf("fully signed script invalid for "+
					"%s: %v", msg, err)
				break
			}
		}
	}
}

type tstInput struct {
	txout              *wire.TxOut
	sigscriptGenerates bool
	inputValidates     bool
	indexOutOfRange    bool
}

type tstSigScript struct {
	name               string
	inputs             []tstInput
	hashType           txscript.SigHashType
	compress           bool
	scriptAtWrongIndex bool
}

var coinbaseOutPoint = &wire.OutPoint{
	Index: (1 << 32) - 1,
}

// Pregenerated private key, with associated public key and pkScripts
// for the uncompressed and compressed hash160.
var (
	privKeyD = []byte{0x6b, 0x0f, 0xd8, 0xda, 0x54, 0x22, 0xd0, 0xb7,
		0xb4, 0xfc, 0x4e, 0x55, 0xd4, 0x88, 0x42, 0xb3, 0xa1, 0x65,
		0xac, 0x70, 0x7f, 0x3d, 0xa4, 0x39, 0x5e, 0xcb, 0x3b, 0xb0,
		0xd6, 0x0e, 0x06, 0x92}
	thisPubKey        = secp256k1.PrivKeyFromBytes(privKeyD).PubKey()
	thisAddressUnc, _ = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		stdaddr.Hash160(thisPubKey.SerializeUncompressed()),
		testingParams)
	_, uncompressedPkScript = thisAddressUnc.PaymentScript()
	thisAddressCom, _       = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		stdaddr.Hash160(thisPubKey.SerializeCompressed()),
		testingParams)
	_, compressedPkScript = thisAddressCom.PaymentScript()
	shortPkScript         = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x88, 0xac}
)

// Pretend output amounts.
const coinbaseVal = 2500000000
const fee = 5000000

var sigScriptTests = []tstSigScript{
	{
		name: "one input uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "one input compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashNone",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashNone,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashSingle",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashSingle,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashAnyoneCanPay",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAnyOneCanPay | txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType non-standard",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     false,
				indexOutOfRange:    false,
			},
		},
		hashType:           0x04,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "invalid compression",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     false,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "short PkScript",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, shortPkScript),
				sigscriptGenerates: false,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "valid script at wrong index",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
	{
		name: "index out of range",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
}

// Test the sigscript generation for valid and invalid inputs, all
// hashTypes, and with and without compression.  This test creates
// sigscripts to spend fake coinbase inputs, as sigscripts cannot be
// created for the MsgTxs in txTests, since they come from the blockchain
// and we don't have the private keys.
func TestSignatureScript(t *testing.T) {
	t.Parallel()

nexttest:
	for i := range sigScriptTests {
		tx := wire.NewMsgTx()

		output := wire.NewTxOut(500, []byte{txscript.OP_RETURN})
		tx.AddTxOut(output)

		for range sigScriptTests[i].inputs {
			txin := wire.NewTxIn(coinbaseOutPoint, 500, nil)
			tx.AddTxIn(txin)
		}

		var script []byte
		var err error
		for j := range tx.TxIn {
			var idx int
			if sigScriptTests[i].inputs[j].indexOutOfRange {
				t.Errorf("at test %v", sigScriptTests[i].name)
				idx = len(sigScriptTests[i].inputs)
			} else {
				idx = j
			}
			script, err = SignatureScript(tx, idx,
				sigScriptTests[i].inputs[j].txout.PkScript,
				sigScriptTests[i].hashType, privKeyD, dcrec.STEcdsaSecp256k1,
				sigScriptTests[i].compress)

			if (err == nil) != sigScriptTests[i].inputs[j].sigscriptGenerates {
				if err == nil {
					t.Errorf("passed test '%v' incorrectly",
						sigScriptTests[i].name)
				} else {
					t.Errorf("failed test '%v': %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
			if !sigScriptTests[i].inputs[j].sigscriptGenerates {
				// done with this test
				continue nexttest
			}

			tx.TxIn[j].SignatureScript = script
		}

		// If testing using a correct sigscript but for an incorrect
		// index, use last input script for first input.  Requires > 0
		// inputs for test.
		if sigScriptTests[i].scriptAtWrongIndex {
			tx.TxIn[0].SignatureScript = script
			sigScriptTests[i].inputs[0].inputValidates = false
		}

		// Validate tx input scripts
		var scriptFlags txscript.ScriptFlags
		for j := range tx.TxIn {
			vm, err := txscript.NewEngine(sigScriptTests[i].inputs[j].txout.
				PkScript, tx, j, scriptFlags, 0,
				nil)
			if err != nil {
				t.Errorf("cannot create script vm for test %v: %v",
					sigScriptTests[i].name, err)
				continue nexttest
			}
			err = vm.Execute()
			if (err == nil) != sigScriptTests[i].inputs[j].inputValidates {
				if err == nil {
					t.Errorf("passed test '%v' validation incorrectly: %v",
						sigScriptTests[i].name, err)
				} else {
					t.Errorf("failed test '%v' validation: %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
		}
	}
}

// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
)

var (
	hash160 = dcrutil.Hash160([]byte("test"))
)

func TestIsRevocationScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "revocation-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "revocation-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "revocation-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsRevocationScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s: expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsTicketPurchaseScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "ticket purchase-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "ticket purchase-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "ticket purchase-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsTicketPurchaseScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsVoteScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "vote-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsVoteScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsStakeChangeScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "stake change-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "stake change-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "stake change-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "stake change-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsStakeChangeScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsNullDataScript(t *testing.T) {
	t.Parallel()

	var overMaxDataCarrierSize = make([]byte, txscript.MaxDataCarrierSize+1)
	var underMaxDataCarrierSize = make([]byte, txscript.MaxDataCarrierSize/2)
	_, err := rand.Read(overMaxDataCarrierSize)
	if err != nil {
		t.Fatalf("[Read]: unexpected error: %v", err)
	}
	_, err = rand.Read(underMaxDataCarrierSize)
	if err != nil {
		t.Fatalf("[Read]: unexpected error: %v", err)
	}

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "OP_RETURN script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN),
			version:  0,
			expected: true,
		},
		{
			name: "OP_RETURN script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN),
			version:  100,
			expected: false,
		},
		{
			name: "OP_RETURN script with data under MaxDataCarrierSize",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN).AddData(underMaxDataCarrierSize),
			version:  0,
			expected: true,
		},
		{
			name: "OP_RETURN script with data over MaxDataCarrierSize",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN).AddData(overMaxDataCarrierSize),
			version:  0,
			expected: false,
		},
		{
			name: "revocation-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsNullDataScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s: expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsMultisigSigScript(t *testing.T) {
	t.Parallel()

	var pubKeys [][]byte
	for i := 0; i < 2; i++ {
		pk := secp256k1.NewPrivateKey(big.NewInt(0))
		pubKeys = append(pubKeys, (*secp256k1.PublicKey)(&pk.PublicKey).SerializeCompressed())
	}

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "multisig script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_1).AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			version:  0,
			expected: true,
		},
		{
			name: "multisig script with invalid version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_1).AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			version:  100,
			expected: false,
		},
		{
			name: "p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_DUP).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUALVERIFY).
				AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		multiSig, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected multi sig script generation error: %s",
				test.name, err)
		}

		sigHex := dcrutil.Hash160(multiSig)
		script, err := txscript.NewScriptBuilder().AddData(sigHex).
			AddData(multiSig).Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsMultisigSigScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s: expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

var mainNetParams = chaincfg.MainNetParams()

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// newAddressPubKey returns a new dcrutil.AddressPubKey from the provided
// serialized public key.  It panics if an error occurs.  This is only used in
// the tests as a helper since the only way it can fail is if there is an error
// in the test source code.
func newAddressPubKey(serializedPubKey []byte) dcrutil.Address {
	pubkey, err := secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		panic("invalid public key in test source")
	}
	addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pubkey, mainNetParams)
	if err != nil {
		panic("invalid public key in test source")
	}

	return addr
}

// newAddressPubKeyHash returns a new dcrutil.AddressPubKeyHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressPubKeyHash(pkHash []byte) dcrutil.Address {
	addr, err := dcrutil.NewAddressPubKeyHash(pkHash, mainNetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		panic("invalid public key hash in test source")
	}

	return addr
}

// newAddressScriptHash returns a new dcrutil.AddressScriptHash from the
// provided hash.  It panics if an error occurs.  This is only used in the tests
// as a helper since the only way it can fail is if there is an error in the
// test source code.
func newAddressScriptHash(scriptHash []byte) dcrutil.Address {
	addr, err := dcrutil.NewAddressScriptHashFromHash(scriptHash, mainNetParams)
	if err != nil {
		panic("invalid script hash in test source")
	}

	return addr
}

func TestExtractPkScriptAddrs(t *testing.T) {
	t.Parallel()

	const scriptVersion = 0
	tests := []struct {
		name    string
		script  []byte
		addrs   []dcrutil.Address
		reqSigs int
		class   txscript.ScriptClass
		noparse bool
	}{
		{
			name: "standard p2pk with compressed pubkey (0x02)",
			script: hexToBytes("2102192d74d0cb94344c9569c2e779015" +
				"73d8d7903c3ebec3a957724895dca52c6b4ac"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("02192d74d0cb9434" +
					"4c9569c2e77901573d8d7903c3ebec3a9577" +
					"24895dca52c6b4")),
			},
			reqSigs: 1,
			class:   txscript.PubKeyTy,
		},
		{
			name: "standard p2pk with uncompressed pubkey (0x04)",
			script: hexToBytes("410411db93e1dcdb8a016b49840f8c53b" +
				"c1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddf" +
				"b84ccf9744464f82e160bfa9b8b64f9d4c03f999b864" +
				"3f656b412a3ac"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("0411db93e1dcdb8a" +
					"016b49840f8c53bc1eb68a382e97b1482eca" +
					"d7b148a6909a5cb2e0eaddfb84ccf9744464" +
					"f82e160bfa9b8b64f9d4c03f999b8643f656" +
					"b412a3")),
			},
			reqSigs: 1,
			class:   txscript.PubKeyTy,
		},
		{
			name: "standard p2pk with compressed pubkey (0x03)",
			script: hexToBytes("2103b0bd634234abbb1ba1e986e884185" +
				"c61cf43e001f9137f23c2c409273eb16e65ac"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("03b0bd634234abbb" +
					"1ba1e986e884185c61cf43e001f9137f23c2" +
					"c409273eb16e65")),
			},
			reqSigs: 1,
			class:   txscript.PubKeyTy,
		},
		{
			name: "2nd standard p2pk with uncompressed pubkey (0x04)",
			script: hexToBytes("4104b0bd634234abbb1ba1e986e884185" +
				"c61cf43e001f9137f23c2c409273eb16e6537a576782" +
				"eba668a7ef8bd3b3cfb1edb7117ab65129b8a2e681f3" +
				"c1e0908ef7bac"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("04b0bd634234abbb" +
					"1ba1e986e884185c61cf43e001f9137f23c2" +
					"c409273eb16e6537a576782eba668a7ef8bd" +
					"3b3cfb1edb7117ab65129b8a2e681f3c1e09" +
					"08ef7b")),
			},
			reqSigs: 1,
			class:   txscript.PubKeyTy,
		},
		{
			name: "standard p2pkh",
			script: hexToBytes("76a914ad06dd6ddee55cbca9a9e3713bd" +
				"7587509a3056488ac"),
			addrs: []dcrutil.Address{
				newAddressPubKeyHash(hexToBytes("ad06dd6ddee5" +
					"5cbca9a9e3713bd7587509a30564")),
			},
			reqSigs: 1,
			class:   txscript.PubKeyHashTy,
		},
		{
			name: "standard p2sh",
			script: hexToBytes("a91463bcc565f9e68ee0189dd5cc67f1b" +
				"0e5f02f45cb87"),
			addrs: []dcrutil.Address{
				newAddressScriptHash(hexToBytes("63bcc565f9e6" +
					"8ee0189dd5cc67f1b0e5f02f45cb")),
			},
			reqSigs: 1,
			class:   txscript.ScriptHashTy,
		},
		// from real tx 60a20bd93aa49ab4b28d514ec10b06e1829ce6818ec06cd3aabd013ebcdc4bb1, vout 0
		{
			name: "standard 1 of 2 multisig",
			script: hexToBytes("514104cc71eb30d653c0c3163990c47b9" +
				"76f3fb3f37cccdcbedb169a1dfef58bbfbfaff7d8a47" +
				"3e7e2e6d317b87bafe8bde97e3cf8f065dec022b51d1" +
				"1fcdd0d348ac4410461cbdcc5409fb4b4d42b51d3338" +
				"1354d80e550078cb532a34bfa2fcfdeb7d76519aecc6" +
				"2770f5b0e4ef8551946d8a540911abe3e7854a26f39f" +
				"58b25c15342af52ae"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("04cc71eb30d653c0" +
					"c3163990c47b976f3fb3f37cccdcbedb169a" +
					"1dfef58bbfbfaff7d8a473e7e2e6d317b87b" +
					"afe8bde97e3cf8f065dec022b51d11fcdd0d" +
					"348ac4")),
				newAddressPubKey(hexToBytes("0461cbdcc5409fb4" +
					"b4d42b51d33381354d80e550078cb532a34b" +
					"fa2fcfdeb7d76519aecc62770f5b0e4ef855" +
					"1946d8a540911abe3e7854a26f39f58b25c1" +
					"5342af")),
			},
			reqSigs: 1,
			class:   txscript.MultiSigTy,
		},
		// from real tx d646f82bd5fbdb94a36872ce460f97662b80c3050ad3209bef9d1e398ea277ab, vin 1
		{
			name: "standard 2 of 3 multisig",
			script: hexToBytes("524104cb9c3c222c5f7a7d3b9bd152f36" +
				"3a0b6d54c9eb312c4d4f9af1e8551b6c421a6a4ab0e2" +
				"9105f24de20ff463c1c91fcf3bf662cdde4783d4799f" +
				"787cb7c08869b4104ccc588420deeebea22a7e900cc8" +
				"b68620d2212c374604e3487ca08f1ff3ae12bdc63951" +
				"4d0ec8612a2d3c519f084d9a00cbbe3b53d071e9b09e" +
				"71e610b036aa24104ab47ad1939edcb3db65f7fedea6" +
				"2bbf781c5410d3f22a7a3a56ffefb2238af8627363bd" +
				"f2ed97c1f89784a1aecdb43384f11d2acc64443c7fc2" +
				"99cef0400421a53ae"),
			addrs: []dcrutil.Address{
				newAddressPubKey(hexToBytes("04cb9c3c222c5f7a" +
					"7d3b9bd152f363a0b6d54c9eb312c4d4f9af" +
					"1e8551b6c421a6a4ab0e29105f24de20ff46" +
					"3c1c91fcf3bf662cdde4783d4799f787cb7c" +
					"08869b")),
				newAddressPubKey(hexToBytes("04ccc588420deeeb" +
					"ea22a7e900cc8b68620d2212c374604e3487" +
					"ca08f1ff3ae12bdc639514d0ec8612a2d3c5" +
					"19f084d9a00cbbe3b53d071e9b09e71e610b" +
					"036aa2")),
				newAddressPubKey(hexToBytes("04ab47ad1939edcb" +
					"3db65f7fedea62bbf781c5410d3f22a7a3a5" +
					"6ffefb2238af8627363bdf2ed97c1f89784a" +
					"1aecdb43384f11d2acc64443c7fc299cef04" +
					"00421a")),
			},
			reqSigs: 2,
			class:   txscript.MultiSigTy,
		},

		// The below are nonstandard script due to things such as
		// invalid pubkeys, failure to parse, and not being of a
		// standard form.

		{
			name: "p2pk with uncompressed pk missing OP_CHECKSIG",
			script: hexToBytes("410411db93e1dcdb8a016b49840f8c53b" +
				"c1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddf" +
				"b84ccf9744464f82e160bfa9b8b64f9d4c03f999b864" +
				"3f656b412a3"),
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
		{
			name: "valid signature from a sigscript - no addresses",
			script: hexToBytes("47304402204e45e16932b8af514961a1d" +
				"3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41022" +
				"0181522ec8eca07de4860a4acdd12909d831cc56cbba" +
				"c4622082221a8768d1d0901"),
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
		// Note the technically the pubkey is the second item on the
		// stack, but since the address extraction intentionally only
		// works with standard PkScripts, this should not return any
		// addresses.
		{
			name: "valid sigscript to redeem p2pk - no addresses",
			script: hexToBytes("493046022100ddc69738bf2336318e4e0" +
				"41a5a77f305da87428ab1606f023260017854350ddc0" +
				"22100817af09d2eec36862d16009852b7e3a0f6dd765" +
				"98290b7834e1453660367e07a014104cd4240c198e12" +
				"523b6f9cb9f5bed06de1ba37e96a1bbd13745fcf9d11" +
				"c25b1dff9a519675d198804ba9962d3eca2d5937d58e" +
				"5a75a71042d40388a4d307f887d"),
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
		// adapted from btc:
		// tx 691dd277dc0e90a462a3d652a1171686de49cf19067cd33c7df0392833fb986a, vout 0
		// invalid public keys
		{
			name: "1 of 3 multisig with invalid pubkeys",
			script: hexToBytes("5141042200007353455857696b696c656" +
				"16b73204361626c6567617465204261636b75700a0a6" +
				"361626c65676174652d3230313031323034313831312" +
				"e377a0a0a446f41046e6c6f61642074686520666f6c6" +
				"c6f77696e67207472616e73616374696f6e732077697" +
				"468205361746f736869204e616b616d6f746f2773206" +
				"46f776e6c6f61410420746f6f6c2077686963680a636" +
				"16e20626520666f756e6420696e207472616e7361637" +
				"4696f6e2036633533636439383731313965663739376" +
				"435616463636453ae"),
			addrs:   []dcrutil.Address{},
			reqSigs: 1,
			class:   txscript.MultiSigTy,
		},
		// adapted from btc:
		// tx 691dd277dc0e90a462a3d652a1171686de49cf19067cd33c7df0392833fb986a, vout 44
		// invalid public keys
		{
			name: "1 of 3 multisig with invalid pubkeys 2",
			script: hexToBytes("514104633365633235396337346461636" +
				"536666430383862343463656638630a6336366263313" +
				"93936633862393461333831316233363536313866653" +
				"16539623162354104636163636539393361333938386" +
				"134363966636336643664616266640a3236363363666" +
				"13963663463303363363039633539336333653931666" +
				"56465373032392102323364643432643235363339643" +
				"338613663663530616234636434340a00000053ae"),
			addrs:   []dcrutil.Address{},
			reqSigs: 1,
			class:   txscript.MultiSigTy,
		},
		{
			name:    "empty script",
			script:  []byte{},
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
		},
		{
			name:    "script that does not parse",
			script:  []byte{txscript.OP_DATA_45},
			addrs:   nil,
			reqSigs: 0,
			class:   txscript.NonStandardTy,
			noparse: true,
		},
	}

	t.Logf("Running %d tests.", len(tests))
	for i, test := range tests {
		class, addrs, reqSigs, err := txscript.ExtractPkScriptAddrs(scriptVersion,
			test.script, mainNetParams)
		if err != nil && !test.noparse {
			t.Errorf("ExtractPkScriptAddrs #%d (%s): %v", i,
				test.name, err)
		}

		if !reflect.DeepEqual(addrs, test.addrs) {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"addresses\ngot  %v\nwant %v", i, test.name,
				addrs, test.addrs)
			continue
		}

		if reqSigs != test.reqSigs {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"number of required signatures - got %d, "+
				"want %d", i, test.name, reqSigs, test.reqSigs)
			continue
		}

		if class != test.class {
			t.Errorf("ExtractPkScriptAddrs #%d (%s) unexpected "+
				"script type - got %s, want %s", i, test.name,
				class, test.class)
			continue
		}
	}
}

// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdaddr

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/decred/base58"
	"github.com/decred/dcrd/crypto/ripemd160"
)

// mockAddrParams implements the AddressParams interface and is used throughout
// the tests to mock multiple networks.
type mockAddrParams struct {
	pubKeyID     [2]byte
	pkhEcdsaID   [2]byte
	pkhEd25519ID [2]byte
	pkhSchnorrID [2]byte
	scriptHashID [2]byte
	privKeyID    [2]byte
}

// AddrIDPubKeyV0 returns the magic prefix bytes associated with the mock params
// for version 0 pay-to-pubkey addresses.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyV0() [2]byte {
	return p.pubKeyID
}

// AddrIDPubKeyHashECDSAV0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey is secp256k1 and the signature algorithm is ECDSA.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashECDSAV0() [2]byte {
	return p.pkhEcdsaID
}

// AddrIDPubKeyHashEd25519V0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey and signature algorithm are Ed25519.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashEd25519V0() [2]byte {
	return p.pkhEd25519ID
}

// AddrIDPubKeyHashSchnorrV0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey is secp256k1 and the signature algorithm is Schnorr.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashSchnorrV0() [2]byte {
	return p.pkhSchnorrID
}

// AddrIDScriptHashV0 returns the magic prefix bytes associated with the mock
// params for version 0 pay-to-script-hash addresses.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDScriptHashV0() [2]byte {
	return p.scriptHashID
}

// mockMainNetParams returns mock mainnet address parameters to use throughout
// the tests.  They match the Decred mainnet params as of the time this comment
// was written.
func mockMainNetParams() *mockAddrParams {
	return &mockAddrParams{
		pubKeyID:     [2]byte{0x13, 0x86}, // starts with Dk
		pkhEcdsaID:   [2]byte{0x07, 0x3f}, // starts with Ds
		pkhEd25519ID: [2]byte{0x07, 0x1f}, // starts with De
		pkhSchnorrID: [2]byte{0x07, 0x01}, // starts with DS
		scriptHashID: [2]byte{0x07, 0x1a}, // starts with Dc
		privKeyID:    [2]byte{0x22, 0xde}, // starts with Pm
	}
}

// mockTestNetParams returns mock testnet address parameters to use throughout
// the tests.  They match the Decred testnet params as of the time this comment
// was written.
func mockTestNetParams() *mockAddrParams {
	return &mockAddrParams{
		pubKeyID:     [2]byte{0x28, 0xf7}, // starts with Tk
		pkhEcdsaID:   [2]byte{0x0f, 0x21}, // starts with Ts
		pkhEd25519ID: [2]byte{0x0f, 0x01}, // starts with Te
		pkhSchnorrID: [2]byte{0x0e, 0xe3}, // starts with TS
		scriptHashID: [2]byte{0x0e, 0xfc}, // starts with Tc
		privKeyID:    [2]byte{0x23, 0x0e}, // starts with Pt
	}
}

// TestAddresses ensures that address-related APIs work as intended including
// that they are properly encoded and decoded, that they produce the expected
// payment-related scripts, and that error paths fail as expected.  For
// addresses that implement the stake address interface, the stake-related
// scripts are also tested.
func TestAddresses(t *testing.T) {
	mainNetParams := mockMainNetParams()
	testNetParams := mockTestNetParams()

	type newAddrFn func() (Address, error)
	tests := []struct {
		name         string        // test description
		makeAddr     newAddrFn     // function to construct new address via API
		makeErr      error         // expected error from new address function
		addr         string        // expected address and address to decode
		net          AddressParams // params for network
		decodeErr    error         // expected error from decode
		version      uint16        // expected scripts version
		payScript    string        // hex-encoded expected payment script
		voteScript   string        // hex-encoded expected voting rights script
		rewardAmount int64         // reward commitment amount
		feeLimits    uint16        // reward fee limits commitment
		rewardScript string        // hex-encoded expected reward commitment script
		changeScript string        // hex-encoded expected stake change script
		commitScript string        // hex-encoded expected vote commitment script
		revokeScript string        // hex-encoded expected revoke commitment script
		trsyScript   string        // hex-encoded expected pay from treasury script
	}{{
		// ---------------------------------------------------------------------
		// Misc decoding error tests.
		// ---------------------------------------------------------------------

		name:      "bad checksum",
		addr:      "TsmWaPM77WSyA3aiQ2Q1KnwGDVWvEkhip23",
		net:       testNetParams,
		decodeErr: ErrBadAddressChecksum,
	}, {
		name:      "parse valid mainnet address with testnet rejected",
		addr:      "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		net:       testNetParams,
		decodeErr: ErrUnsupportedAddress,
	}, {
		name:      "mainnet p2pk with no data for pubkey",
		addr:      "Aiz5jz1s",
		net:       mainNetParams,
		decodeErr: ErrUnsupportedAddress,
	}, {
		name:      "invalid base58 (l not in base58 alphabet)",
		addr:      "DsUZxxoHlSty8DCfwfartwTYbuhmVct7tJu",
		net:       mainNetParams,
		decodeErr: ErrUnsupportedAddress,
	}}

	for _, test := range tests {
		// Create address from test constructor and ensure it produces the
		// expected encoded address when the constructor is specified.
		if test.makeAddr != nil {
			addr, err := test.makeAddr()
			if !errors.Is(err, test.makeErr) {
				t.Errorf("%s: mismatched err -- got %v, want %v", test.name, err,
					test.makeErr)
				continue
			}
			if err != nil {
				continue
			}

			// Ensure encoding the address is the same as the original.
			encoded := addr.Address()
			if encoded != test.addr {
				t.Errorf("%s: unexpected address -- got %v, want %v", test.name,
					encoded, test.addr)
				continue
			}
		}

		// Decode address and ensure the expected error is received.
		decodedAddr, err := DecodeAddress(test.addr, test.net)
		if !errors.Is(err, test.decodeErr) {
			t.Errorf("%s: mismatched err -- got %v, want %v", test.name, err,
				test.decodeErr)
			continue
		}
		if err != nil {
			continue
		}

		// Ensure the payment script version and contents are the expected
		// values.
		wantPayScript, err := hex.DecodeString(test.payScript)
		if err != nil {
			t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
			continue
		}
		gotPayScriptVersion, gotPayScript := decodedAddr.PaymentScript()
		if gotPayScriptVersion != test.version {
			t.Errorf("%s: mismatched payment script version -- got %d, want %d",
				test.name, gotPayScriptVersion, test.version)
			continue
		}
		if !bytes.Equal(gotPayScript, wantPayScript) {
			t.Errorf("%s: mismatched payment script -- got %x, want %x",
				test.name, gotPayScript, wantPayScript)
			continue
		}

		// Ensure stake-specific interface results produce the expected values.
		if stakeAddr, ok := decodedAddr.(StakeAddress); ok {
			// Ensure the voting rights script version and contents are the
			// expected values.
			wantScript, err := hex.DecodeString(test.voteScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript := stakeAddr.VotingRightsScript()
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched voting rights script version -- got "+
					"%d, want %d", test.name, gotScriptVer,
					test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched voting rights script -- got %x, want %x",
					test.name, gotScript, wantScript)
				continue
			}

			// Ensure the reward commitment script version and contents are the
			// expected values.
			wantScript, err = hex.DecodeString(test.rewardScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript = stakeAddr.RewardCommitmentScript(
				test.rewardAmount, test.feeLimits)
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched reward cmt script version -- got %d, "+
					"want %d", test.name, gotScriptVer, test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched reward cmt script -- got %x, want %x",
					test.name, gotScript, wantScript)
				continue
			}

			// Ensure the stake change script version and contents are the
			// expected values.
			wantScript, err = hex.DecodeString(test.changeScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript = stakeAddr.StakeChangeScript()
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched change script version -- got %d, "+
					"want %d", test.name, gotScriptVer, test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched change script -- got %x, want %x",
					test.name, gotScript, wantScript)
				continue
			}

			// Ensure the vote commitment script version and contents are the
			// expected values.
			wantScript, err = hex.DecodeString(test.commitScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript = stakeAddr.PayVoteCommitmentScript()
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched vote commit script version -- got %d, "+
					"want %d", test.name, gotScriptVer, test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched vote commit script -- got %x, want %x",
					test.name, gotScript, wantScript)
				continue
			}

			// Ensure the revoke commitment script version and contents are the
			// expected values.
			wantScript, err = hex.DecodeString(test.revokeScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript = stakeAddr.PayRevokeCommitmentScript()
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched revoke cmt script version -- got %d, "+
					"want %d", test.name, gotScriptVer, test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched revoke cmt script -- got %x, want %x",
					test.name, gotScript, wantScript)
				continue
			}

			// Ensure the pay from treasury script version and contents are the
			// expected values.
			wantScript, err = hex.DecodeString(test.trsyScript)
			if err != nil {
				t.Errorf("%s: unexpected hex decode err: %v", test.name, err)
				continue
			}
			gotScriptVer, gotScript = stakeAddr.PayFromTreasuryScript()
			if gotScriptVer != test.version {
				t.Errorf("%s: mismatched treasury change script version -- "+
					"got %d, want %d", test.name, gotScriptVer, test.version)
				continue
			}
			if !bytes.Equal(gotScript, wantScript) {
				t.Errorf("%s: mismatched treasury change script -- got %x, "+
					"want %x", test.name, gotScript, wantScript)
				continue
			}
		}

		// Ensure encoding the address is the same as the original.
		encoded := decodedAddr.Address()
		if encoded != test.addr {
			t.Errorf("%s: decoding and encoding produced different addresses "+
				"-- got %v, want %v", test.name, encoded, test.addr)
			continue
		}

		// Ensure the stringer returns the same address as the original.
		if ds, ok := decodedAddr.(fmt.Stringer); ok && ds.String() != test.addr {
			t.Errorf("%s: mismatched decoded stringer -- got %v, want %v",
				test.name, ds.String(), test.addr)
			continue
		}

		// Ensure the Hash160 method for the addresses that support it returns
		// the expected value.
		if h160er, ok := decodedAddr.(Hash160er); ok {
			decodedBytes := base58.Decode(test.addr)
			wantH160 := decodedBytes[2 : 2+ripemd160.Size]
			if gotH160 := h160er.Hash160()[:]; !bytes.Equal(gotH160, wantH160) {
				t.Errorf("%s: mismatched hash160 -- got %x, want %x", test.name,
					gotH160, wantH160)
				return
			}
		}
	}
}

// TestDecodeAddressV0Corners ensures that some additional errors that are
// specific to decoding version 0 addresses directly, as opposed to via the
// generic API, work as intended.  This is necessary because the generic address
// decoding function contains additional logic to avoid even attempting to
// decode addresses which can't possibly be one of the supported version 0
// address types, while the version 0 decoding logic specifically attempts to
// decode the address in order to provide more detailed errors.
func TestDecodeAddressV0Corners(t *testing.T) {
	mainNetParams := mockMainNetParams()

	tests := []struct {
		name      string        // test description
		addr      string        // expected address and address to decode
		net       AddressParams // params for network
		decodeErr error         // expected error from decode
	}{{
		// ---------------------------------------------------------------------
		// Misc decoding error tests.
		// ---------------------------------------------------------------------

		name:      "mainnet p2pk with no data for pubkey",
		addr:      "Aiz5jz1s",
		net:       mainNetParams,
		decodeErr: ErrMalformedAddressData,
	}, {
		name:      "invalid base58 (l not in base58 alphabet)",
		addr:      "DsUZxxoHlSty8DCfwfartwTYbuhmVct7tJu",
		net:       mainNetParams,
		decodeErr: ErrMalformedAddress,
	}}

	for _, test := range tests {
		_, err := DecodeAddressV0(test.addr, test.net)
		if !errors.Is(err, test.decodeErr) {
			t.Errorf("%s: mismatched err -- got %v, want %v", test.name, err,
				test.decodeErr)
			continue
		}
	}
}

// TestProbablyV0Base58Addr ensures the function that determines if an address
// is probably a base58 address works as intended by checking off by ones and
// ensuring all allowed characters in the modified base58 alphabet are accepted.
func TestProbablyV0Base58Addr(t *testing.T) {
	tests := []struct {
		name string // test description
		str  string // string to test
		want bool   // expected result
	}{{
		name: "all allowed base58 chars part 1",
		str:  "123456789ABCDEFGHJKLMNPQRSTUVWXYZab",
		want: true,
	}, {
		name: "all allowed base58 chars part 2",
		str:  "QRSTUVWXYZabcdefghijkmnopqrstuvwxyz",
		want: true,
	}, {
		name: "invalid base58 (0 not in base58 alphabet, one less than '1')",
		str:  "DsUZxxoH0Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 ({ not in base58 alphabet, one more than 'z')",
		str:  "DsUZxxoH{Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (I not in base58 alphabet)",
		str:  "DsUZxxoHISty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (O not in base58 alphabet)",
		str:  "DsUZxxoHOSty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (l not in base58 alphabet)",
		str:  "DsUZxxoHlSty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (: not in base58 alphabet, one more than '9')",
		str:  "DsUZxxoH:Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (@ not in base58 alphabet, one less than 'A')",
		str:  "DsUZxxoH@Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 ([ not in base58 alphabet, one more than 'Z')",
		str:  "DsUZxxoH[Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}, {
		name: "invalid base58 (` not in base58 alphabet, one less than 'a')",
		str:  "DsUZxxoH`Sty8DCfwfartwTYbuhmVct7tJu",
		want: false,
	}}

	for _, test := range tests {
		got := probablyV0Base58Addr(test.str)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
			continue
		}
	}
}

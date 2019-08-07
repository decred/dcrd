// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestCalcMerkleRoot ensures the expected merkle root is produced for known
// valid leaf values.
func TestCalcMerkleRoot(t *testing.T) {
	tests := []struct {
		name   string   // test description
		leaves []string // leaves to test
		want   string   // expected result
	}{{
		name:   "no leaves",
		leaves: nil,
		want:   "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "single leaf (mainnet block 1)",
		leaves: []string{
			"b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
		},
		want: "b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
	}, {
		name: "even number of leaves (mainnet block 257)",
		leaves: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
			"9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
			"c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7ce1b2613e21f40d7076c1b2283f363134be992b5fd648a928f023e9cf42de5e",
			"2f568d89cde2957d68a27f41854245b73c1469314e7f31783614bf1919761bcf",
			"e146022bebf7a4273a61084ce20ee5c03f94afbe6744ed48e436169a147a1d1c",
			"a714a3a6f16b18c5b82321b9425a4205b205afd4d83d3f392d6a36af4222c9dd",
			"25f65b3814c55de20576d35fc68ecc202bf058352746c9e2347f7e59f5a2c677",
			"81120d7af7f8d37287ecf558a2d47f1e631bec486e485cb4aab4996a1c2ee7ab",
			"0e3e1ffd23240dbc3e148754eb63faa784e9d338f196cf77b5d821749282fb0c",
			"91d53551633e8b7a894b4e7277616f65203e997c4346895d234a8a2dcea6c849",
			"3caf3db1714a8f7c9b847be782ee2750f3f7073eadbc43a309c800a3d6b1c887",
			"41161b6e5cc65bee31a26b1603e5d701151d9778de6cd0044fb5533dd0da7fe7",
			"a1273c356109ff1d6145eca2ed14b1c5025f0024bf18ae249b8d185b4192cf6e",
			"ceed5ebb8faa597795d04fe06c404e32e72d9d6db43d57b41affc842c402a5c8",
			"7c756776f01aa0e2b115bbef0527a12fe03aadf598fdbf99576dc973fbc42cdc",
			"472c27828b8ecd51f038a676aa9dc2e8d144cc292885e342a37852ec6d0d78a7",
			"bbc48709276a223b6689d181aacfd8684fbb5a91bd7c890e487a3b73ab4b43d5",
			"6c796c53a51ecf8fa0dd7feffbf3c1ca277b17533bb6fc87645527471c2d5499",
			"bec32f1016fd40f2adac39dfbcedb3e45b6d7f9b37cb340d22bce14015759632",
			"06024a8ddaafa5c4b448168bebd8f37d7fb15eef079933579cf29b45dd40edfb",
		},
		want: "4aa7bcd77d51f6f4db4983e731b5e08b3ea724c5cb99d3debd3d75fd67e7c72b",
	}, {
		name: "odd number of leaves > 1 (mainnet block 260)",
		leaves: []string{
			"5e574591d900f7f9abb8f8eb31cc9330247d27ba293ad79c348d602ece717b8b",
			"b3b70fe08c2da744c9559d533e8db35b3bfefba1b0f1c7b31e7d9d523c00a426",
			"dd3058a7fc691ff4dee0a8cd6030f404ffda7e7aee88aff3985f7b2bbe4792f7",
		},
		want: "a144c719391569aa20bf612bf5588bce71cd397574cb6c060e0bac100f6e5805",
	}}

	testFuncs := []string{"CalcMerkleRoot", "CalcMerkleRootInPlace"}
	for _, funcName := range testFuncs {
	nextTest:
		for _, test := range tests {
			// Parse the leaves and store a copy for ensuring they were not
			// mutated.
			leaves := make([]chainhash.Hash, 0, len(test.leaves))
			for _, hashStr := range test.leaves {
				hash, err := chainhash.NewHashFromStr(hashStr)
				if err != nil {
					t.Errorf("%q: unexpected err parsing leaf %q: %v",
						test.name, hashStr, err)
					continue nextTest
				}
				leaves = append(leaves, *hash)
			}
			origLeaves := make([]chainhash.Hash, len(leaves))
			copy(origLeaves, leaves)

			// Parse the expected merkle root.
			want, err := chainhash.NewHashFromStr(test.want)
			if err != nil {
				t.Errorf("%q: unexpected err parsing want hex: %v", test.name,
					err)
				continue nextTest
			}

			// Choose the correct function to use to calculate the merkle root
			// for this iteration.
			var f func([]chainhash.Hash) chainhash.Hash
			switch funcName {
			case "CalcMerkleRoot":
				f = CalcMerkleRoot
			case "CalcMerkleRootInPlace":
				f = CalcMerkleRootInPlace
			default:
				t.Fatalf("invalid function name: %v", funcName)
			}
			result := f(leaves)
			if result != *want {
				t.Errorf("%q: mismatched result -- got %v, want %v", test.name,
					result, *want)
				continue nextTest
			}

			// Ensure the leaves were not mutated for the in-place version.
			if funcName == "CalcMerkleRoot" {
				if len(leaves) != len(origLeaves) {
					t.Errorf("%q: unexpected leaf mutation -- len %v, want %v",
						test.name, len(leaves), len(origLeaves))
					continue nextTest
				}

				for i := range leaves {
					if leaves[i] != origLeaves[i] {
						t.Errorf("%q: unexpected mutation -- got %v, want %v",
							test.name, leaves[i], origLeaves[i])
						continue nextTest
					}
				}
			}
		}
	}
}

// TestCalcTxTreeMerkleRoot ensures the expected merkle root is produced for
// known transactions.
func TestCalcTxTreeMerkleRoot(t *testing.T) {
	tests := []struct {
		name string   // test description
		txns []string // transactions to test
		want string   // expected result
	}{{
		name: "no transactions",
		txns: nil,
		want: "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "single transaction (mainnet block 2)",
		txns: []string{"01000000010000000000000000000000000000000000000000000" +
			"000000000000000000000ffffffff00ffffffff03fa1a981200000000000017a" +
			"914f5916158e3e2c4551c1796708db8367207ed13bb870000000000000000000" +
			"0266a2402000000000000000000000000000000000000000000000000000000f" +
			"fa310d9a6a9588edea1906f0000000000001976a9148ffe7a49ecf0f4858e7a5" +
			"2155302177398d2296988ac000000000000000001d8bc2882000000000000000" +
			"0ffffffff0800002f646372642f",
		},
		want: "c867d085c96604812854399bf6df63d35d857484fedfd147759ed94c3cdeca35",
	}, {
		name: "two transactions (mainnet block 1347)",
		txns: []string{
			"0100000001000000000000000000000000000000000000000000000000000000" +
				"0000000000ffffffff00ffffffff03fa1a981200000000000017a914f591" +
				"6158e3e2c4551c1796708db8367207ed13bb870000000000000000000026" +
				"6a2443050000000000000000000000000000000000000000000000000000" +
				"da2f65220b2d81aedea1906f0000000000001976a914b60ee40ada8e797a" +
				"c6e363ad8c781155000ecf7688ac000000000000000001d8bc2882000000" +
				"0000000000ffffffff0800002f646372642f",

			"0100000001a9c88bc52429e4cb7e91832c5a6908ff46b9171bf4c02e01eec6ee" +
				"af44c3ff550000000000ffffffff02a06870390100000000001976a91446" +
				"28bf5fdd6d4ee9ef281aa7e9c8636ed4e8623e88ac0050d6dc0100000000" +
				"001976a914c4e25c9d857f0389135ac05d7724638d963b003488ac000000" +
				"000000000001b0675a1603000000c0040000010000006b48304502210089" +
				"c186d7459817c81d1e7aa2dd8dd98d60228689ef7a8c6f8548d5b53792c1" +
				"f202200562ac4af193b5f0d2308a5e7b4e2e4d925deb8bab0693bfb5312f" +
				"a31f45a12c01210353284744f576413877e35c1cbe90c84c129fe1c60650" +
				"1181927e2e1649b3f3c4",
		},
		want: "7d366112c093b22ebb138815eaeb5edd692913489f9a53f143fa90349df177e4",
	}}

	for _, test := range tests {
		// Parse the transactions.
		txns := make([]*wire.MsgTx, 0, len(test.txns))
		for _, txHex := range test.txns {
			txBytes, err := hex.DecodeString(txHex)
			if err != nil {
				t.Errorf("%q: unexpected err parsing tx hex %q: %v", test.name,
					txHex, err)
				continue
			}

			var tx wire.MsgTx
			if err := tx.FromBytes(txBytes); err != nil {
				t.Errorf("%q: unexpected err parsing tx: %v", test.name, err)
				continue
			}
			txns = append(txns, &tx)
		}

		// Parse the expected merkle root.
		want, err := chainhash.NewHashFromStr(test.want)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want hex: %v", test.name,
				err)
			continue
		}

		result := CalcTxTreeMerkleRoot(txns)
		if result != *want {
			t.Errorf("%q: mismatched result -- got %v, want %v", test.name,
				result, *want)
			continue
		}
	}
}

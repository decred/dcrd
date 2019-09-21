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

// TestCalcCombinedTxTreeMerkleRoot ensures the expected combined merkle root is
// produced for known transaction trees.
func TestCalcCombinedTxTreeMerkleRoot(t *testing.T) {
	tests := []struct {
		name        string   // test description
		regularTxns []string // regular transactions to test
		stakeTxns   []string // stake transactions to test
		want        string   // expected result
	}{{
		name:        "no transactions",
		regularTxns: nil,
		stakeTxns:   nil,
		want:        "988c02a849815a2c70d97fd613a333d766bcb250cd263663c58d4f954240996d",
	}, {
		name: "single regular tx, single stake tx (from simnet testing)",
		regularTxns: []string{"0100000001000000000000000000000000000000000000" +
			"0000000000000000000000000000ffffffff00ffffffff0300f2052a01000000" +
			"000017a914cbb08d6ca783b533b2c7d24a51fbca92d937bf9987000000000000" +
			"000000000e6a0c3300000062d5e2db66f6c42900ac23fc0600000000001976a9" +
			"14dcd4f28811382640a0d357bf88b14905840c46c788ac000000000000000001" +
			"009e29260800000000000000ffffffff0800002f646372642f",
		},
		stakeTxns: []string{"010000000167835eeb4d3ac8ee05417e93fbb3cf14ba8bcb" +
			"07745fbac5433e15cc3b8eb27d0d00000000ffffffff03204e00000000000000" +
			"001aba76a914fd28ffadc79bfc63642b59aec4a2ee5b2ccf248188ac00000000" +
			"000000000000206a1e6f360606284502c81ce5b3feb0d71c7874999c34c45900" +
			"00000000000058000000000000000000001abd76a91400000000000000000000" +
			"0000000000000000000088ac000000003800000001c459000000000000320000" +
			"00010000006a473044022062abf944c8f258efa9442d70aa6b5a4241c81e8dc1" +
			"b99fffc741d4af40d6475f022044aae4b930d69ffb825df6a2f9a01f6c8a7abc" +
			"1fcab70573aace5be1926ed6650121034c9b704a36fab21e12cbb508691c3159" +
			"3f3fce4f3dd11fb0e8fac44c25c8600b",
		},
		want: "f6b7bd7ac6f1d61c6e48ae1e53302ccc84da2f3b7802a09244c2657a203aa9af",
	}, {
		name: "two regular txns, two stake txns (from simnet testing)",
		regularTxns: []string{
			"0100000001000000000000000000000000000000000000000000000000000000" +
				"0000000000ffffffff00ffffffff0300f2052a01000000000017a914cbb0" +
				"8d6ca783b533b2c7d24a51fbca92d937bf9987000000000000000000000e" +
				"6a0c35000000defbb528181937eb00ac23fc0600000000001976a914dcd4" +
				"f28811382640a0d357bf88b14905840c46c788ac00000000000000000100" +
				"9e29260800000000000000ffffffff0800002f646372642f",

			"01000000012efb68aa183bf9560b038998c0b2a2ba8d207e931b1b08f2224487" +
				"ab093212070200000000ffffffff02acb28afb0600000000001976a91464" +
				"089c6868937d4d7c7978cf7d36f2e742da001588ac809698000000000000" +
				"001976a914d5303c8a454b03f2c0d7833bf09f3b14cc76cec288ac000000" +
				"00000000000100ac23fc060000000a000000000000006b48304502210080" +
				"a15360bd1d592e1add1b2bfc7ef8837aa73b90d543a744a359d243c1db06" +
				"4102204ee34ec6f955841d1cafffa5ad18810bd8f1bf2cd2fe0d3a732b1c" +
				"b23d4ca1890121028b85616443f420a39500ff0bea67d52181e949e5203f" +
				"24413ec0d959758d786d",
		},
		stakeTxns: []string{
			"01000000017364d1a023b2db7b141b6a94e6c4ace36442a338b8175f92a5e4d9" +
				"668ee1f2180100000000ffffffff03204e00000000000000001aba76a914" +
				"53a7f2d43babdafdaa510b9c3a3c17145bbf8f2188ac0000000000000000" +
				"0000206a1e874ec2f5538743d21bba9d6ffbff4bc2abd47baac459000000" +
				"0000000058000000000000000000001abd76a91400000000000000000000" +
				"0000000000000000000088ac000000000000000001c45900000000000034" +
				"000000010000006a473044022041fa36d2d4d0af2ed567167ef395ddfb21" +
				"bed9ec9a502bb20d7e728b9627402e02200f59f391232108c6c75713a65e" +
				"beefbda618d5ab908b80da9c35d8565fca987e012102d871b270e4764359" +
				"7ff904da3d3b0c9a370fec94930d34002feaa3eb3ffc02dd",

			"01000000017364d1a023b2db7b141b6a94e6c4ace36442a338b8175f92a5e4d9" +
				"668ee1f2180000000000ffffffff03204e00000000000000001aba76a914" +
				"53a7f2d43babdafdaa510b9c3a3c17145bbf8f2188ac0000000000000000" +
				"0000206a1ea4f98c668009c921f9e359b2c48cedf17e6174d2c459000000" +
				"0000000058000000000000000000001abd76a91400000000000000000000" +
				"0000000000000000000088ac000000000000000001c45900000000000034" +
				"000000010000006a473044022075102c950466576529642ffbc26f3a4251" +
				"6d53e4aa04fcbd7fff1be0fe8a637a022022baa009da17952e1cf60c8b76" +
				"46b669479da404b0f4d1323a746c123dcffb09012102d871b270e4764359" +
				"7ff904da3d3b0c9a370fec94930d34002feaa3eb3ffc02dd",
		},
		want: "4d82a32275ef4f9e858fbb88a3b61e6f86fba567d87976e639551d3667b6bca2",
	}}

	for _, test := range tests {
		// Parse the regular and stake transactions.
		regularTxns := make([]*wire.MsgTx, 0, len(test.regularTxns))
		for _, txHex := range test.regularTxns {
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
			regularTxns = append(regularTxns, &tx)
		}
		stakeTxns := make([]*wire.MsgTx, 0, len(test.stakeTxns))
		for _, txHex := range test.stakeTxns {
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
			stakeTxns = append(stakeTxns, &tx)
		}

		// Parse the expected merkle root.
		want, err := chainhash.NewHashFromStr(test.want)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want hex: %v", test.name,
				err)
			continue
		}

		result := CalcCombinedTxTreeMerkleRoot(regularTxns, stakeTxns)
		if result != *want {
			t.Errorf("%q: mismatched result -- got %v, want %v", test.name,
				result, *want)
			continue
		}
	}
}

// TestReferenceCalcCombinedMerkleRoot ensures the expected combined merkle root
// is produced for values in DCP0005.
func TestReferenceCalcCombinedMerkleRoot(t *testing.T) {
	tests := []struct {
		name        string // test description
		regularRoot string // regular transaction tree merkle root
		stakeRoot   string // stake transaction tree merkle root
		want        string // expected result
	}{{
		name:        "both zero",
		regularRoot: "0000000000000000000000000000000000000000000000000000000000000000",
		stakeRoot:   "0000000000000000000000000000000000000000000000000000000000000000",
		want:        "988c02a849815a2c70d97fd613a333d766bcb250cd263663c58d4f954240996d",
	}, {
		name:        "stake zero",
		regularRoot: "c867d085c96604812854399bf6df63d35d857484fedfd147759ed94c3cdeca35",
		stakeRoot:   "0000000000000000000000000000000000000000000000000000000000000000",
		want:        "1ac7b08e89cf9a09aa37cbc6da5ef565387d5ab7e8a405a6bf75a31efb1a6987",
	}, {
		name:        "retroactive block 257",
		regularRoot: "4aa7bcd77d51f6f4db4983e731b5e08b3ea724c5cb99d3debd3d75fd67e7c72b",
		stakeRoot:   "53c5472957646d0b5a33ed482138df8b9212d7c00553fca6929351ec912d9a43",
		want:        "9f5c6658689c627a81f29fd54389a4f4e225d59c72257762b62d19da06f76028",
	}, {
		name:        "retroactive block 258",
		regularRoot: "6a37f6c6e7925b011611375ea4bd68aa712770ff9e62fe2291f35d44bfc0da09",
		stakeRoot:   "ec8ca6b6d79d6643b3eff69c669d590e0217ba783f921d8a747829db4122267c",
		want:        "cbbf08173144d05299380009af546de876fadb5c5a7ca2b865e18c87b97c3e66",
	}, {
		name:        "retroactive block 259",
		regularRoot: "928bfdec06342966a27cfa784309cbede715928965c68940fa5b402e4870312d",
		stakeRoot:   "d159449bfa08efc8f55cce3d0a0c3ce43392907193dd244d0c4a90b210c34e89",
		want:        "223a8a09f339daaec50123ffc9ffcafcd9a0daec7d6574f86874b8dca354c85f",
	}, {
		name:        "retroactive block 260",
		regularRoot: "a144c719391569aa20bf612bf5588bce71cd397574cb6c060e0bac100f6e5805",
		stakeRoot:   "dd45ab0b099fbed6dd4ba8e91488cb10c8928e5a82975b03952094b141a93747",
		want:        "85b49d61a922ae883bb31c8f0064ed9c2550425ffc1bd026a9986102ef34b2ad",
	}, {
		name:        "retroactive block 261",
		regularRoot: "5429f4db1794c827a31cf2f25d7e56e6ac8df6adda7be4971c11413dbbd96d53",
		stakeRoot:   "502b3d79caed7b10c4b982f3bfff6847c052f60af77c2bf3a7b41126c6a140a7",
		want:        "025d2d91dff22afd92d3111541c1c61cde9045f6e7fa04296ec8b12fcf3d5b70",
	}, {
		name:        "retroactive block 262",
		regularRoot: "9f3a41320b2c591a47b27bb1379aa7b13462e851977a4a7006b95cfaa0e0bbec",
		stakeRoot:   "00d0a7c9be36a059d47e5c62b1ac81c9263c4d3d3a9c4221b7f80eb257d60603",
		want:        "8b6d589261aa0d7ede6b719255bf989ee22967faf51af5533337e711abd49ee6",
	}, {
		name:        "single regular tx, single stake tx (from simnet testing)",
		regularRoot: "a7bb523ac8ee4cfb39f8ccc9a02281825f7e5d88d6874d5e08724f6bb0ac5083",
		stakeRoot:   "826c95263cc3c7de75b0503796c96a0072f8e4da5adef8b95eee27654008a77b",
		want:        "f6b7bd7ac6f1d61c6e48ae1e53302ccc84da2f3b7802a09244c2657a203aa9af",
	}, {
		name:        "two regular txns, two stake txns (from simnet testing)",
		regularRoot: "b0953c08e873651c5505ff9a1f9d337c258d0719d1951536c1aa379bf7a0d523",
		stakeRoot:   "ab0c7a1e7fab629ef74f95c09c03172489791b29db064c6e5dffdad3da31c4e0",
		want:        "4d82a32275ef4f9e858fbb88a3b61e6f86fba567d87976e639551d3667b6bca2",
	}}

	for _, test := range tests {
		// Parse the regular and stake merkle roots.
		regularRoot, err := chainhash.NewHashFromStr(test.regularRoot)
		if err != nil {
			t.Errorf("%q: unexpected err parsing regular root: %v", test.name,
				err)
			continue
		}
		stakeRoot, err := chainhash.NewHashFromStr(test.stakeRoot)
		if err != nil {
			t.Errorf("%q: unexpected err parsing stake root: %v", test.name,
				err)
			continue
		}

		// Parse the expected merkle root.
		want, err := chainhash.NewHashFromStr(test.want)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want hex: %v", test.name,
				err)
			continue
		}

		result := CalcMerkleRoot([]chainhash.Hash{*regularRoot, *stakeRoot})
		if result != *want {
			t.Errorf("%q: mismatched result -- got %v, want %v", test.name,
				result, *want)
			continue
		}
	}
}

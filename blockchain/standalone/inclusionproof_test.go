// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestGenerateInclusionProof ensures the expected proofs are produced for
// known valid leaf values.
func TestGenerateInclusionProof(t *testing.T) {
	tests := []struct {
		name      string   // test description
		leaves    []string // leaves to test
		leafIndex uint32   // leaf index to test
		want      []string // expected proof
	}{{
		name:      "no leaves",
		leaves:    nil,
		leafIndex: 0,
		want:      nil,
	}, {
		name: "single leaf, leaf index 1 -- out of range",
		leaves: []string{
			"b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
		},
		leafIndex: 1,
		want:      nil,
	}, {
		name: "single leaf, leaf index 0 (left)",
		leaves: []string{
			"b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
		},
		leafIndex: 0,
		want:      nil,
	}, {
		name: "2 leaves, leaf index 1 (right)",
		leaves: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
			"9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
		},
		leafIndex: 1,
		want: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
		},
	}, {
		name: "5 leaves, leaf index 2 (left, right, left)",
		leaves: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
			"9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
			"c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7ce1b2613e21f40d7076c1b2283f363134be992b5fd648a928f023e9cf42de5e",
		},
		leafIndex: 2,
		want: []string{
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
			"b92bb84b19e850458f4eabc098e2990f3931e8b88e9a72a41162e9ae4e2a371a",
		},
	}, {
		name: "22 leaves, leaf index 17 (right, left, left, left, right)",
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
		leafIndex: 17,
		want: []string{
			"7c756776f01aa0e2b115bbef0527a12fe03aadf598fdbf99576dc973fbc42cdc",
			"dc9ecbcb5c2c5bc167bd2b655d24c2cd3928628762ccf66124be1acae1d375c4",
			"d1c35369f005419c4e0f62778939f5ccfc1a6dad5403b4976b5043cd374d5fc4",
			"74a272f7e786ff653dacdab7e9ec04b5a9eb1228bdf1f379f2b7b467efda8e1f",
			"730ec07e8a5bde0d66aef48e59ccd3588ca7daf50428ef2584827542a6d3f50a",
		},
	}, {
		name: "22 leaves, leaf index 8 (left, left, left, right, left)",
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
		leafIndex: 8,
		want: []string{
			"81120d7af7f8d37287ecf558a2d47f1e631bec486e485cb4aab4996a1c2ee7ab",
			"f5fdbb6fc248ded76d32a2c476bbda2f71a94ab9e97ab17f9fa6ae54b9678ae2",
			"61ef60d83b8fac54143a425ff701e39f84160945dc6148a72ef21b36463d4055",
			"bb87df9e2104a7b1006bafd20d57b3232713bb98e04a07417ad92068d61d73e0",
			"7655d6fe0c1994489bc8d71b70b40d854607fd8d012c538a103d272611ef69c8",
		},
	}}

nextTest:
	for _, test := range tests {
		// Parse the leaves.
		leaves := make([]chainhash.Hash, 0, len(test.leaves))
		for _, hashStr := range test.leaves {
			hash, err := chainhash.NewHashFromStr(hashStr)
			if err != nil {
				t.Errorf("%q: unexpected err parsing leaf %q: %v", test.name,
					hashStr, err)
				continue nextTest
			}
			leaves = append(leaves, *hash)
		}

		// Parse the expected proof.
		wantProof := make([]chainhash.Hash, 0, len(test.want))
		for _, hashStr := range test.want {
			hash, err := chainhash.NewHashFromStr(hashStr)
			if err != nil {
				t.Errorf("%q: unexpected err parsing proof hash %q: %v",
					test.name, hashStr, err)
				continue nextTest
			}
			wantProof = append(wantProof, *hash)
		}

		// Generate the proof and ensure it matches the expected result.
		result := GenerateInclusionProof(leaves, test.leafIndex)
		if len(result) != len(wantProof) {
			t.Errorf("%q: unexpected proof length -- got %d, want %d",
				test.name, len(result), len(wantProof))
			continue
		}
		for i := range result {
			if result[i] != wantProof[i] {
				t.Errorf("%q: unexpected proof hash at index %d -- got %s, "+
					"want %s", test.name, i, result[i], wantProof[i])
				continue nextTest
			}
		}
	}
}

// TestVerifyInclusionProof ensures the expected results for various known valid
// and invalid inclusion proofs.
func TestVerifyInclusionProof(t *testing.T) {
	tests := []struct {
		name      string   // test description
		root      string   // root to test
		leaf      string   // leaf to test
		leafIndex uint32   // leaf index to test
		proof     []string // proof to test
		want      bool     // expected result
	}{{
		name:      "single leaf, leaf index 0 (left)",
		root:      "b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
		leaf:      "b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347",
		leafIndex: 0,
		proof:     nil,
		want:      true,
	}, {
		name:      "2 leaves, leaf index 1 (right)",
		root:      "7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
		leaf:      "9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
		leafIndex: 1,
		proof: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
		},
		want: true,
	}, {
		name:      "2 leaves, leaf index 1 (right) -- mismatched root",
		root:      "7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf1",
		leaf:      "9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
		leafIndex: 1,
		proof: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
		},
		want: false,
	}, {
		name:      "2 leaves, leaf index 1 (right) -- mismatched proof size with duplicate",
		root:      "7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
		leaf:      "9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
		leafIndex: 1,
		proof: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb7",
		},
		want: false,
	}, {
		name:      "2 leaves, leaf index 1 (right) -- mismatched proof hash",
		root:      "7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
		leaf:      "9518f53fccc008baf771a6610d4ac506a931286b7e67d98d49bde68e3dec10aa",
		leafIndex: 1,
		proof: []string{
			"46670d055dae85e8f9eceb5d30b1433c7232d3b09068fbde4741db3714dafdb6",
		},
		want: false,
	}, {
		name:      "5 leaves, leaf index 2 (left, right, left)",
		root:      "0b2eb5d6213d6faa732578212aabf3f6e0b73853eb9cc753d2915473b14c4d0f",
		leaf:      "c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
		leafIndex: 2,
		proof: []string{
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
			"b92bb84b19e850458f4eabc098e2990f3931e8b88e9a72a41162e9ae4e2a371a",
		},
		want: true,
	}, {
		name:      "5 leaves, leaf index 2 (left, right, left) -- wrong index",
		root:      "0b2eb5d6213d6faa732578212aabf3f6e0b73853eb9cc753d2915473b14c4d0f",
		leaf:      "c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
		leafIndex: 3,
		proof: []string{
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
			"b92bb84b19e850458f4eabc098e2990f3931e8b88e9a72a41162e9ae4e2a371a",
		},
		want: false,
	}, {
		name:      "5 leaves, leaf index 2 (left, right, left) -- short proof",
		root:      "0b2eb5d6213d6faa732578212aabf3f6e0b73853eb9cc753d2915473b14c4d0f",
		leaf:      "c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
		leafIndex: 2,
		proof: []string{
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
		},
		want: false,
	}, {
		name:      "5 leaves, leaf index 2 (left, right, left) -- proof levels swapped",
		root:      "0b2eb5d6213d6faa732578212aabf3f6e0b73853eb9cc753d2915473b14c4d0f",
		leaf:      "c9bf74b6da5a82e5f720859f9b7730aab59e774fb1c22bef534e60206c1f87b4",
		leafIndex: 2,
		proof: []string{
			"c0657dd580e76866de1a008e691ffcafe790deb733ec79b7b4dea64ab4abd002",
			"b92bb84b19e850458f4eabc098e2990f3931e8b88e9a72a41162e9ae4e2a371a",
			"7569f8adf70ab7a404a6d691c80d2eb10efd35120c526c8d9c6afc038a88dcf0",
		},
		want: false,
	}, {
		name:      "22 leaves, leaf index 17 (right, left, left, left, right)",
		root:      "4aa7bcd77d51f6f4db4983e731b5e08b3ea724c5cb99d3debd3d75fd67e7c72b",
		leaf:      "472c27828b8ecd51f038a676aa9dc2e8d144cc292885e342a37852ec6d0d78a7",
		leafIndex: 17,
		proof: []string{
			"7c756776f01aa0e2b115bbef0527a12fe03aadf598fdbf99576dc973fbc42cdc",
			"dc9ecbcb5c2c5bc167bd2b655d24c2cd3928628762ccf66124be1acae1d375c4",
			"d1c35369f005419c4e0f62778939f5ccfc1a6dad5403b4976b5043cd374d5fc4",
			"74a272f7e786ff653dacdab7e9ec04b5a9eb1228bdf1f379f2b7b467efda8e1f",
			"730ec07e8a5bde0d66aef48e59ccd3588ca7daf50428ef2584827542a6d3f50a",
		},
		want: true,
	}, {
		name:      "22 leaves, leaf index 8 (left, left, left, right, left)",
		root:      "4aa7bcd77d51f6f4db4983e731b5e08b3ea724c5cb99d3debd3d75fd67e7c72b",
		leaf:      "25f65b3814c55de20576d35fc68ecc202bf058352746c9e2347f7e59f5a2c677",
		leafIndex: 8,
		proof: []string{
			"81120d7af7f8d37287ecf558a2d47f1e631bec486e485cb4aab4996a1c2ee7ab",
			"f5fdbb6fc248ded76d32a2c476bbda2f71a94ab9e97ab17f9fa6ae54b9678ae2",
			"61ef60d83b8fac54143a425ff701e39f84160945dc6148a72ef21b36463d4055",
			"bb87df9e2104a7b1006bafd20d57b3232713bb98e04a07417ad92068d61d73e0",
			"7655d6fe0c1994489bc8d71b70b40d854607fd8d012c538a103d272611ef69c8",
		},
		want: true,
	}}

nextTest:
	for _, test := range tests {
		// Parse the merkle root.
		root, err := chainhash.NewHashFromStr(test.root)
		if err != nil {
			t.Errorf("%q: unexpected err parsing merkle root %q: %v", test.name,
				test.root, err)
			continue
		}

		// Parse the merkle leaf.
		leaf, err := chainhash.NewHashFromStr(test.leaf)
		if err != nil {
			t.Errorf("%q: unexpected err parsing leaf %q: %v", test.name,
				test.leaf, err)
			continue
		}

		// Parse the proof.
		proof := make([]chainhash.Hash, 0, len(test.proof))
		for _, hashStr := range test.proof {
			hash, err := chainhash.NewHashFromStr(hashStr)
			if err != nil {
				t.Errorf("%q: unexpected err parsing proof hash %q: %v",
					test.name, hashStr, err)
				continue nextTest
			}
			proof = append(proof, *hash)
		}

		// Verify the proof and ensure the expected result matches.
		result := VerifyInclusionProof(root, leaf, test.leafIndex, proof)
		if result != test.want {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name,
				result, test.want)
			continue
		}
	}
}

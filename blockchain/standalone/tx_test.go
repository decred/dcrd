// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/wire"
)

// TestIsCoinbaseTx ensures the coinbase identification works as intended.
func TestIsCoinbaseTx(t *testing.T) {
	tests := []struct {
		name string // test description
		tx   string // transaction to test
		want bool   // expected coinbase test result
	}{{
		name: "mainnet block 2 coinbase",
		tx: "010000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff03fa1a981200000000000017a914f59161" +
			"58e3e2c4551c1796708db8367207ed13bb8700000000000000000000266a240" +
			"2000000000000000000000000000000000000000000000000000000ffa310d9" +
			"a6a9588edea1906f0000000000001976a9148ffe7a49ecf0f4858e7a5215530" +
			"2177398d2296988ac000000000000000001d8bc28820000000000000000ffff" +
			"ffff0800002f646372642f",
		want: true,
	}, {
		name: "mainnet block 3, tx[1] (one input), not coinbase",
		tx: "0100000001e68bcb9222c7f6336e865c81d7fd3e4b3244cd83998ac9767efcb" +
			"355b3cd295efe02000000ffffffff01105ba3940600000000001976a914dcfd" +
			"20801304752f618295dfe0a4c044afcfde3a88ac000000000000000001e04aa" +
			"7940600000001000000000000006b48304502210089d763b0c28314b5eb0d4c" +
			"97e0183a78bb4a656dcbcd293d29d91921a64c55af02203554e76f432f73862" +
			"edd4f2ed80a4599141b13c6ac2406158b05a97c6867a1ba01210244709193c0" +
			"5a649df0fb0a96180ec1a8e3cbcc478dc9c4a69a3ec5aba1e97a79",
		want: false,
	}, {
		name: "mainnet block 373, tx[5] (two inputs), not coinbase",
		tx: "010000000201261057a5ecaf6edede86c5446c62f067f30d654117668325090" +
			"9ac3e45bec00100000000ffffffff03c65ad19cb990cc916e38dc94f0255f34" +
			"4c5e9b7af3b69bfa19931f6027e44c0100000000ffffffff02c1c5760000000" +
			"00000001976a914e07c0b2a499312f5d95e3bd4f126e618087a15a588ac402c" +
			"42060000000000001976a91411f2b3135e457259009bdd14cfcb942eec58bd7" +
			"a88ac0000000000000000023851f6050000000073010000040000006a473044" +
			"022009ff5aed5d2e5eeec89319d0a700b7abdf842e248641804c82dee17df44" +
			"6c24202207c252cc36199ea8a6cc71d2252a3f7e61f9cce272dff82c5818e3b" +
			"f08167e3a6012102773925f9ee53837aa0efba2212f71ee8ab20aeb603fa732" +
			"4a8c2555efe5c482709ec0e010000000025010000050000006a473044022011" +
			"65136a2b792cc6d7e75f576ed64e1919cbf954afb989f8590844a628e58def0" +
			"2206ba7e60f5ae9810794297359cc883e7ff97ecd21bc7177fcc668a84f64a4" +
			"b9120121026a4151513b4e6650e3d213451037cd6b78ed829d12ed1d43d5d34" +
			"ce0834831e9",
		want: false,
	}}

	for _, test := range tests {
		txBytes, err := hex.DecodeString(test.tx)
		if err != nil {
			t.Errorf("%q: unexpected err parsing tx hex %q: %v", test.name,
				test.tx, err)
			continue
		}

		var tx wire.MsgTx
		if err := tx.FromBytes(txBytes); err != nil {
			t.Errorf("%q: unexpected err parsing tx: %v", test.name, err)
			continue
		}

		result := IsCoinBaseTx(&tx)
		if result != test.want {
			t.Errorf("%s: unexpected result -- got %v, want %v", test.name,
				result, test.want)
			continue
		}
	}
}

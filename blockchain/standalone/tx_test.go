// Copyright (c) 2019-2020 The Decred developers
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
		name         string // test description
		tx           string // transaction to test
		wantPreTrsy  bool   // expected coinbase result before treasury active
		wantPostTrsy bool   // expected coinbase result after treasury active
	}{{
		name: "mainnet block 2 coinbase",
		tx: "010000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff03fa1a981200000000000017a914f59161" +
			"58e3e2c4551c1796708db8367207ed13bb8700000000000000000000266a240" +
			"2000000000000000000000000000000000000000000000000000000ffa310d9" +
			"a6a9588edea1906f0000000000001976a9148ffe7a49ecf0f4858e7a5215530" +
			"2177398d2296988ac000000000000000001d8bc28820000000000000000ffff" +
			"ffff0800002f646372642f",
		wantPreTrsy:  true,
		wantPostTrsy: false,
	}, {
		name: "modified mainnet block 2 coinbase: tx version 3",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff02fa1a981200000000000017a914f59161" +
			"58e3e2c4551c1796708db8367207ed13bb8700000000000000000000266a240" +
			"2000000000000000000000000000000000000000000000000000000ffa310d9" +
			"a6a9588e000000000000000001d8bc28820000000000000000ffffffff08000" +
			"02f646372642f",
		wantPreTrsy:  true,
		wantPostTrsy: true,
	}, {
		name: "modified mainnet block 2 coinbase: tx version 3, no miner payout",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff02fa1a981200000000000017a914f59161" +
			"58e3e2c4551c1796708db8367207ed13bb8700000000000000000000266a240" +
			"2000000000000000000000000000000000000000000000000000000ffa310d9" +
			"a6a9588e000000000000000001d8bc28820000000000000000ffffffff08000" +
			"02f646372642f",
		wantPreTrsy:  true,
		wantPostTrsy: true,
	}, {
		name: "mainnet block 3, tx[1] (one input), not coinbase",
		tx: "0100000001e68bcb9222c7f6336e865c81d7fd3e4b3244cd83998ac9767efcb" +
			"355b3cd295efe02000000ffffffff01105ba3940600000000001976a914dcfd" +
			"20801304752f618295dfe0a4c044afcfde3a88ac000000000000000001e04aa" +
			"7940600000001000000000000006b48304502210089d763b0c28314b5eb0d4c" +
			"97e0183a78bb4a656dcbcd293d29d91921a64c55af02203554e76f432f73862" +
			"edd4f2ed80a4599141b13c6ac2406158b05a97c6867a1ba01210244709193c0" +
			"5a649df0fb0a96180ec1a8e3cbcc478dc9c4a69a3ec5aba1e97a79",
		wantPreTrsy:  false,
		wantPostTrsy: false,
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
		wantPreTrsy:  false,
		wantPostTrsy: false,
	}, {
		name: "simnet block 32 coinbase (treasury active)",
		tx: "0300000001000000000000000000000000000000000000000000000000000000" +
			"0000000000ffffffff00ffffffff02000000000000000000000e6a0c20000000" +
			"93c1b2181e4cd3b100ac23fc0600000000001976a91423d4150eb4332733b5bf" +
			"88e5d9cea3897bc09dbc88ac00000000000000000100ac23fc06000000000000" +
			"00ffffffff0800002f646372642f",
		wantPreTrsy:  true,
		wantPostTrsy: true,
	}, {
		name: "modified simnet block 32 coinbase (treasury active): no miner payout",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff01000000000000000000000e6a0c200000" +
			"0093c1b2181e4cd3b100000000000000000100ac23fc0600000000000000fff" +
			"fffff0800002f646372642f",
		wantPreTrsy:  true,
		wantPostTrsy: true,
	}, {
		name: "random simnet treasury spend",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200000000000000000000226a20b63c13" +
			"400000000045c279dd8870eff33bc219a4c9a39a9190960601d8fccdefc0321" +
			"3400000000000001ac376a914c945ac8cdbf5e37ad4bfde5dc92f65e13ff5c6" +
			"7488ac000000008201000001b63c13400000000000000000ffffffff6440650" +
			"063174184d0438b26d05a2f6d3190d020994ef3c18ac110bf70df3d1ed7066c" +
			"8c62c349b8ae86862d04ee94d2bddc42bd33d449c1ba53ed37a2c8d1e8f8f62" +
			"102a36b785d584555696b69d1b2bbeff4010332b301e3edd316d79438554cac" +
			"b3e7c2",
		wantPreTrsy:  true,
		wantPostTrsy: false,
	}, {
		// This is neither a valid coinbase nor a valid treasury spend, but
		// since the coinbase identification function is only a fast heuristic,
		// this is crafted to test that.
		name: "modified random simnet treasury spend: missing signature script",
		tx: "0300000001000000000000000000000000000000000000000000000000000000" +
			"0000000000ffffffff00ffffffff0200000000000000000000226a20b63c1340" +
			"0000000045c279dd8870eff33bc219a4c9a39a9190960601d8fccdefc0321340" +
			"0000000000001ac376a914c945ac8cdbf5e37ad4bfde5dc92f65e13ff5c67488" +
			"ac000000008201000001b63c13400000000000000000ffffffff00",
		wantPreTrsy:  true,
		wantPostTrsy: true,
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

		result := IsCoinBaseTx(&tx, noTreasury)
		if result != test.wantPreTrsy {
			t.Errorf("%s: unexpected result pre treasury -- got %v, want %v",
				test.name, result, test.wantPreTrsy)
			continue
		}

		result = IsCoinBaseTx(&tx, withTreasury)
		if result != test.wantPostTrsy {
			t.Errorf("%s: unexpected result post treasury -- got %v, want %v",
				test.name, result, test.wantPostTrsy)
			continue
		}

	}
}

// TestIsTreasurybaseTx ensures treasurybase identification works as intended.
func TestIsTreasurybaseTx(t *testing.T) {
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
		want: false,
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
	}, {
		name: "simnet generated treasury base",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a01000000000001c100000000" +
			"0000000000000e6a0c4600000020cb18b066b6523c00000000000000000100f" +
			"2052a0100000000000000ffffffff00",
		want: true,
	}, {
		name: "modified: no inputs",
		tx: "03000000000200f2052a01000000000001c1000000000000000000000e6a0c4" +
			"600000020cb18b066b6523c000000000000000000",
		want: false,
	}, {
		name: "modified: only one output",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0100f2052a01000000000001c100000000" +
			"000000000100f2052a0100000000000000ffffffff00",
		want: false,
	}, {
		name: "modified: first input sig script not empty",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a01000000000001c100000000" +
			"0000000000000e6a0c4600000020cb18b066b6523c00000000000000000100f" +
			"2052a0100000000000000ffffffff016a",
		want: false,
	}, {
		name: "modified: first output pkscript missing",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a010000000000000000000000" +
			"00000000000e6a0c4600000020cb18b066b6523c00000000000000000100f20" +
			"52a0100000000000000ffffffff00",
		want: false,
	}, {
		name: "modified: first output pkscript not OP_TADD",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a010000000000016a00000000" +
			"0000000000000e6a0c4600000020cb18b066b6523c00000000000000000100f" +
			"2052a0100000000000000ffffffff00",
		want: false,
	}, {
		name: "modified: second output pkscript normal payout (not OP_RETURN)",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a01000000000001c100000000" +
			"0000000000001976a91423d4150eb4332733b5bf88e5d9cea3897bc09dbc88a" +
			"c00000000000000000100f2052a0100000000000000ffffffff00",
		want: false,
	}, {
		name: "modified: second output pkscript 13-byte OP_RETURN",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff00ffffffff0200f2052a01000000000001c100000000" +
			"0000000000000f6a0d20000000e36029002399907a010000000000000000010" +
			"0f2052a0100000000000000ffffffff00",
		want: false,
	}, {
		name: "modified: input prevout tree not regular",
		tx: "030000000100000000000000000000000000000000000000000000000000000" +
			"00000000000ffffffff01ffffffff0200f2052a01000000000001c100000000" +
			"0000000000000e6a0c4600000020cb18b066b6523c00000000000000000100f" +
			"2052a0100000000000000ffffffff00",
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

		result := IsTreasuryBase(&tx)
		if result != test.want {
			t.Errorf("%s: unexpected result -- got %v, want %v", test.name,
				result, test.want)
			continue
		}
	}
}

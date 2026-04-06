// Copyright (c) 2024-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/wire"
)

var (
	testStartingHeight uint32 = 100
	testStartingBlock         = chainhash.Hash{100}
)

var testnetParams = chaincfg.TestNet3Params()

type testBlockchain struct{}

func newTestBlockchain() *testBlockchain {
	return &testBlockchain{}
}

func (b *testBlockchain) CurrentTip() (chainhash.Hash, int64) {
	return testStartingBlock, int64(testStartingHeight)
}

func (b *testBlockchain) ChainParams() *chaincfg.Params {
	return testnetParams
}

// Intentionally create orphans and test their acceptance behavior when PRs
// and KEs are accepted.
func TestOrphans(t *testing.T) {
	pub, priv, err := generateSecp256k1(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := *(*[33]byte)(pub.SerializeCompressed())

	h := blake256.New()

	pr := &wire.MsgMixPairReq{
		Identity: id,
		UTXOs: []wire.MixPairReqUTXO{
			{},
		},
		MessageCount: 1,
		Expiry:       testStartingHeight + 10,
		ScriptClass:  string(mixing.ScriptClassP2PKHv0),
		InputValue:   1 << 18,
	}
	err = mixing.SignMessage(pr, priv)
	if err != nil {
		t.Fatal(err)
	}
	pr.WriteHash(h)

	prs := []*wire.MsgMixPairReq{pr}
	epoch := uint64(time.Now().Unix())
	sid := mixing.SortPRsForSession(prs, epoch)
	ke := &wire.MsgMixKeyExchange{
		Identity: id,
		SeenPRs: []chainhash.Hash{
			pr.Hash(),
		},
		SessionID: sid,
		Epoch:     epoch,
		Run:       0,
	}
	err = mixing.SignMessage(ke, priv)
	if err != nil {
		t.Fatal(err)
	}
	ke.WriteHash(h)

	fp := &wire.MsgMixFactoredPoly{
		Identity:  id,
		SessionID: sid,
		Run:       0,
	}
	err = mixing.SignMessage(fp, priv)
	if err != nil {
		t.Fatal(err)
	}
	fp.WriteHash(h)

	t.Logf("pr %s", pr.Hash())
	t.Logf("ke %s", ke.Hash())
	t.Logf("fp %s", fp.Hash())

	// Create a pair request, KE, and later messages that belong to the
	// session for each KE.  Test different combinations of acceptance
	// order to test orphan processing of various message types.
	type accept struct {
		desc     string
		message  mixing.Message
		errors   bool
		errAs    interface{}
		accepted []mixing.Message
	}
	tests := [][]accept{
		// Accept KE, then PR, then FP
		0: {{
			desc:     "accept KE before PR",
			message:  ke,
			errors:   true,
			errAs:    new(MissingOwnPRError),
			accepted: nil,
		}, {
			desc:     "accept PR after KE; both should now process",
			message:  pr,
			errors:   false,
			accepted: []mixing.Message{pr, ke},
		}, {
			desc:     "accept future message in accepted KE session/run",
			message:  fp,
			errors:   false, // maybe later.
			accepted: []mixing.Message{fp},
		}},

		// Accept FP, then KE, then PR
		1: {{
			desc:     "accept FP first",
			message:  fp,
			errors:   false,
			accepted: nil,
		}, {
			desc:     "accept KE",
			message:  ke,
			errors:   true,
			errAs:    new(MissingOwnPRError),
			accepted: nil,
		}, {
			desc:     "accept PR; all should now be processed",
			message:  pr,
			errors:   false,
			accepted: []mixing.Message{pr, ke, fp},
		}},
	}

	for i, accepts := range tests {
		t.Logf("test %d", i)
		mp := NewPool(newTestBlockchain())

		for j, a := range accepts {
			accepted, err := mp.AcceptMessage(a.message, ZeroSource)
			if err != nil != a.errors {
				t.Errorf("test %d call %d %q: unexpected error: %v", i, j, a.desc, err)
			}
			if a.errors && !errors.As(err, &a.errAs) {
				t.Errorf("test %d call %d %q: unexpected error: %v", i, j, a.desc, err)
			}
			if len(accepted) != len(a.accepted) {
				t.Logf("orphans: %v", mp.orphans)
				t.Logf("orphansByID: %v", mp.orphansByID)
				t.Logf("pr: %v", pr)
				t.Errorf("test %d call %d %q: accepted lengths differ %d != %d", i, j, a.desc,
					len(accepted), len(a.accepted))
			}
			if !reflect.DeepEqual(accepted, a.accepted) {
				t.Errorf("test %d call %d %q: accepted messages differs: %#v", i, j, a.desc, accepted)
			}
		}
	}
}

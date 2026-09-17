// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"errors"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/wire"
)

// TestKeyExchangeConflict ensures that one identity cannot cache conflicting
// key exchanges in a session, including when they arrive before the pair request.
// Exact duplicates remain harmless and a new session may use a new key exchange.
func TestKeyExchangeConflict(t *testing.T) {
	pub, priv, err := generateSecp256k1(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := *(*[33]byte)(pub.SerializeCompressed())
	h := blake256.NewHasher256()
	sign := func(msg mixing.Message) {
		t.Helper()
		if err := mixing.SignMessage(msg, priv); err != nil {
			t.Fatal(err)
		}
		msg.WriteHash(h)
	}
	pr := &wire.MsgMixPairReq{
		Identity:     id,
		UTXOs:        []wire.MixPairReqUTXO{{}},
		MessageCount: 1,
		MixAmount:    1 << 18,
		Expiry:       testStartingHeight + 10,
		ScriptClass:  string(mixing.ScriptClassP2PKHv0),
		InputValue:   1<<18 + 3000,
	}
	sign(pr)
	newKE := func(epoch uint64, commitment byte) *wire.MsgMixKeyExchange {
		sid := mixing.SortPRsForSession([]*wire.MsgMixPairReq{pr}, epoch)
		ke := &wire.MsgMixKeyExchange{
			Identity:   id,
			SeenPRs:    []chainhash.Hash{pr.Hash()},
			SessionID:  sid,
			Epoch:      epoch,
			Commitment: [32]byte{commitment},
		}
		sign(ke)
		return ke
	}
	epoch := uint64(time.Now().Unix())
	ke := newKE(epoch, 1)
	conflict := newKE(epoch, 2)

	t.Run("accepted pair request", func(t *testing.T) {
		p := NewPool(newTestBlockchain())
		for _, msg := range []mixing.Message{pr, ke} {
			accepted, err := p.AcceptMessage(msg, ZeroSource)
			if err != nil || len(accepted) != 1 {
				t.Fatalf("accept %T: messages=%d, err=%v", msg, len(accepted), err)
			}
		}
		accepted, err := p.AcceptMessage(ke, ZeroSource)
		if err != nil || len(accepted) != 0 {
			t.Fatalf("duplicate KE: messages=%d, err=%v", len(accepted), err)
		}
		accepted, err = p.AcceptMessage(conflict, ZeroSource)
		if err == nil || len(accepted) != 0 {
			t.Fatalf("conflicting KE: messages=%d, err=%v", len(accepted), err)
		}
		if len(p.pool) != 1 || len(p.messagesByIdentity[id]) != 2 || p.latestKE[id] != ke {
			t.Fatal("conflicting KE changed cached messages or latest KE")
		}
		if got := p.sessions[ke.SessionID].countFor(msgtypeKE); got != 1 {
			t.Fatalf("session has %d KEs, want 1", got)
		}
		accepted, err = p.AcceptMessage(newKE(epoch+1, 3), ZeroSource)
		if err != nil || len(accepted) != 1 {
			t.Fatalf("new session KE: messages=%d, err=%v", len(accepted), err)
		}
	})

	t.Run("orphan reconsideration", func(t *testing.T) {
		p := NewPool(newTestBlockchain())
		for _, msg := range []*wire.MsgMixKeyExchange{ke, conflict} {
			accepted, err := p.AcceptMessage(msg, ZeroSource)
			var missing *MissingOwnPRError
			if !errors.As(err, &missing) || len(accepted) != 0 {
				t.Fatalf("orphan KE: messages=%d, err=%v", len(accepted), err)
			}
		}
		accepted, err := p.AcceptMessage(pr, ZeroSource)
		if err != nil || len(accepted) != 2 {
			t.Fatalf("accept PR and one KE: messages=%d, err=%v", len(accepted), err)
		}
		if len(p.pool) != 1 || len(p.messagesByIdentity[id]) != 2 {
			t.Fatal("multiple conflicting orphan KEs entered the message pool")
		}
		if got := p.sessions[ke.SessionID].countFor(msgtypeKE); got != 1 {
			t.Fatalf("session has %d KEs, want 1", got)
		}
	})
}

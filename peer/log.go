// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
// The default amount of logging is none.
var log = slog.Disabled

// UseLogger uses a specified Logger to output package logging info.
func UseLogger(logger slog.Logger) {
	log = logger
}

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// formatLockTime returns a transaction lock time as a human-readable string.
func formatLockTime(lockTime uint32) string {
	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the lockTimeThreshold.  When it is under the
	// threshold it is a block height.
	if lockTime < txscript.LockTimeThreshold {
		return fmt.Sprintf("height %d", lockTime)
	}

	return time.Unix(int64(lockTime), 0).String()
}

// invSummary returns an inventory message as a human-readable string.
func invSummary(invList []*wire.InvVect) string {
	// No inventory.
	invLen := len(invList)
	if invLen == 0 {
		return "empty"
	}

	// One inventory item.
	if invLen == 1 {
		iv := invList[0]
		switch iv.Type {
		case wire.InvTypeError:
			return fmt.Sprintf("error %s", iv.Hash)
		case wire.InvTypeBlock:
			return fmt.Sprintf("block %s", iv.Hash)
		case wire.InvTypeTx:
			return fmt.Sprintf("tx %s", iv.Hash)
		case wire.InvTypeFilteredBlock:
			return fmt.Sprintf("filtered block %s", iv.Hash)
		case wire.InvTypeMix:
			return fmt.Sprintf("mix message %s", iv.Hash)
		}

		return fmt.Sprintf("unknown (%d) %s", uint32(iv.Type), iv.Hash)
	}

	// More than one inv item.
	var numTxns, numBlocks, numMixes uint64
	for _, iv := range invList {
		switch iv.Type {
		case wire.InvTypeTx:
			numTxns++
		case wire.InvTypeBlock:
			numBlocks++
		case wire.InvTypeMix:
			numMixes++
		}
	}

	diff := uint64(invLen) - (numTxns + numBlocks + numMixes)
	return fmt.Sprintf("txns %d, blocks %d, mixmsgs %d, other %d", numTxns,
		numBlocks, numMixes, diff)
}

// locatorSummary returns a block locator as a human-readable string.
func locatorSummary(locator []*chainhash.Hash, stopHash *chainhash.Hash) string {
	if len(locator) > 0 {
		return fmt.Sprintf("locator %s, stop %s", locator[0], stopHash)
	}

	return fmt.Sprintf("no locator, stop %s", stopHash)
}

type mixMessage interface {
	Hash() chainhash.Hash
	Pub() []byte
	Sid() []byte    // PR returns nil
	GetRun() uint32 // PR returns 0
}

// mixMessageSummary returns short summary of a mixing message as a
// human-readable string.
func mixMessageSummary(msg mixMessage) string {
	if pr, ok := msg.(*wire.MsgMixPairReq); ok {
		return fmt.Sprintf("identity %x, hash %v", pr.Pub(), pr.Hash())
	}
	return fmt.Sprintf("identity %x, hash %v, session %x, run %d",
		msg.Pub(), msg.Hash(), msg.Sid(), msg.GetRun())
}

// messageSummary returns a human-readable string which summarizes a message.
// Not all messages have or need a summary.  This is used for debug logging.
func messageSummary(msg wire.Message) string {
	switch msg := msg.(type) {
	case *wire.MsgVersion:
		return fmt.Sprintf("agent %s, pver %d, block %d",
			msg.UserAgent, msg.ProtocolVersion, msg.LastBlock)

	case *wire.MsgVerAck:
		// No summary.

	case *wire.MsgGetAddr:
		// No summary.

	case *wire.MsgAddr:
		return fmt.Sprintf("%d addr", len(msg.AddrList))

	case *wire.MsgPing:
		// No summary - perhaps add nonce.

	case *wire.MsgPong:
		// No summary - perhaps add nonce.

	case *wire.MsgMemPool:
		// No summary.

	case *wire.MsgTx:
		return fmt.Sprintf("hash %s, %d inputs, %d outputs, lock %s",
			msg.TxHash(), len(msg.TxIn), len(msg.TxOut),
			formatLockTime(msg.LockTime))

	case *wire.MsgBlock:
		header := &msg.Header
		return fmt.Sprintf("hash %s, ver %d, %d tx, %s", msg.BlockHash(),
			header.Version, len(msg.Transactions), header.Timestamp)

	case *wire.MsgMixPairReq:
		return mixMessageSummary(msg)
	case *wire.MsgMixKeyExchange:
		return mixMessageSummary(msg)
	case *wire.MsgMixCiphertexts:
		return mixMessageSummary(msg)
	case *wire.MsgMixSlotReserve:
		return mixMessageSummary(msg)
	case *wire.MsgMixDCNet:
		return mixMessageSummary(msg)
	case *wire.MsgMixConfirm:
		return mixMessageSummary(msg)
	case *wire.MsgMixFactoredPoly:
		return mixMessageSummary(msg)
	case *wire.MsgMixSecrets:
		return mixMessageSummary(msg)

	case *wire.MsgInv:
		return invSummary(msg.InvList)

	case *wire.MsgNotFound:
		return invSummary(msg.InvList)

	case *wire.MsgGetData:
		return invSummary(msg.InvList)

	case *wire.MsgGetBlocks:
		return locatorSummary(msg.BlockLocatorHashes, &msg.HashStop)

	case *wire.MsgGetHeaders:
		return locatorSummary(msg.BlockLocatorHashes, &msg.HashStop)

	case *wire.MsgHeaders:
		summary := fmt.Sprintf("num %d", len(msg.Headers))
		if len(msg.Headers) > 0 {
			finalHeader := msg.Headers[len(msg.Headers)-1]
			summary = fmt.Sprintf("%s, final hash %s, height %d", summary,
				finalHeader.BlockHash(), finalHeader.Height)
		}
		return summary

	case *wire.MsgGetInitState:
		return fmt.Sprintf("types %v", msg.Types)

	case *wire.MsgInitState:
		return fmt.Sprintf("blocks %d, votes %d, treasury spends %d",
			len(msg.BlockHashes), len(msg.VoteHashes),
			len(msg.TSpendHashes))
	}

	// No summary for other messages.
	return ""
}

// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"fmt"

	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/wire"
)

func checkPRLimits(pr *wire.MsgMixPairReq) error {
	if pr.MixAmount > mixing.MaxMixAmount {
		return ruleError(fmt.Errorf("mixed output value %d exceeds max %d",
			pr.MixAmount, mixing.MaxMixAmount))
	}
	if pr.MessageCount > mixing.MaxMcount {
		return ruleError(fmt.Errorf("message count %d exceeds max %d",
			pr.MessageCount, mixing.MaxMcount))
	}

	return nil
}

func checkKELimits(ke *wire.MsgMixKeyExchange) error {
	if len(ke.SeenPRs) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced PRs exceeds max peers %d",
			len(ke.SeenPRs), mixing.MaxPeers))
	}

	return nil
}

func checkCTLimits(ct *wire.MsgMixCiphertexts) error {
	if len(ct.Ciphertexts) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d ciphertexts exceeds max peers %d",
			len(ct.Ciphertexts), mixing.MaxPeers))
	}
	if len(ct.SeenKeyExchanges) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced KEs exceeds max peers %d",
			len(ct.SeenKeyExchanges), len(ct.Ciphertexts)))
	}

	return nil
}

func checkSRLimits(sr *wire.MsgMixSlotReserve) error {
	if len(sr.DCMix) > mixing.MaxMcount {
		return ruleError(fmt.Errorf("outer DC-mix dimension size %d exceeds max message count %v",
			len(sr.DCMix), mixing.MaxMcount))
	}
	for i := range sr.DCMix {
		if len(sr.DCMix[i]) > mixing.MaxMtot {
			return ruleError(fmt.Errorf("inner DC-mix dimension size %d exceeds max session message total %v",
				len(sr.DCMix[i]), mixing.MaxMtot))
		}
	}
	if len(sr.SeenCiphertexts) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced CTs exceeds max peers %d",
			len(sr.SeenCiphertexts), mixing.MaxPeers))
	}

	return nil
}

func checkDCLimits(dc *wire.MsgMixDCNet) error {
	if len(dc.DCNet) > mixing.MaxMcount {
		return ruleError(fmt.Errorf("outer DC-net dimension size %d exceeds max message count %v",
			len(dc.DCNet), mixing.MaxMcount))
	}
	for i := range dc.DCNet {
		if len(dc.DCNet[i]) > mixing.MaxMtot {
			return ruleError(fmt.Errorf("inner DC-net dimension size %d exceeds max session message total %d",
				len(dc.DCNet[i]), mixing.MaxMtot))
		}
	}
	if len(dc.SeenSlotReserves) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced SRs exceeds max peers %d",
			len(dc.SeenSlotReserves), mixing.MaxPeers))
	}

	return nil
}

func checkCMLimits(cm *wire.MsgMixConfirm) error {
	if sz := cm.Mix.SerializeSize(); sz > mixing.MaxMixTxSerializeSize {
		return ruleError(fmt.Errorf("mix transaction serialize size %d exceeds maximum size %d",
			sz, mixing.MaxMixTxSerializeSize))
	}
	if len(cm.SeenDCNets) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced DCs exceeds max peers %d",
			len(cm.SeenDCNets), mixing.MaxPeers))
	}

	return nil
}

func checkFPLimits(fp *wire.MsgMixFactoredPoly) error {
	if len(fp.Roots) > mixing.MaxMtot {
		return ruleError(fmt.Errorf("%d solved roots exceeds max session message total %d",
			len(fp.Roots), mixing.MaxMtot))
	}
	if len(fp.SeenSlotReserves) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced SRs exceeds max peers %d",
			len(fp.SeenSlotReserves), mixing.MaxPeers))
	}

	return nil
}

func checkRSLimits(rs *wire.MsgMixSecrets) error {
	if len(rs.SlotReserveMsgs) > mixing.MaxMcount {
		return ruleError(fmt.Errorf("%d unpadded SR messages exceeds max message count %d",
			len(rs.SlotReserveMsgs), mixing.MaxMcount))
	}
	if len(rs.DCNetMsgs) > mixing.MaxMcount {
		return ruleError(fmt.Errorf("%d unpadded DC messages exceeds max message count %d",
			len(rs.DCNetMsgs), mixing.MaxMcount))
	}
	if len(rs.SeenSecrets) > mixing.MaxPeers {
		return ruleError(fmt.Errorf("%d referenced RSs exceeds max peers %d",
			len(rs.SeenSecrets), mixing.MaxPeers))
	}

	return nil
}

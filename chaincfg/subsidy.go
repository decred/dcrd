// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

//go:generate go run -tags subsidydefs generatesubsidytables.go

type blockOnePayout struct {
	offset int
	amount int64
}

func tokenPayouts(scriptsHex string, payouts []blockOnePayout) []TokenPayout {
	tokenPayouts := make([]TokenPayout, len(payouts))
	var offset int
	scripts := hexDecode(scriptsHex)
	for i := range payouts {
		tokenPayouts[i] = TokenPayout{
			ScriptVersion: 0,
			Script:        scripts[offset:payouts[i].offset],
			Amount:        payouts[i].amount,
		}
		offset = payouts[i].offset
	}
	return tokenPayouts
}

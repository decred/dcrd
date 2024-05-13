// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
)

// MaxExpiry returns the maximum allowed expiry for a new pair request message
// created with a blockchain tip at tipHeight.
func MaxExpiry(tipHeight uint32, params *chaincfg.Params) uint32 {
	target := params.TargetTimePerBlock
	return tipHeight + uint32(60*time.Minute/target) + 1
}

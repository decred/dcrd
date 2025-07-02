// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"github.com/decred/dcrd/mixing"
)

// PeerNotifier provides an interface to notify peers of status changes related
// to blocks and transactions.
type PeerNotifier interface {
	// AnnounceMixMessages generates and relays inventory vectors of the
	// passed messages.
	AnnounceMixMessages(msgs []mixing.Message)
}

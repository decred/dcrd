// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/gcs/v2"
)

const (
	// HeaderCmtFilterIndex is the proof index for the filter header commitment.
	HeaderCmtFilterIndex = 0
)

// FilterByBlockHash returns the version 2 GCS filter for the given block hash
// when it exists.  This function returns the filters regardless of whether or
// not their associated block is part of the main chain.
//
// An error of type NoFilterError will be returned when the filter for the given
// block hash does not exist.
//
// This function is safe for concurrent access.
func (b *BlockChain) FilterByBlockHash(hash *chainhash.Hash) (*gcs.FilterV2, error) {
	var filter *gcs.FilterV2
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		filter, err = dbFetchGCSFilter(dbTx, hash)
		return err
	})
	if err == nil && filter == nil {
		err = NoFilterError(hash.String())
	}
	return filter, err
}

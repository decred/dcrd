// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"context"

	"github.com/decred/dcrd/database/v3"
)

const (
	// addrIndexName is the human-readable name for the index.
	addrIndexName = "address index"
)

var (
	// addrIndexKey is the key of the address index and the db bucket used
	// to house it.
	addrIndexKey = []byte("txbyaddridx")
)

// DropAddrIndex drops the address index from the provided database if it
// exists.
func DropAddrIndex(ctx context.Context, db database.DB) error {
	// Nothing to do if the index doesn't already exist.
	exists, err := existsIndex(db, addrIndexKey)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	log.Infof("Dropping all legacy %s entries.  This might take a while...",
		addrIndexName)

	// Since the indexes can be so large, attempting to simply delete the bucket
	// in a single database transaction would result in massive memory usage and
	// likely crash many systems due to ulimits.  In order to avoid this, use a
	// cursor to delete a maximum number of entries out of the bucket at a time.
	err = incrementalFlatDrop(ctx, db, addrIndexKey, addrIndexName)
	if err != nil {
		return err
	}

	// Remove the index tip, version, bucket, and in-progress drop flag now that
	// all index entries have been removed.
	err = dropIndexMetadata(db, addrIndexKey)
	if err != nil {
		return err
	}

	log.Infof("Dropped %s", addrIndexName)
	return nil
}

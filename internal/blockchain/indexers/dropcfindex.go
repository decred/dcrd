// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"context"

	"github.com/decred/dcrd/database/v3"
)

const (
	// cfIndexName is the human-readable name for the index.
	cfIndexName = "committed filter index"
)

var (
	// cfIndexParentBucketKey is the name of the parent bucket used to house
	// the index. The rest of the buckets live below this bucket.
	cfIndexParentBucketKey = []byte("cfindexparentbucket")
)

// DropCfIndex drops the CF index from the provided database if it exists.
func DropCfIndex(ctx context.Context, db database.DB) error {
	// Nothing to do if the index doesn't already exist.
	exists, err := existsIndex(db, cfIndexParentBucketKey)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	log.Infof("Dropping all legacy %s entries.  This might take a while...",
		cfIndexName)

	// Remove the index tip, version, bucket, and in-progress drop flag now
	// that all index entries have been removed.
	err = dropIndexMetadata(db, cfIndexParentBucketKey)
	if err != nil {
		return err
	}

	log.Infof("Dropped %s", cfIndexName)
	return nil
}

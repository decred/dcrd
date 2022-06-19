// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

// NB: This package is scehduled for removal once DropConsumerDepsBucket no
// longer needs to be called to remove unneeded persisted data.

import (
	"errors"

	"github.com/decred/dcrd/database/v3"
)

var (
	// spendConsumerDepsBucketName is the name of the bucket used in storing
	// spend journal consumer dependencies.
	spendConsumerDepsBucketName = []byte("spendconsumerdeps")
)

// DropConsumerDepsBucket removes the spend pruner consumer dependencies bucket
// if it exists.
func DropConsumerDepsBucket(db database.DB) error {
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		err := meta.DeleteBucket(spendConsumerDepsBucketName)
		if !errors.Is(err, database.ErrBucketNotFound) {
			return err
		}

		return nil
	})
}

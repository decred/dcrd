// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package votingdb_test

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake/internal/votingdb"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   votingdb.ErrorCode
		want string
	}{
		{votingdb.ErrUninitializedBucket, "ErrUninitializedBucket"},
		{votingdb.ErrMissingKey, "ErrMissingKey"},
		{votingdb.ErrChainStateShortRead, "ErrChainStateShortRead"},
		{votingdb.ErrDatabaseInfoShortRead, "ErrDatabaseInfoShortRead"},
		{votingdb.ErrTallyShortRead, "ErrTallyShortRead"},
		{votingdb.ErrBlockKeyShortRead, "ErrBlockKeyShortRead"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestRuleError tests the error output for the RuleError type.
func TestRuleError(t *testing.T) {
	tests := []struct {
		in   votingdb.DBError
		want string
	}{
		{votingdb.DBError{Description: "duplicate block"},
			"duplicate block",
		},
		{votingdb.DBError{Description: "human-readable error"},
			"human-readable error",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("Error #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

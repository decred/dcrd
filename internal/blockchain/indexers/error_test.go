// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"io"
	"testing"
)

// TestErrorKindStringer tests the stringized output for the ErrorKind type.
func TestErrorKindStringer(t *testing.T) {
	tests := []struct {
		in   ErrorKind
		want string
	}{
		{ErrUnsupportedAddressType, "ErrUnsupportedAddressType"},
		{ErrConnectBlock, "ErrConnectBlock"},
		{ErrDisconnectBlock, "ErrDisconnectBlock"},
		{ErrRemoveSpendDependency, "ErrRemoveSpendDependency"},
		{ErrInvalidNotificationType, "ErrInvalidNotificationType"},
		{ErrInterruptRequested, "ErrInterruptRequested"},
		{ErrFetchSubscription, "ErrFetchSubscription"},
		{ErrFetchTip, "ErrFetchTip"},
		{ErrMissingNotification, "ErrMissingNotification"},
		{ErrBlockNotOnMainChain, "ErrBlockNotOnMainChain"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestIndexerError tests the error output for the IndexerError type.
func TestIndexerError(t *testing.T) {
	tests := []struct {
		in   IndexerError
		want string
	}{{
		IndexerError{Description: "duplicate block"},
		"duplicate block",
	}, {
		IndexerError{Description: "human-readable error"},
		"human-readable error",
	}}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestIndexerErrorKindIsAs ensures both ErrorKind and IndexerError can be
// identified as being a specific error kind via errors.Is and unwrapped
// via errors.As.
func TestIndexerErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrUnsupportedAddressType == ErrUnsupportedAddressType",
		err:       ErrUnsupportedAddressType,
		target:    ErrUnsupportedAddressType,
		wantMatch: true,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "IndexerError.ErrUnsupportedAddressType == ErrUnsupportedAddressType",
		err:       indexerError(ErrUnsupportedAddressType, ""),
		target:    ErrUnsupportedAddressType,
		wantMatch: true,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "IndexerError.ErrUnsupportedAddressType == IndexerError.ErrUnsupportedAddressType",
		err:       indexerError(ErrUnsupportedAddressType, ""),
		target:    indexerError(ErrUnsupportedAddressType, ""),
		wantMatch: true,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "ErrUnsupportedAddressType != ErrConnectBlock",
		err:       ErrUnsupportedAddressType,
		target:    ErrConnectBlock,
		wantMatch: false,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "IndexerError.ErrUnsupportedAddressType != ErrConnectBlock",
		err:       indexerError(ErrUnsupportedAddressType, ""),
		target:    ErrConnectBlock,
		wantMatch: false,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "ErrUnsupportedAddressType != IndexerError.ErrConnectBlock",
		err:       ErrUnsupportedAddressType,
		target:    indexerError(ErrConnectBlock, ""),
		wantMatch: false,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "IndexerError.ErrUnsupportedAddressType != IndexerError.ErrConnectBlock",
		err:       indexerError(ErrUnsupportedAddressType, ""),
		target:    indexerError(ErrConnectBlock, ""),
		wantMatch: false,
		wantAs:    ErrUnsupportedAddressType,
	}, {
		name:      "IndexerError.ErrUnsupportedAddressType != io.EOF",
		err:       indexerError(ErrUnsupportedAddressType, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrUnsupportedAddressType,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error kind can be unwrapped and is the
		// expected kind.
		var kind ErrorKind
		if !errors.As(test.err, &kind) {
			t.Errorf("%s: unable to unwrap to error kind", test.name)
			continue
		}
		if kind != test.wantAs {
			t.Errorf("%s: unexpected unwrapped error kind -- got %v, want %v",
				test.name, kind, test.wantAs)
			continue
		}
	}
}

// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		name        string
		errorKind   ErrorKind
		description string
		wantErr     error
	}{{
		name:        "ErrAddressNotFound",
		errorKind:   ErrAddressNotFound,
		description: "address not found",
		wantErr:     ErrAddressNotFound,
	}, {
		name:        "ErrUnknownAddressType",
		errorKind:   ErrUnknownAddressType,
		description: "unknown address type",
		wantErr:     ErrUnknownAddressType,
	}, {
		name:        "ErrMismatchedAddressType",
		errorKind:   ErrMismatchedAddressType,
		description: "mismatched address type",
		wantErr:     ErrMismatchedAddressType,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test makeError
			err := makeError(test.errorKind, test.description)
			if err.Description != test.description {
				t.Errorf("unexpected error description: want %q, got %q", test.description, err.Description)
			}
			// Test unwrapping
			if !errors.Is(err, test.wantErr) {
				t.Errorf("failed to find the expected error: want %v, got %v", test.wantErr, err.Err)
			}

			// Test ErrorKind.Error
			if got := test.errorKind.Error(); got != string(test.errorKind) {
				t.Errorf("unexpected errorKind: want %v, got %v", string(test.errorKind), got)
			}

			// Test Error.Error
			if got := err.Error(); got != test.description {
				t.Errorf("unexpected error: want %v, got %v", test.description, got)
			}
		})
	}
}

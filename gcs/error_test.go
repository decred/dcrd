// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

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
		{ErrNTooBig, "ErrNTooBig"},
		{ErrPTooBig, "ErrPTooBig"},
		{ErrBTooBig, "ErrBTooBig"},
		{ErrMisserialized, "ErrMisserialized"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestError tests the error output for the Error type.
func TestError(t *testing.T) {
	tests := []struct {
		in   Error
		want string
	}{{
		Error{Description: "duplicate block"},
		"duplicate block",
	}, {
		Error{Description: "human-readable error"},
		"human-readable error",
	},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestErrorKindIsAs ensures both ErrorKind and Error can be identified as being
// a specific error kind via errors.Is and unwrapped via errors.As.
func TestErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrNTooBig == ErrNTooBig",
		err:       ErrNTooBig,
		target:    ErrNTooBig,
		wantMatch: true,
		wantAs:    ErrNTooBig,
	}, {
		name:      "Error.ErrNTooBig == ErrNTooBig",
		err:       makeError(ErrNTooBig, ""),
		target:    ErrNTooBig,
		wantMatch: true,
		wantAs:    ErrNTooBig,
	}, {
		name:      "Error.ErrNTooBig == Error.ErrNTooBig",
		err:       makeError(ErrNTooBig, ""),
		target:    makeError(ErrNTooBig, ""),
		wantMatch: true,
		wantAs:    ErrNTooBig,
	}, {
		name:      "ErrNTooBig != ErrPTooBig",
		err:       ErrNTooBig,
		target:    ErrPTooBig,
		wantMatch: false,
		wantAs:    ErrNTooBig,
	}, {
		name:      "Error.ErrNTooBig != ErrPTooBig",
		err:       makeError(ErrNTooBig, ""),
		target:    ErrPTooBig,
		wantMatch: false,
		wantAs:    ErrNTooBig,
	}, {
		name:      "ErrNTooBig != Error.ErrPTooBig",
		err:       ErrNTooBig,
		target:    makeError(ErrPTooBig, ""),
		wantMatch: false,
		wantAs:    ErrNTooBig,
	}, {
		name:      "Error.ErrNTooBig != Error.ErrPTooBig",
		err:       makeError(ErrNTooBig, ""),
		target:    makeError(ErrPTooBig, ""),
		wantMatch: false,
		wantAs:    ErrNTooBig,
	}, {
		name:      "Error.ErrMisserialized != io.EOF",
		err:       makeError(ErrMisserialized, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrMisserialized,
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

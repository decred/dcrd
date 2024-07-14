// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blake256

import (
	"errors"
	"testing"
)

// TestErrorKindStringer tests the stringized output for the [ErrorKind] type.
func TestErrorKindStringer(t *testing.T) {
	tests := []struct {
		in   ErrorKind
		want string
	}{
		{ErrMalformedState, "ErrMalformedState"},
		{ErrMismatchedState, "ErrMismatchedState"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestError tests the error output for the [Error] type.
func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   Error
		want string
	}{{
		Error{Description: "some error"},
		"some error",
	}, {
		Error{Description: "human-readable error"},
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

// TestErrorKindIsAs ensures both ErrorKind and Error can be identified as being
// a specific error kind via [errors.Is] and unwrapped via [errors.As].
func TestErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrMalformedState == ErrMalformedState",
		err:       ErrMalformedState,
		target:    ErrMalformedState,
		wantMatch: true,
		wantAs:    ErrMalformedState,
	}, {
		name:      "Error.ErrMalformedState == ErrMalformedState",
		err:       makeError(ErrMalformedState, ""),
		target:    ErrMalformedState,
		wantMatch: true,
		wantAs:    ErrMalformedState,
	}, {
		name:      "ErrMalformedState != ErrMismatchedState",
		err:       ErrMalformedState,
		target:    ErrMismatchedState,
		wantMatch: false,
		wantAs:    ErrMalformedState,
	}, {
		name:      "Error.ErrMalformedState != ErrMismatchedState",
		err:       makeError(ErrMalformedState, ""),
		target:    ErrMismatchedState,
		wantMatch: false,
		wantAs:    ErrMalformedState,
	}, {
		name:      "ErrMalformedState != Error.ErrMismatchedState",
		err:       ErrMalformedState,
		target:    makeError(ErrMismatchedState, ""),
		wantMatch: false,
		wantAs:    ErrMalformedState,
	}, {
		name:      "Error.ErrMalformedState != Error.ErrMismatchedState",
		err:       makeError(ErrMalformedState, ""),
		target:    makeError(ErrMismatchedState, ""),
		wantMatch: false,
		wantAs:    ErrMalformedState,
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

// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"errors"
	"testing"
)

// TestErrorKindStringer tests the stringized output for the [ErrorKind] type.
func TestErrorKindStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ErrorKind
		want string
	}{
		{ErrNotVersionMessage, "ErrNotVersionMessage"},
		{ErrSelfConnection, "ErrSelfConnection"},
		{ErrProtocolVerTooOld, "ErrProtocolVerTooOld"},
		{ErrNotVerAckMessage, "ErrNotVerAckMessage"},
		{ErrHandshakeTimeout, "ErrHandshakeTimeout"},
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

// TestErrorKindIsAs ensures both [ErrorKind] and [Error] can be identified as
// being a specific error kind via [errors.Is] and unwrapped via [errors.As].
func TestErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrNotVersionMessage == ErrNotVersionMessage",
		err:       ErrNotVersionMessage,
		target:    ErrNotVersionMessage,
		wantMatch: true,
		wantAs:    ErrNotVersionMessage,
	}, {
		name:      "Error.ErrNotVersionMessage == ErrNotVersionMessage",
		err:       makeError(ErrNotVersionMessage, ""),
		target:    ErrNotVersionMessage,
		wantMatch: true,
		wantAs:    ErrNotVersionMessage,
	}, {
		name:      "ErrNotVersionMessage != ErrSelfConnection",
		err:       ErrNotVersionMessage,
		target:    ErrSelfConnection,
		wantMatch: false,
		wantAs:    ErrNotVersionMessage,
	}, {
		name:      "Error.ErrNotVersionMessage != ErrSelfConnection",
		err:       makeError(ErrNotVersionMessage, ""),
		target:    ErrSelfConnection,
		wantMatch: false,
		wantAs:    ErrNotVersionMessage,
	}, {
		name:      "ErrNotVersionMessage != Error.ErrSelfConnection",
		err:       ErrNotVersionMessage,
		target:    makeError(ErrSelfConnection, ""),
		wantMatch: false,
		wantAs:    ErrNotVersionMessage,
	}, {
		name:      "Error.ErrNotVersionMessage != Error.ErrSelfConnection",
		err:       makeError(ErrNotVersionMessage, ""),
		target:    makeError(ErrSelfConnection, ""),
		wantMatch: false,
		wantAs:    ErrNotVersionMessage,
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

// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"testing"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   ErrorCode
		want string
	}{
		{ErrNTooBig, "ErrNTooBig"},
		{ErrPTooBig, "ErrPTooBig"},
		{ErrMisserialized, "ErrMisserialized"},
		{ErrUnsupportedVersion, "ErrUnsupportedVersion"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(numErrorCodes) {
		t.Errorf("It appears an error code was added without adding an " +
			"associated stringer test")
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result, test.want)
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

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("Error #%d\n got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestIsErrorCode ensures IsErrorCode works as intended.
func TestIsErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrorCode
		want bool
	}{{
		name: "ErrNTooBig testing for ErrNTooBig",
		err:  makeError(ErrNTooBig, ""),
		code: ErrNTooBig,
		want: true,
	}, {
		name: "ErrPTooBig testing for ErrPTooBig",
		err:  makeError(ErrPTooBig, ""),
		code: ErrPTooBig,
		want: true,
	}, {
		name: "ErrMisserialized testing for ErrMisserialized",
		err:  makeError(ErrMisserialized, ""),
		code: ErrMisserialized,
		want: true,
	}, {
		name: "ErrNTooBig error testing for ErrPTooBig",
		err:  makeError(ErrNTooBig, ""),
		code: ErrPTooBig,
		want: false,
	}, {
		name: "ErrNTooBig error testing for unknown error code",
		err:  makeError(ErrNTooBig, ""),
		code: 0xffff,
		want: false,
	}, {
		name: "nil error testing for ErrNTooBig",
		err:  nil,
		code: ErrNTooBig,
		want: false,
	}}
	for _, test := range tests {
		result := IsErrorCode(test.err, test.code)
		if result != test.want {
			t.Errorf("%s: unexpected result -- got: %v want: %v", test.name,
				result, test.want)
			continue
		}
	}
}

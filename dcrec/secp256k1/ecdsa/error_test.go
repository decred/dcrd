// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"errors"
	"testing"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   ErrorCode
		want string
	}{
		{ErrSigTooShort, "ErrSigTooShort"},
		{ErrSigTooLong, "ErrSigTooLong"},
		{ErrSigInvalidSeqID, "ErrSigInvalidSeqID"},
		{ErrSigInvalidDataLen, "ErrSigInvalidDataLen"},
		{ErrSigMissingSTypeID, "ErrSigMissingSTypeID"},
		{ErrSigMissingSLen, "ErrSigMissingSLen"},
		{ErrSigInvalidSLen, "ErrSigInvalidSLen"},
		{ErrSigInvalidRIntID, "ErrSigInvalidRIntID"},
		{ErrSigZeroRLen, "ErrSigZeroRLen"},
		{ErrSigNegativeR, "ErrSigNegativeR"},
		{ErrSigTooMuchRPadding, "ErrSigTooMuchRPadding"},
		{ErrSigRIsZero, "ErrSigRIsZero"},
		{ErrSigRTooBig, "ErrSigRTooBig"},
		{ErrSigInvalidSIntID, "ErrSigInvalidSIntID"},
		{ErrSigZeroSLen, "ErrSigZeroSLen"},
		{ErrSigNegativeS, "ErrSigNegativeS"},
		{ErrSigTooMuchSPadding, "ErrSigTooMuchSPadding"},
		{ErrSigSIsZero, "ErrSigSIsZero"},
		{ErrSigSTooBig, "ErrSigSTooBig"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(numSigErrorCodes) {
		t.Errorf("It appears a signature error code was added without adding " +
			"an associated stringer test")
	}

	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
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

// TestErrorCodeIsAs ensures both ErrorCode and Error can be identified as being
// a specific error code via errors.Is and unwrapped via errors.As.
func TestErrorCodeIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorCode
	}{{
		name:      "ErrSigTooShort == ErrSigTooShort",
		err:       ErrSigTooShort,
		target:    ErrSigTooShort,
		wantMatch: true,
		wantAs:    ErrSigTooShort,
	}, {
		name:      "Error.ErrSigTooShort == ErrSigTooShort",
		err:       signatureError(ErrSigTooShort, ""),
		target:    ErrSigTooShort,
		wantMatch: true,
		wantAs:    ErrSigTooShort,
	}, {
		name:      "ErrSigTooShort == Error.ErrSigTooShort",
		err:       ErrSigTooShort,
		target:    signatureError(ErrSigTooShort, ""),
		wantMatch: true,
		wantAs:    ErrSigTooShort,
	}, {
		name:      "Error.ErrSigTooShort == Error.ErrSigTooShort",
		err:       signatureError(ErrSigTooShort, ""),
		target:    signatureError(ErrSigTooShort, ""),
		wantMatch: true,
		wantAs:    ErrSigTooShort,
	}, {
		name:      "ErrSigTooLong != ErrSigTooShort",
		err:       ErrSigTooLong,
		target:    ErrSigTooShort,
		wantMatch: false,
		wantAs:    ErrSigTooLong,
	}, {
		name:      "Error.ErrSigTooLong != ErrSigTooShort",
		err:       signatureError(ErrSigTooLong, ""),
		target:    ErrSigTooShort,
		wantMatch: false,
		wantAs:    ErrSigTooLong,
	}, {
		name:      "ErrSigTooLong != Error.ErrSigTooShort",
		err:       ErrSigTooLong,
		target:    signatureError(ErrSigTooShort, ""),
		wantMatch: false,
		wantAs:    ErrSigTooLong,
	}, {
		name:      "Error.ErrSigTooLong != Error.ErrSigTooShort",
		err:       signatureError(ErrSigTooLong, ""),
		target:    signatureError(ErrSigTooShort, ""),
		wantMatch: false,
		wantAs:    ErrSigTooLong,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error code can be unwrapped is and is the
		// expected code.
		var code ErrorCode
		if !errors.As(test.err, &code) {
			t.Errorf("%s: unable to unwrap to error code", test.name)
			continue
		}
		if code != test.wantAs {
			t.Errorf("%s: unexpected unwrapped error code -- got %v, want %v",
				test.name, code, test.wantAs)
			continue
		}
	}
}

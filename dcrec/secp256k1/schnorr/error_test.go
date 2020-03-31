// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

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
		{ErrBadInputSize, "ErrBadInputSize"},
		{ErrInputValue, "ErrInputValue"},
		{ErrSchnorrHashValue, "ErrSchnorrHashValue"},
		{ErrPointNotOnCurve, "ErrPointNotOnCurve"},
		{ErrBadSigRYValue, "ErrBadSigRYValue"},
		{ErrBadSigRNotOnCurve, "ErrBadSigRNotOnCurve"},
		{ErrUnequalRValues, "ErrUnequalRValues"},
		{ErrRegenerateRPoint, "ErrRegenerateRPoint"},
		{ErrRegenSig, "ErrRegenSig"},
		{ErrBadNonce, "ErrBadNonce"},
		{ErrZeroSigS, "ErrZeroSigS"},
		{ErrNonmatchingR, "ErrNonmatchingR"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(numErrorCodes) {
		t.Fatalf("It appears an error code was added without adding an " +
			"associated stringer test")
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
		name:      "ErrBadInputSize == ErrBadInputSize",
		err:       ErrBadInputSize,
		target:    ErrBadInputSize,
		wantMatch: true,
		wantAs:    ErrBadInputSize,
	}, {
		name:      "Error.ErrBadInputSize == ErrBadInputSize",
		err:       signatureError(ErrBadInputSize, ""),
		target:    ErrBadInputSize,
		wantMatch: true,
		wantAs:    ErrBadInputSize,
	}, {
		name:      "ErrBadInputSize == Error.ErrBadInputSize",
		err:       ErrBadInputSize,
		target:    signatureError(ErrBadInputSize, ""),
		wantMatch: true,
		wantAs:    ErrBadInputSize,
	}, {
		name:      "Error.ErrBadInputSize == Error.ErrBadInputSize",
		err:       signatureError(ErrBadInputSize, ""),
		target:    signatureError(ErrBadInputSize, ""),
		wantMatch: true,
		wantAs:    ErrBadInputSize,
	}, {
		name:      "ErrInputValue != ErrBadInputSize",
		err:       ErrInputValue,
		target:    ErrBadInputSize,
		wantMatch: false,
		wantAs:    ErrInputValue,
	}, {
		name:      "Error.ErrInputValue != ErrBadInputSize",
		err:       signatureError(ErrInputValue, ""),
		target:    ErrBadInputSize,
		wantMatch: false,
		wantAs:    ErrInputValue,
	}, {
		name:      "ErrInputValue != Error.ErrBadInputSize",
		err:       ErrInputValue,
		target:    signatureError(ErrBadInputSize, ""),
		wantMatch: false,
		wantAs:    ErrInputValue,
	}, {
		name:      "Error.ErrInputValue != Error.ErrBadInputSize",
		err:       signatureError(ErrInputValue, ""),
		target:    signatureError(ErrBadInputSize, ""),
		wantMatch: false,
		wantAs:    ErrInputValue,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error code can be unwrapped and is the expected
		// code.
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

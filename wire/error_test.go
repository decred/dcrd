// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"testing"
)

// TestMessageErrorCodeStringer tests the stringized output for
// the ErrorCode type.
func TestMessageErrorCodeStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ErrorCode
		want string
	}{
		{ErrNonCanonicalVarInt, "ErrNonCanonicalVarInt"},
		{ErrVarStringTooLong, "ErrVarStringTooLong"},
		{ErrVarBytesTooLong, "ErrVarBytesTooLong"},
		{ErrCmdTooLong, "ErrCmdTooLong"},
		{ErrPayloadTooLarge, "ErrPayloadTooLarge"},
		{ErrWrongNetwork, "ErrWrongNetwork"},
		{ErrMalformedCmd, "ErrMalformedCmd"},
		{ErrUnknownCmd, "ErrUnknownCmd"},
		{ErrPayloadChecksum, "ErrPayloadChecksum"},
		{ErrTooManyAddrs, "ErrTooManyAddrs"},
		{ErrTooManyTxs, "ErrTooManyTxs"},
		{ErrMsgInvalidForPVer, "ErrMsgInvalidForPVer"},
		{ErrFilterTooLarge, "ErrFilterTooLarge"},
		{ErrTooManyProofs, "ErrTooManyProofs"},
		{ErrTooManyFilterTypes, "ErrTooManyFilterTypes"},
		{ErrTooManyLocators, "ErrTooManyLocators"},
		{ErrTooManyVectors, "ErrTooManyVectors"},
		{ErrTooManyHeaders, "ErrTooManyHeaders"},
		{ErrHeaderContainsTxs, "ErrHeaderContainsTxs"},
		{ErrTooManyVotes, "ErrTooManyVotes"},
		{ErrTooManyBlocks, "ErrTooManyBlocks"},
		{ErrMismatchedWitnessCount, "ErrMismatchedWitnessCount"},
		{ErrUnknownTxType, "ErrUnknownTxType"},
		{ErrReadInPrefixFromWitnessOnlyTx, "ErrReadInPrefixFromWitnessOnlyTx"},
		{ErrInvalidMsg, "ErrInvalidMsg"},
		{ErrUserAgentTooLong, "ErrUserAgentTooLong"},
		{ErrTooManyFilterHeaders, "ErrTooManyFilterHeaders"},
		{ErrMalformedStrictString, "ErrMalformedStrictString"},
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

// TestMessageError tests the error output for the MessageError type.
func TestMessageError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   MessageError
		want string
	}{{
		MessageError{Description: "some error"},
		"some error",
	}, {
		MessageError{Description: "human-readable error"},
		"human-readable error",
	}, {
		MessageError{Func: "foo", Description: "something bad happened"},
		"foo: something bad happened",
	}}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestErrorCodeIsAs ensures both ErrorCode and MessageError can be identified
// as being a specific error code via errors.Is and unwrapped via errors.As.
func TestErrorCodeIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorCode
	}{{
		name:      "ErrTooManyAddrs == ErrTooManyAddrs",
		err:       ErrTooManyAddrs,
		target:    ErrTooManyAddrs,
		wantMatch: true,
		wantAs:    ErrTooManyAddrs,
	}, {
		name:      "MessageError.ErrTooManyAddrs == ErrTooManyAddrs",
		err:       messageError("", ErrTooManyAddrs, ""),
		target:    ErrTooManyAddrs,
		wantMatch: true,
		wantAs:    ErrTooManyAddrs,
	}, {
		name:      "ErrTooManyAddrs == MessageError.ErrTooManyAddrs",
		err:       ErrTooManyAddrs,
		target:    messageError("", ErrTooManyAddrs, ""),
		wantMatch: true,
		wantAs:    ErrTooManyAddrs,
	}, {
		name:      "MessageError.ErrTooManyAddrs == MessageError.ErrTooManyAddrs",
		err:       messageError("", ErrTooManyAddrs, ""),
		target:    messageError("", ErrTooManyAddrs, ""),
		wantMatch: true,
		wantAs:    ErrTooManyAddrs,
	}, {
		name:      "ErrTooManyTxs != ErrTooManyAddrs",
		err:       ErrTooManyTxs,
		target:    ErrTooManyAddrs,
		wantMatch: false,
		wantAs:    ErrTooManyTxs,
	}, {
		name:      "MessageError.ErrTooManyTxs != ErrTooManyAddrs",
		err:       messageError("", ErrTooManyTxs, ""),
		target:    ErrTooManyAddrs,
		wantMatch: false,
		wantAs:    ErrTooManyTxs,
	}, {
		name:      "ErrTooManyTxs != MessageError.ErrTooManyAddrs",
		err:       ErrTooManyTxs,
		target:    messageError("", ErrTooManyAddrs, ""),
		wantMatch: false,
		wantAs:    ErrTooManyTxs,
	}, {
		name:      "MessageError.ErrTooManyTxs != MessageError.ErrTooManyAddrs",
		err:       messageError("", ErrTooManyTxs, ""),
		target:    messageError("", ErrTooManyAddrs, ""),
		wantMatch: false,
		wantAs:    ErrTooManyTxs,
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

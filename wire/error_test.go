// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
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
		"Unknown ErrorCode (0): some error",
	}, {
		MessageError{ErrorCode: ErrNonCanonicalVarInt,
			Description: "human-readable error"},
		"ErrNonCanonicalVarInt: human-readable error",
	}, {
		MessageError{ErrorCode: ErrMsgInvalidForPVer,
			Description: "something bad happened"},
		"ErrMsgInvalidForPVer: something bad happened",
	}, {
		MessageError{Func: "foo", ErrorCode: ErrMsgInvalidForPVer,
			Description: "something bad happened"},
		"foo: [ErrMsgInvalidForPVer] something bad happened",
	}}

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

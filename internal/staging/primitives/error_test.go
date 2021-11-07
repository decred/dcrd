// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

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
		{ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{ErrHighHash, "ErrHighHash"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestRuleError tests the error output for the RuleError type.
func TestRuleError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   RuleError
		want string
	}{{
		RuleError{Description: "some error"},
		"some error",
	}, {
		RuleError{Description: "human-readable error"},
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

// TestErrorKindIsAs ensures both ErrorKind and RuleError can be identified as
// being a specific error kind via errors.Is and unwrapped via errors.As.
func TestErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrUnexpectedDifficulty == ErrUnexpectedDifficulty",
		err:       ErrUnexpectedDifficulty,
		target:    ErrUnexpectedDifficulty,
		wantMatch: true,
		wantAs:    ErrUnexpectedDifficulty,
	}, {
		name:      "RuleError.ErrUnexpectedDifficulty == ErrUnexpectedDifficulty",
		err:       ruleError(ErrUnexpectedDifficulty, ""),
		target:    ErrUnexpectedDifficulty,
		wantMatch: true,
		wantAs:    ErrUnexpectedDifficulty,
	}, {
		name:      "ErrHighHash != ErrUnexpectedDifficulty",
		err:       ErrHighHash,
		target:    ErrUnexpectedDifficulty,
		wantMatch: false,
		wantAs:    ErrHighHash,
	}, {
		name:      "RuleError.ErrHighHash != ErrUnexpectedDifficulty",
		err:       ruleError(ErrHighHash, ""),
		target:    ErrUnexpectedDifficulty,
		wantMatch: false,
		wantAs:    ErrHighHash,
	}, {
		name:      "ErrHighHash != RuleError.ErrUnexpectedDifficulty",
		err:       ErrHighHash,
		target:    ruleError(ErrUnexpectedDifficulty, ""),
		wantMatch: false,
		wantAs:    ErrHighHash,
	}, {
		name:      "RuleError.ErrHighHash != RuleError.ErrUnexpectedDifficulty",
		err:       ruleError(ErrHighHash, ""),
		target:    ruleError(ErrUnexpectedDifficulty, ""),
		wantMatch: false,
		wantAs:    ErrHighHash,
	}, {
		name:      "RuleError.ErrHighHash != io.EOF",
		err:       ruleError(ErrHighHash, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrHighHash,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error kind can be unwrapped is and is the
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

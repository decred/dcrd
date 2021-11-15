// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/decred/dcrd/txscript/v4"
)

var (
	// tokenRE is a regular expression used to parse tokens from short form
	// scripts.  It splits on repeated tokens and spaces.  Repeated tokens are
	// denoted by being wrapped in angular brackets followed by a suffix which
	// consists of a number inside braces.
	tokenRE = regexp.MustCompile(`\<.+?\>\{[0-9]+\}|[^\s]+`)

	// repTokenRE is a regular expression used to parse short form scripts for a
	// series of tokens repeated a specified number of times.
	repTokenRE = regexp.MustCompile(`^\<(.+)\>\{([0-9]+)\}$`)

	// repRawRE is a regular expression used to parse short form scripts for raw
	// data that is to be repeated a specified number of times.
	repRawRE = regexp.MustCompile(`^(0[xX][0-9a-fA-F]+)\{([0-9]+)\}$`)

	// repQuoteRE is a regular expression used to parse short form scripts for
	// quoted data that is to be repeated a specified number of times.
	repQuoteRE = regexp.MustCompile(`^'(.*)'\{([0-9]+)\}$`)
)

// shortFormOps holds a map of opcode names to values for use in short form
// parsing.  It is declared here so it only needs to be created once.
var shortFormOps map[string]byte

// parseHex parses a hex string token into raw bytes.
func parseHex(tok string) ([]byte, error) {
	if !strings.HasPrefix(tok, "0x") {
		return nil, errors.New("not a hex number")
	}
	return hex.DecodeString(tok[2:])
}

// parseShortFormV0 parses a version 0 script from a human-readable format that
// allows for convenient testing into the associated raw script bytes.
//
// The format used is as follows:
//   - Opcodes other than the push opcodes and unknown are present as either
//     OP_NAME or just NAME
//   - Plain numbers are made into push operations
//   - Numbers beginning with 0x are inserted into the []byte without
//     modification (so 0x14 is OP_DATA_20)
//   - Numbers beginning with 0x which have a suffix which consists of a number
//     in braces (e.g. 0x6161{10}) repeat the raw bytes the specified number of
//     times and are inserted without modification
//   - Single quoted strings are pushed as data
//   - Single quoted strings that have a suffix which consists of a number in
//     braces (e.g. 'b'{10}) repeat the data the specified number of times and
//     are pushed as a single data push
//   - Tokens inside of angular brackets with a suffix which consists of a
//     number in braces (e.g. <0 0 CHECKMULTSIG>{5}) is parsed as if the tokens
//     inside the angular brackets were manually repeated the specified number
//     of times
//   - Anything else is an error
func parseShortFormV0(script string) ([]byte, error) {
	// Only create the short form opcode map once.
	if shortFormOps == nil {
		ops := make(map[string]byte)
		for opcodeName, opcodeValue := range txscript.OpcodeByName {
			if strings.Contains(opcodeName, "OP_UNKNOWN") {
				continue
			}
			ops[opcodeName] = opcodeValue

			// The opcodes named OP_# can't have the OP_ prefix stripped or they
			// would conflict with the plain numbers.  Also, since OP_FALSE and
			// OP_TRUE are aliases for the OP_0, and OP_1, respectively, they
			// have the same value, so detect those by name and allow them.
			if (opcodeName == "OP_FALSE" || opcodeName == "OP_TRUE") ||
				(opcodeValue != txscript.OP_0 && (opcodeValue < txscript.OP_1 ||
					opcodeValue > txscript.OP_16)) {

				ops[strings.TrimPrefix(opcodeName, "OP_")] = opcodeValue
			}
		}
		shortFormOps = ops
	}

	builder := txscript.NewScriptBuilder()

	var handleToken func(tok string) error
	handleToken = func(tok string) error {
		// Multiple repeated tokens.
		if m := repTokenRE.FindStringSubmatch(tok); m != nil {
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			tokens := tokenRE.FindAllStringSubmatch(m[1], -1)
			for i := 0; i < int(count); i++ {
				for _, t := range tokens {
					if err := handleToken(t[0]); err != nil {
						return err
					}
				}
			}
			return nil
		}

		// Plain number.
		if num, err := strconv.ParseInt(tok, 10, 64); err == nil {
			builder.AddInt64(num)
			return nil
		}

		// Raw data.
		if bts, err := parseHex(tok); err == nil {
			// Use the unchecked variant since the test code intentionally
			// creates scripts that are too large and would cause the builder to
			// error otherwise.
			builder.AddOpsUnchecked(bts)
			return nil
		}

		// Repeated raw bytes.
		if m := repRawRE.FindStringSubmatch(tok); m != nil {
			bts, err := parseHex(m[1])
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}

			// Use the unchecked variant since the test code intentionally
			// creates scripts that are too large and would cause the builder to
			// error otherwise.
			bts = bytes.Repeat(bts, int(count))
			builder.AddOpsUnchecked(bts)
			return nil
		}

		// Quoted data.
		if len(tok) >= 2 && tok[0] == '\'' && tok[len(tok)-1] == '\'' {
			builder.AddDataUnchecked([]byte(tok[1 : len(tok)-1]))
			return nil
		}

		// Repeated quoted data.
		if m := repQuoteRE.FindStringSubmatch(tok); m != nil {
			count, err := strconv.ParseInt(m[2], 10, 32)
			if err != nil {
				return fmt.Errorf("bad token %q", tok)
			}
			data := strings.Repeat(m[1], int(count))
			builder.AddDataUnchecked([]byte(data))
			return nil
		}

		// Named opcode.
		if opcode, ok := shortFormOps[tok]; ok {
			builder.AddOp(opcode)
			return nil
		}

		return fmt.Errorf("bad token %q", tok)
	}

	for _, tokens := range tokenRE.FindAllStringSubmatch(script, -1) {
		if err := handleToken(tokens[0]); err != nil {
			return nil, err
		}
	}
	return builder.Script()
}

// parseShortForm parses a script from a human-readable format for the given
// script version that allows for convenient testing into the associated raw
// script bytes.
//
// See the associated version-specific short form parsing function for each
// version for details regarding the format since it may or may not differ
// between script version.
func parseShortForm(scriptVersion uint16, script string) ([]byte, error) {
	switch scriptVersion {
	case 0:
		return parseShortFormV0(script)
	}

	str := fmt.Sprintf("parsing short form for version %d scripts is not "+
		"supported", scriptVersion)
	return nil, makeError(ErrUnsupportedScriptVersion, str)
}

// mustParseShortForm parses the passed short form script and returns the
// resulting bytes.  It panics if an error occurs.  This is only used in the
// tests as a helper since the only way it can fail is if there is an error in
// the test source code.
func mustParseShortForm(scriptVersion uint16, script string) []byte {
	s, err := parseShortForm(scriptVersion, script)
	if err != nil {
		panic("invalid short form script in test source: err " + err.Error() +
			", script: " + script)
	}

	return s
}

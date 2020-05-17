// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

func TestVerifyMessage(t *testing.T) {
	msg := "verifymessage test"

	var tests = []struct {
		addr    string
		sig     string
		params  AddressParams
		isValid bool
	}{
		// valid
		{"TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv", "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=", chaincfg.TestNet3Params(), true},

		// wrong address
		{"TsWeG3TJzucZgYyMfZFC2GhBvbeNfA48LTo", "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=", chaincfg.TestNet3Params(), false},

		// wrong signature
		{"TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv", "HxzZggzHMljSWpKHnw1Dow84KGWvTRBCG2JqBM5W4Q7iePW0dirZXCggSeXHVQ26D0MbDFffi3yw+x2Z5nQ94gg=", chaincfg.TestNet3Params(), false},

		// wrong params
		{"TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv", "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=", chaincfg.MainNetParams(), false},
	}

	for i, test := range tests {
		err := VerifyMessage(test.addr, test.sig, msg, test.params)
		if (test.isValid && err != nil) || (!test.isValid && err == nil) {
			t.Fatalf("VerifyMessage test #%d failed", i+1)
		}
	}
}

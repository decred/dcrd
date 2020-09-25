// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"testing"
)

// TestVerifyMessage ensures the verifying a message works as intended.
func TestVerifyMessage(t *testing.T) {
	mainNetParams := mockMainNetParams()
	testNetParams := mockTestNetParams()

	msg := "verifymessage test"

	var tests = []struct {
		name    string
		addr    string
		sig     string
		params  AddressParams
		isValid bool
	}{{
		name:    "valid",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  testNetParams,
		isValid: true,
	}, {
		name:    "wrong address",
		addr:    "TsWeG3TJzucZgYyMfZFC2GhBvbeNfA48LTo",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  testNetParams,
		isValid: false,
	}, {
		name:    "wrong signature",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "HxzZggzHMljSWpKHnw1Dow84KGWvTRBCG2JqBM5W4Q7iePW0dirZXCggSeXHVQ26D0MbDFffi3yw+x2Z5nQ94gg=",
		params:  testNetParams,
		isValid: false,
	}, {
		name:    "wrong params",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  mainNetParams,
		isValid: false,
	}}

	for _, test := range tests {
		err := VerifyMessage(test.addr, test.sig, msg, test.params)
		if (test.isValid && err != nil) || (!test.isValid && err == nil) {
			t.Fatalf("%s: failed", test.name)
		}
	}
}

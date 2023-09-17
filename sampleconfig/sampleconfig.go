// Copyright (c) 2017-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sampleconfig

import (
	_ "embed"
)

// sampleDcrdConf is a string containing the commented example config for dcrd.
//
//go:embed sample-dcrd.conf
var sampleDcrdConf string

// sampleDcrctlConf is a string containing the commented example config for
// dcrctl.
//
//go:embed sample-dcrctl.conf
var sampleDcrctlConf string

// Dcrd returns a string containing the commented example config for dcrd.
func Dcrd() string {
	return sampleDcrdConf
}

// FileContents returns a string containing the commented example config for
// dcrd.
//
// Deprecated: Use the [Dcrd] function instead.
func FileContents() string {
	return Dcrd()
}

// Dcrctl returns a string containing the commented example config for dcrctl.
func Dcrctl() string {
	return sampleDcrctlConf
}

// DcrctlSampleConfig is a string containing the commented example config for
// dcrctl.
//
// Deprecated: Use the [Dcrctl] function instead.
var DcrctlSampleConfig = Dcrctl()

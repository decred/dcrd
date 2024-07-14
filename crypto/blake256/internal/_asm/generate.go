// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Generation code originally written by Dave Collins July 2024.

//go:generate go run gen_amd64_compress_asm.go -out ../compress/blocks_amd64.s -stubs ../compress/blocks_amd64.go -pkg compress

package main

import (
	_ "github.com/mmcloughlin/avo/build"
)

// State houses the current chain value and salt used during block compression.
//
// See the definition in the internal compress package for more details.
type State struct {
	CV [8]uint32 // the current chain value
	S  [4]uint32 // salt (zero by default)
}

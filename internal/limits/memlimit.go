// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build go1.19
// +build go1.19

package limits

import "runtime/debug"

// SupportsMemoryLimit indicates that a runtime enforced soft memory limit is
// supported starting with Go 1.19.
const SupportsMemoryLimit = true

// SetMemoryLimit configures the runtime to use the provided limit as a soft
// memory limit starting with Go 1.19.
func SetMemoryLimit(limit int64) {
	debug.SetMemoryLimit(limit)
}

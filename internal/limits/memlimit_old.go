// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !go1.19
// +build !go1.19

package limits

// SupportsMemoryLimit indicates that a runtime enforced soft memory limit is
// not supported for versions of Go prior to version 1.19.
const SupportsMemoryLimit = false

// SetMemoryLimit is a no-op on versions of Go prior to version 1.19 since the
// the ability is not supported on those versions.
func SetMemoryLimit(_ int64) {
}

// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !(386 || amd64) || purego

package cpufeat

// detect returns false for all features by default.
func detect() Features {
	return Features{}
}

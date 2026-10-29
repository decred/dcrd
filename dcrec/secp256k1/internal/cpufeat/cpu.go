// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package cpufeat provides detection of CPU support for relevant hardware
// features.
package cpufeat

// features caches the result of querying the CPU for supported features.
var features = detect()

// Supported returns the feature set detected during package initialization.
func Supported() Features {
	return features
}

// Features houses flags that specify whether or not various features
// are supported by the CPU.
type Features struct {
	BMI2 bool
	ADX  bool
}

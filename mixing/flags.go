// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

const (
	// PRFlagCanSolveRoots describes a bit in the pair request flags field
	// indicating support for solving and publishing factored slot
	// reservation polynomials.
	PRFlagCanSolveRoots byte = 1 << iota
)

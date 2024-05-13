package mixing

const (
	// PRFlagCanSolveRoots describes a bit in the pair request flags field
	// indicating support for solving and publishing factored slot
	// reservation polynomials.
	PRFlagCanSolveRoots byte = 1 << iota
)

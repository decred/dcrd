package types

// EstimateSmartFeeModeAddr is a helper routine that allocates a new
// EstimateSmartFeeMode value to store v and returns a pointer to it. This is
// useful when assigning optional parameters.
func EstimateSmartFeeModeAddr(v EstimateSmartFeeMode) *EstimateSmartFeeMode {
	p := new(EstimateSmartFeeMode)
	*p = v
	return p
}

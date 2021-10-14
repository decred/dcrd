// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
)

// calcRateKey is used as the key when caching results for the false positive
// and query access calculations.
type calcRateKey struct {
	a, i uint16
}

// calcFPRateInternal calculates and returns the false positive rate for the
// provided parameters using the given results map to cache intermediate
// results.
func calcFPRateInternal(results map[calcRateKey]float64, k, l uint8, a, i uint16) float64 {
	// The false positive rate is calculated according to the following
	// recursively-defined function provided in [APBF]:
	//
	//              {1                                        , if a = k
	// F(k,l,a,i) = {0                                        , if i > l + a
	//              {(r_i)F(k,l,a+1,i+1) + (1-r_i)F(k,l,0,i+1), otherwise

	if a == uint16(k) {
		return 1
	} else if i > uint16(l)+a {
		return 0
	}

	// Return stored results to avoid a bunch of duplicate work.
	resultKey := calcRateKey{a, i}
	if result, ok := results[resultKey]; ok {
		return result
	}

	// Calculate the fill ratio for the slice.
	fillRatio := float64(0.5)
	if i < uint16(k) {
		fillRatio = float64(i+1) / float64(2*k)
	}

	firstTerm := fillRatio * calcFPRateInternal(results, k, l, a+1, i+1)
	secondTerm := (1 - fillRatio) * calcFPRateInternal(results, k, l, 0, i+1)
	result := firstTerm + secondTerm
	results[resultKey] = result
	return result
}

// calcFPRate calculates and returns the false positive rate for an APBF
// created with the given parameters.
func calcFPRate(k, l uint8) float64 {
	results := make(map[calcRateKey]float64, 2*uint16(k)*uint16(l))
	return calcFPRateInternal(results, k, l, 0, 0)
}

// calcAvgAccessFalseInternal calculates and returns the average number of
// accesses for false queries for the provided parameters using the given
// results map to cache intermediate results.
func calcAvgAccessFalseInternal(results map[calcRateKey]float64, k, l uint8, p, c, a uint16, i int16) float64 {
	// The average expected number of accesses for false queries is calculated
	// according to the following recursively-defined function provided in
	// [APBF]:
	//
	//                  {a                                        , if i < 0
	// A(k,l,p,c,a,i) = {0                                        , if p + c = k
	//                  {(r_i)A(k,l,p,c+1,a+1,i+1) +
	//                   (1-r_i)A(k,l,c,0,a+1,i-k)                , otherwise
	//

	if i < 0 {
		return float64(a)
	} else if p+c == uint16(k) {
		return 0
	}

	// Return stored results to avoid a bunch of duplicate work.
	resultKey := calcRateKey{a, uint16(i)}
	if result, ok := results[resultKey]; ok {
		return result
	}

	// Calculate the fill ratio for the slice.
	fillRatio := float64(0.5)
	if i < int16(k) {
		fillRatio = float64(i+1) / float64(2*k)
	}

	firstTerm := fillRatio
	firstTerm *= calcAvgAccessFalseInternal(results, k, l, p, c+1, a+1, i+1)
	secondTerm := (1 - fillRatio)
	secondTerm *= calcAvgAccessFalseInternal(results, k, l, c, 0, a+1, i-int16(k))
	result := firstTerm + secondTerm
	results[resultKey] = result
	return result
}

// calcAvgAccessFalse calculates and returns the average number of accesses for
// false queries for an APBF created with the given parameters.
func calcAvgAccessFalse(k, l uint8) float64 {
	results := make(map[calcRateKey]float64, 2*uint16(k)*uint16(l))
	return calcAvgAccessFalseInternal(results, k, l, 0, 0, 0, int16(l)) /
		(1 - calcFPRate(k, l))
}

func main() {
	fi, err := os.Create("apbftable.md")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer fi.Close()
	p := func(format string, args ...interface{}) {
		fmt.Fprintf(fi, format, args...)
	}

	p("## Age-Partitioned Bloom Filter Parameter Metrics\n")
	for k := uint8(1); k <= 50; k++ {
		p("\n")
		p("### k=%d\n\n", k)
		p(" k  |  l  |  fprate  | query accesses (false result)\n")
		p("----|-----|----------|------------------------------\n")
		for l := uint8(1); l <= 100; l++ {
			fp := calcFPRate(k, l)

			// Ignore entries that essentially have a 100% fprate.
			if 1-fp < 0.001 {
				continue
			}
			accesses := calcAvgAccessFalse(k, l)

			if fp > 10e-6 {
				p(" %2d | %3d | %0.6f | %0.2f\n", k, l, fp, accesses)
			} else {
				p(" %2d | %3d | %0.3g | %0.2f\n", k, l, fp, accesses)
			}
		}
	}
}

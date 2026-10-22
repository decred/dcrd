// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

package field4x64

import (
	"testing"
)

// BenchmarkMulReduceAMD64 benchmarks how long it takes to multiply two field
// elements together and reduce them modulo the field prime with each of the
// specialized amd64 implementations along with the number of allocations
// needed.
func BenchmarkMulReduceAMD64(b *testing.B) {
	benches := []struct {
		name      string
		fn        func(r *[4]uint64, a, b *[4]uint64)
		supported bool
	}{
		{name: "Generic", fn: mulReduceGeneric, supported: true},
		{name: "BMI2/ADX", fn: mulReduceADX, supported: useBMI2AndADX},
	}

	a := mustElement("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab").n
	c := mustElement("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca").n
	var r [4]uint64

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			if !bench.supported {
				// Note that this is intentionally not using b.Skipf because
				// tinygo does not support it.
				b.Logf("Skipping %s bench (disabled or no instruction set "+
					"support)", bench.name)
				return
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.fn(&r, &a, &c)
			}
		})
	}
}

// BenchmarkSquareReduceAMD64 benchmarks how long it takes to square a field
// element and reduce it modulo the field prime with each of the specialized
// amd64 implementations along with the number of allocations needed.
func BenchmarkSquareReduceAMD64(b *testing.B) {
	benches := []struct {
		name      string
		fn        func(r *[4]uint64, a *[4]uint64)
		supported bool
	}{
		{name: "Generic", fn: squareReduceGeneric, supported: true},
		{name: "BMI2/ADX", fn: squareReduceADX, supported: useBMI2AndADX},
	}

	a := mustElement("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca").n
	var r [4]uint64

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			if !bench.supported {
				// Note that this is intentionally not using b.Skipf because
				// tinygo does not support it.
				b.Logf("Skipping %s bench (disabled or no instruction set "+
					"support)", bench.name)
				return
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.fn(&r, &a)
			}
		})
	}
}

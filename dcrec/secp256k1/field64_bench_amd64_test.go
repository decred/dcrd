// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 && !purego

package secp256k1

import (
	"testing"
)

func BenchmarkField64MulAMD64(b *testing.B) {
	a := mustFieldVal64("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab").n
	c := mustFieldVal64("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca").n
	var r [4]uint64

	b.Run("Generic", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			field64MulGeneric(&r, &a, &c)
		}
	})
	if field64UseADX {
		b.Run("MULX/ADX", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				field64MulADX(&r, &a, &c)
			}
		})
	} else {
		b.Log("Skipping MULX/ADX bench (disabled or no instruction set support)")
	}
}

func BenchmarkField64SquareAMD64(b *testing.B) {
	a := mustFieldVal64("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca").n
	var r [4]uint64

	b.Run("Generic", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			field64SquareGeneric(&r, &a)
		}
	})
	if field64UseADX {
		b.Run("MULX/ADX", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				field64SquareADX(&r, &a)
			}
		})
	} else {
		b.Log("Skipping MULX/ADX bench (disabled or no instruction set support)")
	}
}

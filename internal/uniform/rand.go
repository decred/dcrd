// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package uniform provides uniform, cryptographically secure random numbers
// with randomness obtained from a crypto/rand.Reader or other CSPRNG reader.
//
// Random sources are required to never error; any errors reading the random
// source will result in a panic.
package uniform

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"time"
)

func read(rand io.Reader, buf []byte) {
	_, err := io.ReadFull(rand, buf)
	if err != nil {
		panic(fmt.Errorf("uniform: read of random source errored: %w", err))
	}
}

// Uint32 returns a uniform random uint32.
func Uint32(rand io.Reader) uint32 {
	b := make([]byte, 4)
	read(rand, b)
	return binary.LittleEndian.Uint32(b)
}

// Uint32n returns a random uint32 in range [0,n) without modulo bias.
// Panics if n == 0.
func Uint32n(rand io.Reader, n uint32) uint32 {
	if n == 0 {
		panic("uniform: invalid argument to Uint32n")
	}
	if n == 1 {
		return 0
	}
	n--
	mask := ^uint32(0) >> bits.LeadingZeros32(n)
	for {
		v := Uint32(rand) & mask
		if v <= n {
			return v
		}
	}
}

// Uint64 returns a uniform random uint64.
func Uint64(rand io.Reader) uint64 {
	b := make([]byte, 8)
	read(rand, b)
	return binary.LittleEndian.Uint64(b)
}

// Uint64n returns a random uint32 in range [0,n) without modulo bias.
// Panics if n == 0.
func Uint64n(rand io.Reader, n uint64) uint64 {
	if n == 0 {
		panic("uniform: invalid argument to Uint64n")
	}
	if n == 1 {
		return 0
	}
	n--
	mask := ^uint64(0) >> bits.LeadingZeros64(n)
	for {
		v := Uint64(rand) & mask
		if v <= n {
			return v
		}
	}
}

// Int32 returns a random 31-bit non-negative integer as an int32 without
// modulo bias.
func Int32(rand io.Reader) int32 {
	return int32(Uint32(rand) & 0x7FFFFFFF)
}

// Int32n returns, as an int32, a random 31-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int32n(rand io.Reader, n int32) int32 {
	if n <= 0 {
		panic("uniform: invalid argument to Int32n")
	}
	return int32(Uint32n(rand, uint32(n)))
}

// Int64 returns a random 63-bit non-negative integer as an int64 without
// modulo bias.
func Int64(rand io.Reader) int64 {
	return int64(Uint64(rand) & 0x7FFFFFFF_FFFFFFFF)
}

// Int64n returns, as an int64, a random 63-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int64n(rand io.Reader, n int64) int64 {
	if n <= 0 {
		panic("uniform: invalid argument to Int64n")
	}
	return int64(Uint64n(rand, uint64(n)))
}

// Duration returns a random duration in [0,n).
// Panics if n <= 0.
func Duration(rand io.Reader, n time.Duration) time.Duration {
	return time.Duration(Int64n(rand, int64(n)))
}

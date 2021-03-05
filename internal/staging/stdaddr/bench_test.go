// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdaddr

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

// BenchmarkDecode benchmarks the performance of decoding various types of
// addresses.
func BenchmarkDecode(b *testing.B) {
	mainNetParams := chaincfg.MainNetParams()

	benches := []struct {
		name   string // benchmark name
		addr   string // address to decode
		params AddressParams
	}{{
		name:   "v0 p2sh",
		addr:   "DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-ecdsa-secp256k1",
		addr:   "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-ed25519",
		addr:   "DeeUhrRoTp4DftsqddVW96yMGMW4sgQFYUE",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-schnorr-secp256k1",
		addr:   "DSXcZv4oSRiEoWL2a9aD8sgfptRo1YEXNKj",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-ecdsa-secp256k1",
		addr:   "DkM3ZigNyiwHrsXRjkDQ8t8tW6uKGW9g61qEkG3bMqQPQWYEf5X3J",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-ed25519",
		addr:   "DkM5zR8tqWNAHngZQDTyAeqzabZxMKrkSbCFULDhmvySn3uHmm221",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-schnorr-secp256k1",
		addr:   "DkM7TD2qsne9DKo4uA2ZNt3XhejYVwT5mmQWtUXtjdPhRHXTSKxN4",
		params: mainNetParams,
	}}

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := DecodeAddress(bench.addr, bench.params)
				if err != nil {
					b.Fatalf("%q: unexpected error: %v", bench.name, err)
				}
			}
		})
	}
}

// BenchmarkPaymentScript benchmarks the performance of generating payment
// scripts for various types of addresses.
func BenchmarkPaymentScript(b *testing.B) {
	mainNetParams := chaincfg.MainNetParams()

	benches := []struct {
		name   string // benchmark name
		addr   string // address to decode
		params AddressParams
	}{{
		name:   "v0 p2sh",
		addr:   "DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-ecdsa-secp256k1",
		addr:   "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-ed25519",
		addr:   "DeeUhrRoTp4DftsqddVW96yMGMW4sgQFYUE",
		params: mainNetParams,
	}, {
		name:   "v0 p2pkh-schnorr-secp256k1",
		addr:   "DSXcZv4oSRiEoWL2a9aD8sgfptRo1YEXNKj",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-ecdsa-secp256k1",
		addr:   "DkM3ZigNyiwHrsXRjkDQ8t8tW6uKGW9g61qEkG3bMqQPQWYEf5X3J",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-ed25519",
		addr:   "DkM5zR8tqWNAHngZQDTyAeqzabZxMKrkSbCFULDhmvySn3uHmm221",
		params: mainNetParams,
	}, {
		name:   "v0 p2pk-schnorr-secp256k1",
		addr:   "DkM7TD2qsne9DKo4uA2ZNt3XhejYVwT5mmQWtUXtjdPhRHXTSKxN4",
		params: mainNetParams,
	}}

	for _, bench := range benches {
		addr, err := DecodeAddress(bench.addr, bench.params)
		if err != nil {
			b.Fatalf("%q: unexpected error: %v", bench.name, err)
		}
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = addr.PaymentScript()
			}
		})
	}
}

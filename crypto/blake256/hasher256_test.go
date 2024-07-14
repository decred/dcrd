// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// BLAKE-256 tests originally written by Dave Collins May 2020.

package blake256

import (
	"encoding/hex"
	"errors"
	"testing"
)

// TestHasher256Vectors ensures the BLAKE-256 hasher computes the correct hash
// for all of the known-good vectors.
func TestHasher256Vectors(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		hasher := NewHasher256()
		if test.salt != nil {
			hasher = NewHasher256Salt(test.salt)
		}
		hasher.WriteBytes(test.data)
		hash := hasher.Sum256()
		result := hex.EncodeToString(hash[:])
		if result != test.hash256 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash256)
			continue
		}
	}
}

// TestHasher256VectorsMultiWrite ensures the BLAKE-256 hasher computes the
// correct hash for all of the known-good vectors when writing the data in
// multiple independent calls.
func TestHasher256VectorsMultiWrite(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		hasher := NewHasher256()
		if test.salt != nil {
			hasher = NewHasher256Salt(test.salt)
		}
		if l := len(test.data); l >= 3 {
			hasher.WriteBytes(test.data[:l/3])
			hasher.WriteBytes(test.data[l/3 : 2*l/3])
			hasher.WriteBytes(test.data[2*l/3:])
		} else {
			hasher.Write(test.data)
		}
		hash := hasher.Sum256()
		result := hex.EncodeToString(hash[:])
		if result != test.hash256 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash256)
			continue
		}
	}
}

// TestHasher256HashInterface ensures the BLAKE-256 hasher correctly implements
// the [hash.Hash] interface.
func TestHasher256HashInterface(t *testing.T) {
	t.Parallel()

	// Ensure the expected block size is returned.
	hasher := New()
	if blockSize := hasher.BlockSize(); blockSize != BlockSize {
		t.Fatalf("mismatched block size: got %d, want %d", blockSize, BlockSize)
	}

	// Ensure the expected hash size is returned.
	if hashSize := hasher.Size(); hashSize != Size {
		t.Fatalf("mismatched block size: got %d, want %d", hashSize, Size)
	}

	// Ensure the hasher computes the correct hash for all of the known-good
	// vectors using the [hash.Hash] interface.
	sum := make([]byte, Size)
	for _, test := range hasherVecTests {
		hasher = New()
		if test.salt != nil {
			hasher = NewSalt(test.salt)
		}
		hasher.Write(test.data)
		result := hex.EncodeToString(hasher.Sum(sum[:0]))
		if result != test.hash256 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash256)
			continue
		}
	}
}

// TestHasher256Write ensures the various write methods of the BLAKE-256 hasher
// work as intended.
func TestHasher256Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string      // test description
		val  interface{} // value to write
		want string      // expected hash
	}{{
		name: "byte",
		val:  byte(0x0a),
		want: "e81db4f0d76e02805155441f50c861a8f86374f3ae34c7a3ff4111d3a634ecb1",
	}, {
		name: "uint16",
		val:  uint16(0x0102),
		want: "544f959e2d8a82938cc83f507d1168139789cb0a2df2250ac1a847b672028835",
	}, {
		name: "uint32",
		val:  uint32(0x01020304),
		want: "47d2a93d821d2aec5969537a72b0a9dc48ff8b6caedbd65acc4a95791af54575",
	}, {
		name: "uint64",
		val:  uint64(0x0102030405060708),
		want: "9b20fcbc30b63ae0ce02acefea7098a80bbd812e5020844e2511c08e5441427a",
	}, {
		name: "bytes",
		val:  []byte{0x01, 0x02, 0x03},
		want: "6d65d219f82e93a3fe008f4e26e6450fdc97673f0907b9f8bc989ed19539702f",
	}, {
		name: "string",
		val:  "abc",
		want: "1833a9fa7cf4086bd5fda73da32e5a1d75b4c3f89d5c436369f9d78bb2da5c28",
	}}

	hasher := NewHasher256()
	for _, test := range tests {
		// Use the correct func depending on the value type.  This also doubles
		// to ensure Reset works properly.
		hasher.Reset()
		switch val := test.val.(type) {
		case byte:
			hasher.WriteByte(val)
		case uint16:
			hasher.WriteUint16LE(val)
			hasher.WriteUint16BE(val)
		case uint32:
			hasher.WriteUint32LE(val)
			hasher.WriteUint32BE(val)
		case uint64:
			hasher.WriteUint64LE(val)
			hasher.WriteUint64BE(val)
		case []byte:
			hasher.Write(val)
			hasher.WriteBytes(val)
		case string:
			hasher.WriteString(val)

		default:
			t.Fatalf("Invalid value type %T in test data", val)
		}

		// Ensure the result is the expected value.
		hash := hasher.Sum256()
		result := hex.EncodeToString(hash[:])
		if result != test.want {
			t.Errorf("%s: got %q, want %q", test.name, result, test.want)
			continue
		}
	}
}

// TestHasher256Copy ensures saving and restoring the intermediate state of the
// BLAKE-256 hasher by copying the instance works as intended.
func TestHasher256Copy(t *testing.T) {
	t.Parallel()

	for _, test := range savedStateTests {
		data1 := hexToBytes(test.dat1)
		data2 := hexToBytes(test.dat2)

		// Write the first bit of test data.
		hasher := NewHasher256()
		if test.salt != nil {
			hasher = NewHasher256Salt(test.salt)
		}
		hasher.WriteBytes(data1)

		// Save the intermediate state of the hasher.
		savedHasher := *hasher

		// Write some junk into the original hasher to ensure it is properly
		// restored below.
		hasher.WriteBytes([]byte{0xde, 0xad, 0xb3, 0x3f})

		// Restore the intermediate hasher state and write the second bit of
		// test data.
		*hasher = savedHasher
		hasher.WriteBytes(data2)

		// Ensure the result is the expected value.
		hash := hasher.Sum256()
		result := hex.EncodeToString(hash[:])
		if result != test.hash256 {
			t.Errorf("%s: got %q, want %q", test.name, result, test.hash256)
			continue
		}
	}
}

// TestHasher256SaveRestore ensures saving and restoring the intermediate state
// of the BLAKE-256 hasher by using the various state serialization methods
// works as intended.
func TestHasher256SaveRestore(t *testing.T) {
	t.Parallel()

	for _, test := range savedStateTests {
		data1 := hexToBytes(test.dat1)
		data2 := hexToBytes(test.dat2)

		// Various methods to test saving the intermediate state.
		saveTests := []struct {
			name   string
			saveFn func(*Hasher256) ([]byte, error)
		}{{
			name: "SaveStateUnbacked",
			saveFn: func(h *Hasher256) ([]byte, error) {
				return h.SaveState(nil), nil
			},
		}, {
			name: "SaveStateBacked",
			saveFn: func(h *Hasher256) ([]byte, error) {
				state := make([]byte, SavedStateSize)
				return h.SaveState(state[:0]), nil
			},
		}, {
			name: "SaveStateBackedPartialSmaller",
			saveFn: func(h *Hasher256) ([]byte, error) {
				const existingBytes = SavedStateSize - 1
				state := make([]byte, existingBytes)
				state = h.SaveState(state)
				return state[existingBytes:], nil
			},
		}, {
			name: "SaveStateBackedPartialLarger",
			saveFn: func(h *Hasher256) ([]byte, error) {
				const existingBytes = SavedStateSize + 1
				state := make([]byte, existingBytes)
				state = h.SaveState(state)
				return state[existingBytes:], nil
			},
		}, {
			name: "MarshalBinary",
			saveFn: func(h *Hasher256) ([]byte, error) {
				return h.MarshalBinary()
			},
		}}

		for _, saveTest := range saveTests {
			// Write the first bit of test data.
			hasher := NewHasher256()
			if test.salt != nil {
				hasher = NewHasher256Salt(test.salt)
			}
			hasher.WriteBytes(data1)

			// Save the intermediate state of the hasher using the save function
			// specified by the test.
			savedState, err := saveTest.saveFn(hasher)
			if err != nil {
				t.Fatalf("%q-%q: unexpected error saving state: %v", test.name,
					saveTest.name, err)
			}

			// Write some junk into the original hasher to ensure it is properly
			// restored below.
			hasher.WriteBytes([]byte{0xab, 0xcd, 0xef, 0x3f})

			// Restore the intermediate hasher state and write the second bit of
			// test data.
			if err := hasher.UnmarshalBinary(savedState); err != nil {
				t.Fatalf("%q-%q: unexpected error restoring state: %v",
					test.name, saveTest.name, err)
			}
			hasher.WriteBytes(data2)

			// Ensure the result is the expected value.
			hash := hasher.Sum256()
			result := hex.EncodeToString(hash[:])
			if result != test.hash256 {
				t.Errorf("%q-%q: got %q, want %q", test.name, saveTest.name,
					result, test.hash256)
				continue
			}
		}
	}
}

// TestSum256Vectors ensures Sum256 computes the correct hash for all of the
// known-good vectors.
func TestSum256Vectors(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		if test.salt != nil {
			continue
		}

		hash := Sum256(test.data)
		result := hex.EncodeToString(hash[:])
		if result != test.hash256 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash256)
			continue
		}
	}
}

// TestUnmarshalBinary256Errors ensures attempting for restore various malformed
// serialized intermediate states for BLAKE-256 return the expected errors.
func TestUnmarshalBinary256Errors(t *testing.T) {
	tests := []struct {
		name  string // test description
		state string // hex-encoded state to unmarshal
		err   error  // expected error
	}{{
		name:  "empty state",
		state: "",
		err:   ErrMalformedState,
	}, {
		name: "not enough bytes (one too few)",
		state: "bb67ae856a09e667bb67ae853c6ef372a54ff53a510e527f9b05688c1f83d9" +
			"ab5be0cd19313233343536373839303132333435360000000000000000201f1e" +
			"1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201000000" +
			"0000000000000000000000000000000000000000000000000000000000000000",
		err: ErrMalformedState,
	}, {
		name: "saved state from BLAKE-224",
		state: "367cd507c1059ed8367cd5073070dd17f70e5939ffc00b316858151164f98" +
			"fa7befa4fa40000000000000000000000000000000000000000000000000102" +
			"000000000000000000000000000000000000000000000000000000000000000" +
			"000000000000000000000000000000000000000000000000000000000000000" +
			"000002",
		err: ErrMismatchedState,
	}}
	for _, test := range tests {
		hasher := NewHasher256()
		err := hasher.UnmarshalBinary(hexToBytes(test.state))
		if !errors.Is(err, test.err) {
			t.Fatalf("%q: mismatched err -- got %v, want %v", test.name, err,
				test.err)
		}
	}
}

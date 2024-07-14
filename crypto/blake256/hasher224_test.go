// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// BLAKE-224 tests originally written by Dave Collins July 2024.

package blake256

import (
	"encoding/hex"
	"errors"
	"testing"
)

// TestHasher224Vectors ensures the BLAKE-224 hasher computes the correct hash
// for all of the known-good vectors.
func TestHasher224Vectors(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		hasher := NewHasher224()
		if test.salt != nil {
			hasher = NewHasher224Salt(test.salt)
		}
		hasher.WriteBytes(test.data)
		hash := hasher.Sum224()
		result := hex.EncodeToString(hash[:])
		if result != test.hash224 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash224)
			continue
		}
	}
}

// TestHasher224VectorsMultiWrite ensures the BLAKE-224 hasher computes the
// correct hash for all of the known-good vectors when writing the data in
// multiple independent calls.
func TestHasher224VectorsMultiWrite(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		hasher := NewHasher224()
		if test.salt != nil {
			hasher = NewHasher224Salt(test.salt)
		}
		if l := len(test.data); l >= 3 {
			hasher.WriteBytes(test.data[:l/3])
			hasher.WriteBytes(test.data[l/3 : 2*l/3])
			hasher.WriteBytes(test.data[2*l/3:])
		} else {
			hasher.Write(test.data)
		}
		hash := hasher.Sum224()
		result := hex.EncodeToString(hash[:])
		if result != test.hash224 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash224)
			continue
		}
	}
}

// TestHasher224HashInterface ensures the BLAKE-224 hasher correctly implements
// the [hash.Hash] interface.
func TestHasher224HashInterface(t *testing.T) {
	t.Parallel()

	// Ensure the expected block size is returned.
	hasher := New224()
	if blockSize := hasher.BlockSize(); blockSize != BlockSize {
		t.Fatalf("mismatched block size: got %d, want %d", blockSize, BlockSize)
	}

	// Ensure the expected hash size is returned.
	if hashSize := hasher.Size(); hashSize != Size224 {
		t.Fatalf("mismatched block size: got %d, want %d", hashSize, Size224)
	}

	// Ensure the hasher computes the correct hash for all of the known-good
	// vectors using the [hash.Hash] interface.
	sum := make([]byte, Size224)
	for _, test := range hasherVecTests {
		hasher = New224()
		if test.salt != nil {
			hasher = New224Salt(test.salt)
		}
		hasher.Write(test.data)
		result := hex.EncodeToString(hasher.Sum(sum[:0]))
		if result != test.hash224 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash224)
			continue
		}
	}
}

// TestHasher224Write ensures the various write methods of the BLAKE-224 hasher
// work as intended.
func TestHasher224Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string      // test description
		val  interface{} // value to write
		want string      // expected hash
	}{{
		name: "byte",
		val:  byte(0x0a),
		want: "eae5b01261424c859cb8eaa983e320667a28010b4d1ee08199ae104c",
	}, {
		name: "uint16",
		val:  uint16(0x0201 + 3),
		want: "219cad0f0444480766b5f9469fd41acedccd96d1b16641081ffad6ec",
	}, {
		name: "uint32",
		val:  uint32(0x04030201),
		want: "c1697b2cd801bdc70da9ce64bf8cefeb886ed6ad35e140aef0775e58",
	}, {
		name: "uint64",
		val:  uint64(0x0807060504030201),
		want: "838600130c12f3208c13fd7df92875ffcde520d98e374576aa5f3911",
	}, {
		name: "bytes",
		val:  []byte{0x01, 0x02, 0x03},
		want: "308e9a11ed485ded9374076be721fc1b7c1693f050ea155656308ef2",
	}, {
		name: "string",
		val:  "abc",
		want: "7c270941a0b4a412db099b710da90112ce49f8510add4f896c07ace4",
	}}

	hasher := NewHasher224()
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
		hash := hasher.Sum224()
		result := hex.EncodeToString(hash[:])
		if result != test.want {
			t.Errorf("%s: got %q, want %q", test.name, result, test.want)
			continue
		}
	}
}

// TestHasher224Copy ensures saving and restoring the intermediate state of the
// BLAKE-224 hasher by copying the instance works as intended.
func TestHasher224Copy(t *testing.T) {
	t.Parallel()

	for _, test := range savedStateTests {
		data1 := hexToBytes(test.dat1)
		data2 := hexToBytes(test.dat2)

		// Write the first bit of test data.
		hasher := NewHasher224()
		if test.salt != nil {
			hasher = NewHasher224Salt(test.salt)
		}
		hasher.WriteBytes(data1)

		// Save the intermediate state of the hasher.
		savedHasher := *hasher

		// Write some junk into the original hasher to ensure it is properly
		// restored below.
		hasher.WriteBytes([]byte{0xab, 0xcd, 0xef, 0x3f})

		// Restore the intermediate hasher state and write the second bit of
		// test data.
		*hasher = savedHasher
		hasher.WriteBytes(data2)

		// Ensure the result is the expected value.
		hash := hasher.Sum224()
		result := hex.EncodeToString(hash[:])
		if result != test.hash224 {
			t.Errorf("%s: got %q, want %q", test.name, result, test.hash224)
			continue
		}
	}
}

// TestHasher224SaveRestore ensures saving and restoring the intermediate state
// of the BLAKE-224 hasher by using the various state serialization methods
// works as intended.
func TestHasher224SaveRestore(t *testing.T) {
	t.Parallel()

	for _, test := range savedStateTests {
		data1 := hexToBytes(test.dat1)
		data2 := hexToBytes(test.dat2)

		// Various methods to test saving the intermediate state.
		saveTests := []struct {
			name   string
			saveFn func(*Hasher224) ([]byte, error)
		}{{
			name: "MarshalBinary",
			saveFn: func(h *Hasher224) ([]byte, error) {
				return h.MarshalBinary()
			},
		}, {
			name: "SaveStateBacked",
			saveFn: func(h *Hasher224) ([]byte, error) {
				state := make([]byte, SavedStateSize)
				return h.SaveState(state[:0]), nil
			},
		}, {
			name: "SaveStateBackedPartial",
			saveFn: func(h *Hasher224) ([]byte, error) {
				const existingBytes = 10
				state := make([]byte, existingBytes)
				state = h.SaveState(state)
				return state[existingBytes:], nil
			},
		}, {
			name: "SaveStateUnbacked",
			saveFn: func(h *Hasher224) ([]byte, error) {
				return h.SaveState(nil), nil
			},
		}}

		for _, saveTest := range saveTests {
			// Write the first bit of test data.
			hasher := NewHasher224()
			if test.salt != nil {
				hasher = NewHasher224Salt(test.salt)
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
			hash := hasher.Sum224()
			result := hex.EncodeToString(hash[:])
			if result != test.hash224 {
				t.Errorf("%q-%q: got %q, want %q", test.name, saveTest.name,
					result, test.hash224)
				continue
			}
		}
	}
}

// TestSum224Vectors ensures Sum224 computes the correct hash for all of the
// known-good vectors.
func TestSum224Vectors(t *testing.T) {
	t.Parallel()

	for _, test := range hasherVecTests {
		if test.salt != nil {
			continue
		}

		hash := Sum224(test.data)
		result := hex.EncodeToString(hash[:])
		if result != test.hash224 {
			t.Errorf("%q: got %q, want %q", test.name, result, test.hash224)
			continue
		}
	}
}

// TestUnmarshalBinary224Errors ensures attempting for restore various malformed
// serialized intermediate states for BLAKE-224 return the expected errors.
func TestUnmarshalBinary224Errors(t *testing.T) {
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
		state: "367cd507c1059ed8367cd5073070dd17f70e5939ffc00b316858151164f98f" +
			"a7befa4fa4313233343536373839303132333435360000000000000000201f1e" +
			"1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201000000" +
			"0000000000000000000000000000000000000000000000000000000000000000",
		err: ErrMalformedState,
	}, {
		name: "saved state from BLAKE-256",
		state: "bb67ae856a09e667bb67ae853c6ef372a54ff53a510e527f9b05688c1f83d9" +
			"ab5be0cd19313233343536373839303132333435360000000000000000201f1e" +
			"1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201000000" +
			"000000000000000000000000000000000000000000000000000000000000000020",
		err: ErrMismatchedState,
	}}
	for _, test := range tests {
		hasher := NewHasher224()
		err := hasher.UnmarshalBinary(hexToBytes(test.state))
		if !errors.Is(err, test.err) {
			t.Fatalf("%q: mismatched err -- got %v, want %v", test.name, err,
				test.err)
		}
	}
}

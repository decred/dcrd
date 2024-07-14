// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// BLAKE-256 tests originally written by Dave Collins May 2020.  BLAKE-224 tests
// added July 2024.

package blake256

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// hasherVecTest describes data to hash along with the expected hash for both
// BLAKE-224 and BLAKE-256.  It's defined separately since it is intended for
// use in multiple tests.
type hasherVecTest struct {
	name    string // test description
	salt    []byte // salt to use (defaults to all zero)
	data    []byte // data to hash
	hash224 string // expected BLAKE-224 hash
	hash256 string // expected BLAKE-256 hash
}

// hasherVecTests houses expected hash values for BLAKE-224 and BLAKE-256.  It
// includes the golden values from the specification, some values designed to
// test various edge conditions, and other known good values.
var hasherVecTests = []hasherVecTest{{
	name:    "one-block message from specification",
	data:    []byte{0x00},
	hash224: "4504cb0314fb2a4f7a692e696e487912fe3f2468fe312c73a5278ec5",
	hash256: "0ce8d4ef4dd7cd8d62dfded9d4edb0a774ae6a41929a74da23109e8f11139c87",
}, {
	name:    "two-block message from specification",
	data:    bytes.Repeat([]byte{0x00}, 72),
	hash224: "f5aa00dd1cb847e3140372af7b5c46b4888d82c8c0a917913cfb5d04",
	hash256: "d419bad32d504fb7d44d460c42c5593fe544fa4c135dec31e21bd9abdcc22d41",
}, {
	name:    "empty (no salt)",
	data:    nil,
	hash224: "7dc5313b1c04512a174bd6503b89607aecbee0903d40a8a569c94eed",
	hash256: "716f6e863f744b9ac22c97ec7b76ea5f5908bc5b2f67c61510bfc4751384ea7a",
}, {
	name:    "padding-only block (no salt)",
	data:    bytes.Repeat([]byte{0x03}, 127),
	hash224: "1720e615098f02ff69ca5b4f3f66b4fd9f5f8df95a4e570327c973bc",
	hash256: "9adc25ef2bd7129e6e5d9fb5dc93b91f9b33453b61b5085fbc40b0e44fe69eea",
}, {
	name:    "empty (salted)",
	salt:    bytes.Repeat([]byte{0x01}, 16),
	data:    nil,
	hash224: "8436b12388ccd3041f6bb10777ac5566f0695480da93180c35767909",
	hash256: "f60463bd4d9e3d9275228159b1a87fa5de77a0f9e47fd4a20db8d9ca35460d11",
}, {
	name:    "padding-only block (salted)",
	salt:    bytes.Repeat([]byte{0x01}, 16),
	data:    bytes.Repeat([]byte{0xde}, 127),
	hash224: "f0594c9048561be2f2f153019f9f80c13705d28840adaab3f75149b6",
	hash256: "513da045656d5128053c6e75676f9a379cea1eea4cafc6f29578cbb5010b7da4",
}, {
	name:    "one padding byte (no salt)",
	data:    bytes.Repeat([]byte{0x01}, 55),
	hash224: "7f016a9765c497402c49ecc5d27c76ca59bdd9a36d94e63ebba0c408",
	hash256: "b7d949165810b1a2f018c37392c633031bde4d45e453a3d5e62f2ba73fbf9769",
}, {
	name:    "two padding bytes (no salt)",
	data:    bytes.Repeat([]byte{0xff}, 54),
	hash224: "9b55f02a55dbe466d39d127cbb194ebee8e160670dab17efd38fc873",
	hash256: "a4391221d360dfebe984848d8e6cb81802d9a56a1b10c5bcbe0cdb3812efb322",
}, {
	name:    "one padding byte in second block (salted)",
	salt:    bytes.Repeat([]byte{0x5f}, 16),
	data:    bytes.Repeat([]byte{0x01}, 119),
	hash224: "fe6c61ef2bb29dc42f1e50437c977159f0b478d55b9d589d1619612f",
	hash256: "bdfa19309534dc7cec4c9ef9731374ef18774bf6c18a93ca94297e342946eed0",
}, {
	name:    "str1 (no salt)",
	data:    []byte("The quick brown fox jumps over the lazy dog"),
	hash224: "c8e92d7088ef87c1530aee2ad44dc720cc10589cc2ec58f95a15e51b",
	hash256: "7576698ee9cad30173080678e5965916adbb11cb5245d386bf1ffda1cb26c9d7",
}, {
	name:    "str2 (no salt)",
	data:    []byte("BLAKE"),
	hash224: "cfb6848add73e1cb47994c4765df33b8f973702705a30a71fe4747a3",
	hash256: "07663e00cf96fbc136cf7b1ee099c95346ba3920893d18cc8851f22ee2e36aa6",
}, {
	name:    "str3 (no salt)",
	data:    []byte("'BLAKE wins SHA-3! Hooray!!!' (I have time machine)"),
	hash224: "b0d0ca94a288bde157e47687f0a6675bac4858898f3ea59f35a456de",
	hash256: "18a393b4e62b1887a2edf79a5c5a5464daf5bbb976f4007bea16a73e4c1e198e",
}, {
	name:    "str4 (no salt)",
	data:    []byte("Go"),
	hash224: "dde9e442003c24495db607b17e07ec1f67396cc1907642a09a96594e",
	hash256: "fd7282ecc105ef201bb94663fc413db1b7696414682090015f17e309b835f1c2",
}, {
	name:    "str5 (no salt)",
	data:    []byte("HELP! I'm trapped in hash!"),
	hash224: "d11963a0dfb571fb54f9051e92489f50de877d7fd1c7d86a06fb4528",
	hash256: "1e75db2a709081f853c2229b65fd1558540aa5e7bd17b04b9a4b31989effa711",
}, {
	name: "str6 (no salt)",
	data: []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Donec a diam lectus. Sed sit amet ipsum mauris. Maecenas congu"),
	hash224: "fe312db9138c074964b6dfe347b078bb927074b73481c4d86ecb8b54",
	hash256: "af95fffc7768821b1e08866a2f9f66916762bfc9d71c4acb5fd515f31fd6785a",
}, {
	name:    "str7 (salted)",
	salt:    []byte("1234567890123456"),
	data:    nil,
	hash224: "e28df478e7e4bc7ab73111f3352f3e9ac10200fa1f00f6717301c019",
	hash256: "561d6d0cfa3d31d5eedaf2d575f3942539b03522befc2a1196ba0e51af8992a8",
}, {
	name:    "str8 (salted)",
	salt:    []byte("SALTsaltSaltSALT"),
	data:    []byte("It's so salty out there!"),
	hash224: "288b80c5de334c0d3283c25ccd691ccee5c842b62ecc49e3dce8edcb",
	hash256: "88cc11889bbbee42095337fe2153c591971f94fbf8fe540d3c7e9f1700ab2d0c",
}}

// savedStateTest describes data to hash before and after saving and restoring
// intermediate hashing states along with the expected hash for both BLAKE-224
// and BLAKE-256.  It's defined separately since it is intended for use in
// multiple tests.
type savedStateTest struct {
	name    string // test description
	salt    []byte // salt to use (defaults to all zero)
	dat1    string // hex-encoded data to write before saving state
	dat2    string // hex-encoded data to write after restoring state
	hash224 string // expected BLAKE-224 hash
	hash256 string // expected BLAKE-256 hash
}

// savedStateTests houses data to hash before and after saving and restoring
// intermediate hashing states along with the expected hash for BLAKE-224 and
// BLAKE-256.
var savedStateTests = []savedStateTest{{
	name:    "saved during first block, still in first block after (no salt)",
	dat1:    "0102",
	dat2:    "03",
	hash224: "4b64d199d1106fb472f0fb38ff9b1c8445386d6ac8a51ef296796b90",
	hash256: "e2e85c298e751fd418325ba59d021d5b9dad5ec1bf0af586765ba29fd85fdfb9",
}, {
	name:    "saved during first block, in second block after (no salt)",
	dat1:    "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
	dat2:    "2122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f4001",
	hash224: "551d3e9c7fca5d3d19570eef58f60231467c9822df9031f3168a46b1",
	hash256: "922b68374af4bf5daa46f8bf912874ae71cb0a9ef893fed0fba7695b020f2d13",
}, {
	name: "saved during second block, in second block after (no salt)",
	dat1: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20" +
		"2122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f4001",
	dat2:    "02",
	hash224: "e52de67c1e001ae4097616651a800c5ee37cb56c288a417c05e4575d",
	hash256: "387da3eee12183d972a7dc7232470ea3a856c1c18657d403da4d5b721273e864",
}, {
	name: "saved during second block, in third block after (no salt)",
	dat1: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20" +
		"2122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f4001",
	dat2: "02030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021" +
		"22232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f400102",
	hash224: "5a081cdf8f245c3dde5b9f5417e467a035a500b5a004fa61fd9695f6",
	hash256: "47b3cd98166a86cb6cce624683f61a5241440272b6f2bacaf813501855115623",
}, {
	name:    "saved during first block, still in first block after (salted)",
	salt:    bytes.Repeat([]byte{0x01}, 16),
	dat1:    "03",
	dat2:    "0102",
	hash224: "568417d5650fbf679e025901e1812db168052c64ed0d551920c816d8",
	hash256: "ee8345617ff51e69f909d1d92dc7be4e3e67fdb00474652847969d17adc0fcdd",
}, {
	name:    "saved during first block, in second block after (salted)",
	salt:    []byte("1234567890123456"),
	dat1:    "201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201",
	dat2:    "1004f3e3d3c3b3a393837363534333231303f2e2d2c2b2a2928272625242322212",
	hash224: "dfb8a2abc84706d3c634bafcc54f9c2291724c5f5806b5d658071a41",
	hash256: "3119f9fcf7051fe2b55920b733033a92731d9994cb4ad47cfdceb35f829c8497",
}, {
	name: "saved during second block, in second block after (salted)",
	salt: []byte("6543210987654321"),
	dat1: "1004f3e3d3c3b3a393837363534333231303f2e2d2c2b2a29282726252423222" +
		"1202f1e1d1c1b1a191817161514131211101f0e0d0c0b0a0908070605040302010",
	dat2:    "03",
	hash224: "eede890209b36e2438a4cf018e2efe2bcfe6d82ed9d7e045e67799ef",
	hash256: "d22f972d3d9adad090a48cd03f8af2d5bef8134afad4e75f6c548e8005bd607e",
}, {
	name: "saved during second block, in third block after (salted)",
	salt: []byte("aa55aa55aa55aa55"),
	dat1: "1004f3e3d3c3b3a393837363534333231303f2e2d2c2b2a29282726252423222" +
		"1202f1e1d1c1b1a191817161514131211101f0e0d0c0b0a0908070605040302010",
	dat2: "201004f3e3d3c3b3a393837363534333231303f2e2d2c2b2a292827262524232" +
		"221202f1e1d1c1b1a191817161514131211101f0e0d0c0b0a09080706050403020",
	hash224: "1de525166e500f8d4ba5c7375e50535ae9410e36d5aabbf3a128db75",
	hash256: "a60ff6286511587e741e808f8ef460c12e61dbe21d0b7dc71cfbf2175ba0f1c4",
}}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected.  It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// TestSaltPanics ensures the various methods that accept salt panic when
// provided with an incorrect salt length.
func TestSaltPanics(t *testing.T) {
	// Ensure passing salt of the wrong length to the various salt methods
	// panics.
	testSaltPanic := func(funcName string, fn func()) {
		t.Helper()

		defer func() {
			if err := recover(); err == nil {
				t.Errorf("%s did not panic with incorrect salt len", funcName)
			}
		}()
		fn()
	}
	badSalt := make([]byte, 15)
	testSaltPanic("New224Salt", func() { New224Salt(badSalt) })
	testSaltPanic("NewSalt", func() { NewSalt(badSalt) })
	testSaltPanic("NewHasher224Salt", func() { NewHasher224Salt(badSalt) })
	testSaltPanic("NewHasher256Salt", func() { NewHasher256Salt(badSalt) })
}

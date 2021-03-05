// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdaddr

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/decred/base58"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/ripemd160"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/edwards/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// These are redefinitions of version 0 opcodes in the txscript package that are
// used in this package to generate payment scripts.  Ultimately, this should be
// using the constant definitions from txscript instead, but it would currently
// create a cyclic dependency since txscript will need to depend on this package
// for signing.
const (
	opData20      = 0x14
	opData30      = 0x1e
	opData32      = 0x20
	opData33      = 0x21
	op1           = 0x51
	op2           = 0x52
	opReturn      = 0x6a
	opDup         = 0x76
	opEqual       = 0x87
	opEqualVerify = 0x88
	opHash160     = 0xa9
	opCheckSig    = 0xac
	opSSTx        = 0xba
	opSSGen       = 0xbb
	opSSRTx       = 0xbc
	opSSTxChange  = 0xbd
	opCheckSigAlt = 0xbe
	opTGen        = 0xc3
)

const (
	// opPushSTEd25519 is the dcrec.STEd25519 signature type converted to the
	// associated small integer data push opcode.
	opPushSTEd25519 = op1

	// opPushSTSchnorrSecp256k1 is the dcrec.STSchnorrSecp256k1 signature type
	// converted to the associated small integer data push opcode.
	opPushSTSchnorrSecp256k1 = op2

	// sigTypeSecp256k1PubKeyCompOddFlag specifies the bitmask to apply to the
	// pubkey address signature type byte for those that deal with compressed
	// secp256k1 pubkeys to specify the omitted y coordinate is odd.
	sigTypeSecp256k1PubKeyCompOddFlag = uint8(1 << 7)

	// commitP2SHFlag specifies the bitmask to apply to an amount in a ticket
	// commitment in order to specify if it is a pay-to-script-hash commitment.
	// The value is derived from the fact it is encoded as the most significant
	// bit in the amount.
	commitP2SHFlag = uint64(1 << 63)

	// p2pkhPaymentScriptLen is the length of a standard version 0 P2PKH script
	// for secp256k1+ecdsa.
	p2pkhPaymentScriptLen = 25

	// p2shPaymentScriptLen is the length of a standard version 0 P2SH script.
	p2shPaymentScriptLen = 23
)

// AddressParamsV0 defines an interface that is used to provide the parameters
// required when encoding and decoding addresses for version 0 scripts.  These
// values are typically well-defined and unique per network.
type AddressParamsV0 interface {
	// AddrIDPubKeyV0 returns the magic prefix bytes for version 0 pay-to-pubkey
	// addresses.
	AddrIDPubKeyV0() [2]byte

	// AddrIDPubKeyHashECDSAV0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey is secp256k1 and
	// the signature algorithm is ECDSA.
	AddrIDPubKeyHashECDSAV0() [2]byte

	// AddrIDPubKeyHashEd25519V0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey and signature
	// algorithm are Ed25519.
	AddrIDPubKeyHashEd25519V0() [2]byte

	// AddrIDPubKeyHashSchnorrV0 returns the magic prefix bytes for version 0
	// pay-to-pubkey-hash addresses where the underlying pubkey is secp256k1 and
	// the signature algorithm is Schnorr.
	AddrIDPubKeyHashSchnorrV0() [2]byte

	// AddrIDScriptHashV0 returns the magic prefix bytes for version 0
	// pay-to-script-hash addresses.
	AddrIDScriptHashV0() [2]byte
}

// encodeAddressV0 returns a human-readable payment address for the data and
// netID which encodes the network and address type using the format for version
// 0 scripts.
func encodeAddressV0(data []byte, netID [2]byte) string {
	// The overall format for an address for version 0 scripts is the base58
	// check encoding of data which varies by address type.  In other words, it
	// is:
	//
	//   2-byte network and address type || data || 4-byte checksum
	return base58.CheckEncode(data, netID)
}

// Hash160 calculates the hash ripemd160(blake256(b)).
func Hash160(buf []byte) []byte {
	b256Hash := blake256.Sum256(buf)
	hasher := ripemd160.New()
	hasher.Write(b256Hash[:])
	return hasher.Sum(nil)
}

// AddressPubKeyEcdsaSecp256k1V0 specifies an address that represents a payment
// destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key.
//
// This is commonly referred to as pay-to-pubkey (P2PK) for legacy reasons,
// however, since it is possible to support multiple algorithm and signature
// scheme combinations, it is technically more accurate to refer to it as
// pay-to-pubkey-ecdsa-secp256k1.
type AddressPubKeyEcdsaSecp256k1V0 struct {
	pubKeyID         [2]byte
	pubKeyHashID     [2]byte
	serializedPubKey []byte
}

// Ensure AddressPubKeyEcdsaSecp256k1V0 implements the Address and
// AddressPubKeyHasher interfaces.
var _ Address = (*AddressPubKeyEcdsaSecp256k1V0)(nil)
var _ AddressPubKeyHasher = (*AddressPubKeyEcdsaSecp256k1V0)(nil)

// NewAddressPubKeyEcdsaSecp256k1V0Raw returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key using version 0 scripts.
//
// The provided public key MUST be a valid secp256k1 public key serialized in
// the _compressed_ format or an error will be returned.
//
// See NewAddressPubKeyEcdsaSecp256k1V0 for a variant that accepts the public
// key as a concrete type instance instead.
//
// This function can be useful to callers who already need the serialized public
// key for other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeyEcdsaSecp256k1V0Raw(serializedPubKey []byte,
	params AddressParamsV0) (*AddressPubKeyEcdsaSecp256k1V0, error) {

	// Attempt to parse the provided public key to ensure it is both a valid
	// serialization and that it is a valid point on the secp256k1 curve.
	_, err := secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		str := fmt.Sprintf("failed to parse public key: %v", err)
		return nil, makeError(ErrInvalidPubKey, str)
	}

	// Ensure the provided serialized public key is in the compressed format.
	// This probably should be returned from secp256k1, but do it here to avoid
	// API churn.  The pubkey is known to be valid since it parsed above, so
	// it's safe to simply examine the leading byte to get the format.
	//
	// Notice that both the uncompressed and hybrid forms are intentionally not
	// supported.
	switch serializedPubKey[0] {
	case secp256k1.PubKeyFormatCompressedEven:
	case secp256k1.PubKeyFormatCompressedOdd:
	default:
		str := fmt.Sprintf("serialized public key %x is not a valid format",
			serializedPubKey)
		return nil, makeError(ErrInvalidPubKeyFormat, str)
	}

	return &AddressPubKeyEcdsaSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashECDSAV0(),
		serializedPubKey: serializedPubKey,
	}, nil
}

// NewAddressPubKeyEcdsaSecp256k1V0 returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid ECDSA
// signature for a specific secp256k1 public key using version 0 scripts.
//
// See NewAddressPubKeyEcdsaSecp256k1V0Raw for a variant that accepts the public
// key already serialized in the _compressed_ format instead of a concrete type.
// It can be useful to callers who already need the serialized public key for
// other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeyEcdsaSecp256k1V0(pubKey Secp256k1PublicKey,
	params AddressParamsV0) (*AddressPubKeyEcdsaSecp256k1V0, error) {

	return &AddressPubKeyEcdsaSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashECDSAV0(),
		serializedPubKey: pubKey.SerializeCompressed(),
	}, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEcdsaSecp256k1V0) Address() string {
	// The format for the data portion of a public key address used with
	// elliptic curves is:
	//   identifier byte || 32-byte X coordinate
	//
	// The identifier byte specifies the curve and signature scheme combination
	// as well as encoding the oddness of the Y coordinate for secp256k1 public
	// keys in the high bit.
	var data [33]byte
	data[0] = byte(dcrec.STEcdsaSecp256k1)
	if addr.serializedPubKey[0] == secp256k1.PubKeyFormatCompressedOdd {
		data[0] |= sigTypeSecp256k1PubKeyCompOddFlag
	}
	copy(data[1:], addr.serializedPubKey[1:])
	return encodeAddressV0(data[:], addr.pubKeyID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEcdsaSecp256k1V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-ecdsa-secp256k1 script is one of the following forms:
	//  <33-byte compressed pubkey> CHECKSIG
	//  <65-byte uncompressed pubkey> CHECKSIG
	//
	// However, this address type intentionally only supports the compressed
	// form.
	var script [35]byte
	script[0] = opData33
	copy(script[1:34], addr.serializedPubKey)
	script[34] = opCheckSig
	return 0, script[:]
}

// AddressPubKeyHash returns the address converted to a
// pay-to-pubkey-hash-ecdsa-secp256k1 address.
//
// Note that the hash used in resulting address is the hash of the serialized
// public key and the address constructor intentionally only supports public
// keys in the compressed format.  In other words, the resulting address will
// impose an encumbrance that requires the public key to be provided in the
// compressed format.
func (addr *AddressPubKeyEcdsaSecp256k1V0) AddressPubKeyHash() Address {
	pkHash := Hash160(addr.serializedPubKey)
	addrPKH := &AddressPubKeyHashEcdsaSecp256k1V0{
		netID: addr.pubKeyHashID,
	}
	copy(addrPKH.hash[:], pkHash)
	return addrPKH
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyEcdsaSecp256k1V0) String() string {
	return addr.Address()
}

// AddressPubKeyEd25519V0 specifies an address that represents a payment
// destination which imposes an encumbrance that requires a valid Ed25519
// signature for a specific Ed25519 public key.
//
// This is commonly referred to as pay-to-pubkey-ed25519.
type AddressPubKeyEd25519V0 struct {
	pubKeyID         [2]byte
	pubKeyHashID     [2]byte
	serializedPubKey []byte
}

// Ensure AddressPubKeyEd25519V0 implements the Address and AddressPubKeyHasher
// interfaces.
var _ Address = (*AddressPubKeyEd25519V0)(nil)
var _ AddressPubKeyHasher = (*AddressPubKeyEd25519V0)(nil)

// NewAddressPubKeyEd25519V0Raw returns an address that represents a payment
// destination which imposes an encumbrance that requires a valid Ed25519
// signature for a specific Ed25519 public key using version 0 scripts.
//
// See NewAddressPubKeyEd25519V0 for a variant that accepts the public key as a
// concrete type instance instead.
func NewAddressPubKeyEd25519V0Raw(serializedPubKey []byte,
	params AddressParamsV0) (*AddressPubKeyEd25519V0, error) {

	// Attempt to parse the provided public key to ensure it is both a valid
	// serialization and that it is a valid point on the underlying curve.
	_, err := edwards.ParsePubKey(serializedPubKey)
	if err != nil {
		str := fmt.Sprintf("failed to parse public key: %v", err)
		return nil, makeError(ErrInvalidPubKey, str)
	}

	return &AddressPubKeyEd25519V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashEd25519V0(),
		serializedPubKey: serializedPubKey,
	}, nil
}

// NewAddressPubKeyEd25519V0 returns an address that represents a payment
// destination which imposes an encumbrance that requires a valid Ed25519
// signature for a specific Ed25519 public key using version 0 scripts.
//
// See NewAddressPubKeyEd25519Raw for a variant that accepts the public key
// already serialized instead of a concrete type.  It can be useful to callers
// who already need the serialized public key for other purposes to avoid the
// need to serialize it multiple times.
func NewAddressPubKeyEd25519V0(pubKey Ed25519PublicKey,
	params AddressParamsV0) (*AddressPubKeyEd25519V0, error) {

	return &AddressPubKeyEd25519V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashEd25519V0(),
		serializedPubKey: pubKey.Serialize(),
	}, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEd25519V0) Address() string {
	// The format for the data portion of a public key address used with
	// elliptic curves is:
	//   identifier byte || 32-byte X coordinate
	//
	// The identifier byte specifies the curve and signature scheme combination
	// as well as encoding the oddness of the Y coordinate for secp256k1 public
	// keys in the high bit.
	//
	// Since this address is for an ed25519 public key, the oddness bit is not
	// used/encoded.
	var data [33]byte
	data[0] = byte(dcrec.STEd25519)
	copy(data[1:], addr.serializedPubKey)
	return encodeAddressV0(data[:], addr.pubKeyID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyEd25519V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-ed25519 script is one of the form:
	//  <32-byte pubkey> <1-byte sigtype> CHECKSIGALT
	//
	// Since the signature type is 1, it is pushed as a small integer.
	var script [35]byte
	script[0] = opData32
	copy(script[1:33], addr.serializedPubKey)
	script[33] = opPushSTEd25519
	script[34] = opCheckSigAlt
	return 0, script[:]
}

// AddressPubKeyHash returns the address converted to a
// pay-to-pubkey-hash-ed25519 address.
func (addr *AddressPubKeyEd25519V0) AddressPubKeyHash() Address {
	pkHash := Hash160(addr.serializedPubKey)
	addrPKH := &AddressPubKeyHashEd25519V0{
		netID: addr.pubKeyHashID,
	}
	copy(addrPKH.hash[:], pkHash)
	return addrPKH
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyEd25519V0) String() string {
	return addr.Address()
}

// AddressPubKeySchnorrSecp256k1V0 specifies an address that represents a
// payment destination which imposes an encumbrance that requires a valid
// EC-Schnorr-DCRv0 signature for a specific secp256k1 public key.
//
// This is commonly referred to as pay-to-pubkey-schnorr-secp256k1.
type AddressPubKeySchnorrSecp256k1V0 struct {
	pubKeyID         [2]byte
	pubKeyHashID     [2]byte
	serializedPubKey []byte
}

// Ensure AddressPubKeySchnorrSecp256k1V0 implements the Address and
// AddressPubKeyHasher interface.
var _ Address = (*AddressPubKeySchnorrSecp256k1V0)(nil)
var _ AddressPubKeyHasher = (*AddressPubKeySchnorrSecp256k1V0)(nil)

// NewAddressPubKeySchnorrSecp256k1V0Raw returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid
// EC-Schnorr-DCRv0 signature for a specific secp256k1 public key using version
// 0 scripts.
//
// The provided public key MUST be a valid secp256k1 public key serialized in
// the _compressed_ format or an error will be returned.
//
// See NewAddressPubKeySchnorrSecp256k1V0 for a variant that accepts the public
// key as a concrete type instance instead.
//
// This function can be useful to callers who already need the serialized public
// key for other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeySchnorrSecp256k1V0Raw(serializedPubKey []byte,
	params AddressParamsV0) (*AddressPubKeySchnorrSecp256k1V0, error) {

	// Attempt to parse the provided public key to ensure it is both a valid
	// serialization and that it is a valid point on the secp256k1 curve.
	_, err := secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		str := fmt.Sprintf("failed to parse public key: %v", err)
		return nil, makeError(ErrInvalidPubKey, str)
	}

	// Ensure the provided serialized public key is in the compressed format.
	// This probably should be returned from secp256k1, but do it here to avoid
	// API churn.  The pubkey is known to be valid since it parsed above, so
	// it's safe to simply examine the leading byte to get the format.
	//
	// Notice that both the uncompressed and hybrid forms are intentionally not
	// supported.
	switch serializedPubKey[0] {
	case secp256k1.PubKeyFormatCompressedEven:
	case secp256k1.PubKeyFormatCompressedOdd:
	default:
		str := fmt.Sprintf("serialized public key %x is not a valid format",
			serializedPubKey)
		return nil, makeError(ErrInvalidPubKeyFormat, str)
	}

	return &AddressPubKeySchnorrSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashSchnorrV0(),
		serializedPubKey: serializedPubKey,
	}, nil
}

// NewAddressPubKeySchnorrSecp256k1V0 returns an address that represents a
// payment destination which imposes an encumbrance that requires a valid
// EC-Schnorr-DCRv0 signature for a specific secp256k1 public key using version
// 0 scripts.
//
// See NewAddressPubKeySchnorrSecp256k1V0Raw for a variant that accepts the public
// key already serialized in the _compressed_ format instead of a concrete type.
// It can be useful to callers who already need the serialized public key for
// other purposes to avoid the need to serialize it multiple times.
func NewAddressPubKeySchnorrSecp256k1V0(pubKey Secp256k1PublicKey,
	params AddressParamsV0) (*AddressPubKeySchnorrSecp256k1V0, error) {

	return &AddressPubKeySchnorrSecp256k1V0{
		pubKeyID:         params.AddrIDPubKeyV0(),
		pubKeyHashID:     params.AddrIDPubKeyHashSchnorrV0(),
		serializedPubKey: pubKey.SerializeCompressed(),
	}, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeySchnorrSecp256k1V0) Address() string {
	// The format for the data portion of a public key address used with
	// elliptic curves is:
	//   identifier byte || 32-byte X coordinate
	//
	// The identifier byte specifies the curve and signature scheme combination
	// as well as encoding the oddness of the Y coordinate for secp256k1 public
	// keys in the high bit.
	var data [33]byte
	data[0] = byte(dcrec.STSchnorrSecp256k1)
	if addr.serializedPubKey[0] == secp256k1.PubKeyFormatCompressedOdd {
		data[0] |= sigTypeSecp256k1PubKeyCompOddFlag
	}
	copy(data[1:], addr.serializedPubKey[1:])
	return encodeAddressV0(data[:], addr.pubKeyID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeySchnorrSecp256k1V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-schnorr-secp256k1 script is of the following form:
	//  <33-byte compressed pubkey> <1-byte sigtype> CHECKSIGALT
	//
	// Since the signature type is 2, it is pushed as a small integer.
	var script [36]byte
	script[0] = opData33
	copy(script[1:34], addr.serializedPubKey)
	script[34] = opPushSTSchnorrSecp256k1
	script[35] = opCheckSigAlt
	return 0, script[:]
}

// AddressPubKeyHash returns the address converted to a
// pay-to-pubkey-hash-schnorr-secp256k1 address.
//
// Note that the hash used in resulting address is the hash of the serialized
// public key and only public keys in the compressed format are supported.  In
// other words, the resulting address will impose an encumbrance that requires
// the public key to be provided in the compressed format.
func (addr *AddressPubKeySchnorrSecp256k1V0) AddressPubKeyHash() Address {
	pkHash := Hash160(addr.serializedPubKey)
	addrPKH := &AddressPubKeyHashSchnorrSecp256k1V0{
		netID: addr.pubKeyHashID,
	}
	copy(addrPKH.hash[:], pkHash)
	return addrPKH
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeySchnorrSecp256k1V0) String() string {
	return addr.Address()
}

// AddressPubKeyHashEcdsaSecp256k1V0 specifies an address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key that hashes to the given public key hash along with a valid ECDSA
// signature for that public key.
//
// This is commonly referred to as pay-to-pubkey-hash (P2PKH) for legacy
// reasons, however, since it is possible to support multiple algorithm and
// signature scheme combinations, it is technically more accurate to refer to it
// as pay-to-pubkey-hash-ecdsa-secp256k1.
type AddressPubKeyHashEcdsaSecp256k1V0 struct {
	netID [2]byte
	hash  [ripemd160.Size]byte
}

// Ensure AddressPubKeyHashEcdsaSecp256k1V0 implements the Address,
// StakeAddress, and Hash160er interfaces.
var _ Address = (*AddressPubKeyHashEcdsaSecp256k1V0)(nil)
var _ StakeAddress = (*AddressPubKeyHashEcdsaSecp256k1V0)(nil)
var _ Hash160er = (*AddressPubKeyHashEcdsaSecp256k1V0)(nil)

// NewAddressPubKeyHashEcdsaSecp256k1V0 returns an address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key that hashes to the provided public key hash along with a valid
// ECDSA signature for that public key using version 0 scripts.
//
// The provided public key hash must be 20 bytes and is expected to be the
// Hash160 of the associated secp256k1 public key serialized in the _compressed_
// format.
//
// It is important to note that while it is technically possible for legacy
// reasons to create this specific type of address based on the hash of a public
// key in the uncompressed format, so long as it is also redeemed with that same
// public key in uncompressed format, it is *HIGHLY* recommended to use the
// compressed format since it occupies less space on the chain and is more
// consistent with other address formats where uncompressed public keys are NOT
// supported.
func NewAddressPubKeyHashEcdsaSecp256k1V0(pkHash []byte,
	params AddressParamsV0) (*AddressPubKeyHashEcdsaSecp256k1V0, error) {

	// Check for a valid script hash length.
	if len(pkHash) != ripemd160.Size {
		str := fmt.Sprintf("public key hash is %d bytes vs required %d bytes",
			len(pkHash), ripemd160.Size)
		return nil, makeError(ErrInvalidHashLen, str)
	}

	addr := &AddressPubKeyHashEcdsaSecp256k1V0{
		netID: params.AddrIDPubKeyHashECDSAV0(),
	}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) Address() string {
	// The format for the data portion of addresses that encode 160-bit hashes
	// is merely the hash itself:
	//   20-byte ripemd160 hash
	return encodeAddressV0(addr.hash[:ripemd160.Size], addr.netID)
}

// putPaymentScript serializes the payment script associated with the address
// directly into the passed byte slice which must be at least
// p2pkhPaymentScriptLen bytes in length or it will panic.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) putPaymentScript(script []byte) {
	// A pay-to-pubkey-hash-ecdsa-secp256k1 script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY CHECKSIG
	script[0] = opDup
	script[1] = opHash160
	script[2] = opData20
	copy(script[3:23], addr.hash[:])
	script[23] = opEqualVerify
	script[24] = opCheckSig
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-hash-ecdsa-secp256k1 script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY CHECKSIG
	var script [p2pkhPaymentScriptLen]byte
	addr.putPaymentScript(script[:])
	return 0, script[:]
}

// VotingRightsScript returns the script version associated with the address
// along with a script to give voting rights to the address.  It is only valid
// when used in stake ticket purchase transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) VotingRightsScript() (uint16, []byte) {
	// A script that assigns voting rights for a ticket to this address type is
	// of the form:
	//  SSTX [standard pay-to-pubkey-hash-ecdsa-secp256k1 script]
	var script [p2pkhPaymentScriptLen + 1]byte
	script[0] = opSSTx
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// RewardCommitmentScript returns the script version associated with the address
// along with a script that commits the original funds locked to purchase a
// ticket plus the reward to the address along with limits to impose on any
// fees.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) RewardCommitmentScript(amount int64, limits uint16) (uint16, []byte) {
	// The reward commitment output of a ticket purchase is a provably pruneable
	// script of the form:
	//   RETURN <20-byte hash || 8-byte amount || 2-byte fee limits>
	//
	// The high bit of the amount is used to indicate whether the provided hash
	// is a public key hash that represents a pay-to-pubkey-hash-ecdsa-secp256k1
	// script or a script hash that represents a pay-to-script-hash script.  It
	// is NOT set for a public key hash.
	var script [32]byte
	script[0] = opReturn
	script[1] = opData30
	copy(script[2:22], addr.hash[:])
	binary.LittleEndian.PutUint64(script[22:30], uint64(amount) & ^commitP2SHFlag)
	binary.LittleEndian.PutUint16(script[30:32], limits)
	return 0, script[:]
}

// StakeChangeScript returns the script version associated with the address
// along with a script to pay change to the address.  It is only valid when used
// in stake ticket purchase and treasury add transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) StakeChangeScript() (uint16, []byte) {
	// A stake change script to this address type is of the form:
	//  SSTXCHANGE [standard pay-to-pubkey-hash-ecdsa-secp256k1 script]
	var script [p2pkhPaymentScriptLen + 1]byte
	script[0] = opSSTxChange
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayVoteCommitmentScript returns the script version associated with the
// address along with a script to pay the original funds locked to purchase a
// ticket plus the reward to the address.  The address must have previously been
// committed to by the ticket purchase.  The script is only valid when used in
// stake vote transactions whose associated tickets are eligible to vote.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) PayVoteCommitmentScript() (uint16, []byte) {
	// A script that pays a ticket commitment as part of a vote to this address
	// type is of the form:
	//  SSGEN [standard pay-to-pubkey-hash-ecdsa-secp256k1 script]
	var script [p2pkhPaymentScriptLen + 1]byte
	script[0] = opSSGen
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayRevokeCommitmentScript returns the script version associated with the
// address along with a script to revoke an expired or missed ticket which pays
// the original funds locked to purchase a ticket to the address.  The address
// must have previously been committed to by the ticket purchase.  The script is
// only valid when used in stake revocation transactions whose associated
// tickets have been missed or expired.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) PayRevokeCommitmentScript() (uint16, []byte) {
	// A ticket revocation script to this address type is of the form:
	//  SSRTX [standard pay-to-pubkey-hash-ecdsa-secp256k1 script]
	var script [p2pkhPaymentScriptLen + 1]byte
	script[0] = opSSRTx
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayFromTreasuryScript returns the script version associated with the address
// along with a script that pays funds from the treasury to the address.  The
// script is only valid when used in treasury spend transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) PayFromTreasuryScript() (uint16, []byte) {
	// A script that pays from the treasury as a part of a treasury spend to
	// this address type is of the form:
	//  TGEN [standard pay-to-pubkey-hash-ecdsa-secp256k1 script]
	var script [p2pkhPaymentScriptLen + 1]byte
	script[0] = opTGen
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) Hash160() *[ripemd160.Size]byte {
	return &addr.hash
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyHashEcdsaSecp256k1V0) String() string {
	return addr.Address()
}

// AddressPubKeyHashEd25519V0 specifies an address that represents a a payment
// destination which imposes an encumbrance that requires an Ed25519 public key
// that hashes to the given public key hash along with a valid Ed25519 signature
// for that public key.
//
// This is commonly referred to as pay-to-pubkey-hash-ed25519.
type AddressPubKeyHashEd25519V0 struct {
	netID [2]byte
	hash  [ripemd160.Size]byte
}

// Ensure AddressPubKeyHashEd25519V0 implements the Address and Hash160er
// interfaces.
var _ Address = (*AddressPubKeyHashEd25519V0)(nil)
var _ Hash160er = (*AddressPubKeyHashEd25519V0)(nil)

// NewAddressPubKeyHashEd25519V0 returns an address that represents a a payment
// destination which imposes an encumbrance that requires an Ed25519 public key
// that hashes to the provided public key hash along with a valid Ed25519
// signature for that public key using version 0 scripts.
//
// The provided public key hash must be 20 bytes and be the Hash160 of the
// correct public key or it will not be redeemable with the expected public key
// because it would hash to a different value than the payment script generated
// for the provided incorrect public key hash expects.
func NewAddressPubKeyHashEd25519V0(pkHash []byte,
	params AddressParamsV0) (*AddressPubKeyHashEd25519V0, error) {

	// Check for a valid script hash length.
	if len(pkHash) != ripemd160.Size {
		str := fmt.Sprintf("public key hash is %d bytes vs required %d bytes",
			len(pkHash), ripemd160.Size)
		return nil, makeError(ErrInvalidHashLen, str)
	}

	addr := &AddressPubKeyHashEd25519V0{
		netID: params.AddrIDPubKeyHashEd25519V0(),
	}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashEd25519V0) Address() string {
	// The format for the data portion of addresses that encode 160-bit hashes
	// is merely the hash itself:
	//   20-byte ripemd160 hash
	return encodeAddressV0(addr.hash[:ripemd160.Size], addr.netID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashEd25519V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-hash-ed25519 script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte sigtype> CHECKSIGALT
	//
	// Since the signature type is 1, it is pushed as a small integer.
	var script [26]byte
	script[0] = opDup
	script[1] = opHash160
	script[2] = opData20
	copy(script[3:23], addr.hash[:])
	script[23] = opEqualVerify
	script[24] = opPushSTEd25519
	script[25] = opCheckSigAlt
	return 0, script[:]
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (addr *AddressPubKeyHashEd25519V0) Hash160() *[ripemd160.Size]byte {
	return &addr.hash
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyHashEd25519V0) String() string {
	return addr.Address()
}

// AddressPubKeyHashSchnorrSecp256k1V0 specifies address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key in the _compressed_ format that hashes to the given public key
// hash along with a valid EC-Schnorr-DCRv0 signature for that public key.
//
// This is commonly referred to as pay-to-pubkey-hash-schnorr-secp256k1.
type AddressPubKeyHashSchnorrSecp256k1V0 struct {
	netID [2]byte
	hash  [ripemd160.Size]byte
}

// Ensure AddressPubKeyHashSchnorrSecp256k1V0 implements the Address and
// Hash160er interfaces.
var _ Address = (*AddressPubKeyHashSchnorrSecp256k1V0)(nil)
var _ Hash160er = (*AddressPubKeyHashSchnorrSecp256k1V0)(nil)

// NewAddressPubKeyHashSchnorrSecp256k1V0 returns an address that represents a
// payment destination which imposes an encumbrance that requires a secp256k1
// public key in the _compressed_ format that hashes to the provided public key
// hash along with a valid EC-Schnorr-DCRv0 signature for that public key using
// version 0 scripts.
//
// The provided public key hash must be 20 bytes and is expected to be the
// Hash160 of the associated secp256k1 public key serialized in the _compressed_
// format.
//
// WARNING: It is important to note that, unlike in the case of the ECDSA
// variant of this type of address, redemption via a public key in the
// uncompressed format is NOT supported by the consensus rules for this type, so
// it is *EXTREMELY* important to ensure the provided hash is of the serialized
// public key in the compressed format or the associated coins will NOT be
// redeemable.
func NewAddressPubKeyHashSchnorrSecp256k1V0(pkHash []byte,
	params AddressParamsV0) (*AddressPubKeyHashSchnorrSecp256k1V0, error) {

	// Check for a valid script hash length.
	if len(pkHash) != ripemd160.Size {
		str := fmt.Sprintf("public key hash is %d bytes vs required %d bytes",
			len(pkHash), ripemd160.Size)
		return nil, makeError(ErrInvalidHashLen, str)
	}

	addr := &AddressPubKeyHashSchnorrSecp256k1V0{
		netID: params.AddrIDPubKeyHashSchnorrV0(),
	}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashSchnorrSecp256k1V0) Address() string {
	// The format for the data portion of addresses that encode 160-bit hashes
	// is merely the hash itself:
	//   20-byte ripemd160 hash
	return encodeAddressV0(addr.hash[:ripemd160.Size], addr.netID)
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressPubKeyHashSchnorrSecp256k1V0) PaymentScript() (uint16, []byte) {
	// A pay-to-pubkey-hash-schnorr-secp256k1 script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte sigtype> CHECKSIGALT
	//
	// Since the signature type is 2, it is pushed as a small integer.
	var script [26]byte
	script[0] = opDup
	script[1] = opHash160
	script[2] = opData20
	copy(script[3:23], addr.hash[:])
	script[23] = opEqualVerify
	script[24] = opPushSTSchnorrSecp256k1
	script[25] = opCheckSigAlt
	return 0, script[:]
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (addr *AddressPubKeyHashSchnorrSecp256k1V0) Hash160() *[ripemd160.Size]byte {
	return &addr.hash
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressPubKeyHashSchnorrSecp256k1V0) String() string {
	return addr.Address()
}

// AddressScriptHashV0 specifies an address that represents a payment
// destination which imposes an encumbrance that requires a script that hashes
// to the provided script hash along with all of the encumbrances that script
// itself imposes.  The script is commonly referred to as a redeem script.
//
// This is commonly referred to as pay-to-script-hash (P2SH).
type AddressScriptHashV0 struct {
	netID [2]byte
	hash  [ripemd160.Size]byte
}

// Ensure AddressScriptHashV0 implements the Address and StakeAddress
// interfaces.
var _ Address = (*AddressScriptHashV0)(nil)
var _ StakeAddress = (*AddressScriptHashV0)(nil)

// NewAddressScriptHashV0FromHash returns an address that represents a payment
// destination which imposes an encumbrance that requires a script that hashes
// to the provided script hash along with all of the encumbrances that script
// itself imposes using version 0 scripts.  The script is commonly referred to
// as a redeem script.
//
// The provided script hash must be 20 bytes and is expected to be the Hash160
// of the associated redeem script.
//
// See NewAddressScriptHashV0 for a variant that accepts the redeem script instead
// of its hash.  It can be used as a convenience for callers that have the
// redeem script available.
func NewAddressScriptHashV0FromHash(scriptHash []byte,
	params AddressParamsV0) (*AddressScriptHashV0, error) {

	// Check for a valid script hash length.
	if len(scriptHash) != ripemd160.Size {
		str := fmt.Sprintf("script hash is %d bytes vs required %d bytes",
			len(scriptHash), ripemd160.Size)
		return nil, makeError(ErrInvalidHashLen, str)
	}

	addr := &AddressScriptHashV0{
		netID: params.AddrIDScriptHashV0(),
	}
	copy(addr.hash[:], scriptHash)
	return addr, nil
}

// NewAddressScriptHashV0 returns an address that represents a payment
// destination which imposes an encumbrance that requires a script that hashes
// to the same value as the provided script along with all of the encumbrances
// that script itself imposes using version 0 scripts.  The script is commonly
// referred to as a redeem script.
//
// See NewAddressScriptHashV0FromHash for a variant that accepts the hash of the
// script directly instead of the script.  It can be useful to callers that
// either already have the script hash available or do not know the associated
// script.
func NewAddressScriptHashV0(redeemScript []byte,
	params AddressParamsV0) (*AddressScriptHashV0, error) {

	scriptHash := Hash160(redeemScript)
	return NewAddressScriptHashV0FromHash(scriptHash, params)
}

// Address returns the string encoding of the payment address for the associated
// script version and payment script.
//
// This is part of the Address interface implementation.
func (addr *AddressScriptHashV0) Address() string {
	// The format for the data portion of addresses that encode 160-bit hashes
	// is merely the hash itself:
	//   20-byte ripemd160 hash
	return encodeAddressV0(addr.hash[:ripemd160.Size], addr.netID)
}

// putPaymentScript serializes the payment script associated with the address
// directly into the passed byte slice which must be at least
// p2shPaymentScriptLen bytes in length or it will panic.
func (addr *AddressScriptHashV0) putPaymentScript(script []byte) {
	// A pay-to-script-hash script is of the form:
	//  HASH160 <20-byte hash> EQUAL
	script[0] = opHash160
	script[1] = opData20
	copy(script[2:22], addr.hash[:])
	script[22] = opEqual
}

// PaymentScript returns the script version associated with the address along
// with a script to pay a transaction output to the address.
//
// This is part of the Address interface implementation.
func (addr *AddressScriptHashV0) PaymentScript() (uint16, []byte) {
	// A pay-to-script-hash script is of the form:
	//  HASH160 <20-byte hash> EQUAL
	var script [p2shPaymentScriptLen]byte
	addr.putPaymentScript(script[:])
	return 0, script[:]
}

// VotingRightsScript returns the script version associated with the address
// along with a script to give voting rights to the address.  It is only
// valid when used in stake ticket purchase transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) VotingRightsScript() (uint16, []byte) {
	// A script that assigns voting rights for a ticket to this address type is
	// of the form:
	//  SSTX [standard pay-to-script-hash script]
	var script [p2shPaymentScriptLen + 1]byte
	script[0] = opSSTx
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// RewardCommitmentScript returns the script version associated with the address
// along with a script that commits the original funds locked to purchase a
// ticket plus the reward to the address along with limits to impose on any
// fees.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) RewardCommitmentScript(amount int64, limits uint16) (uint16, []byte) {
	// The reward commitment output of a ticket purchase is a provably pruneable
	// script of the form:
	//   RETURN <20-byte hash || 8-byte amount || 2-byte fee limits>
	//
	// The high bit of the amount is used to indicate whether the provided hash
	// is a public key hash that represents a pay-to-pubkey-hash-ecdsa-secp256k1
	// script or a script hash that represents a pay-to-script-hash script.  It
	// is set for a script hash.
	var script [32]byte
	script[0] = opReturn
	script[1] = opData30
	copy(script[2:22], addr.hash[:])
	binary.LittleEndian.PutUint64(script[22:30], uint64(amount)|commitP2SHFlag)
	binary.LittleEndian.PutUint16(script[30:32], limits)
	return 0, script[:]
}

// StakeChangeScript returns the script version associated with the address
// along with a script to pay change to the address.  It is only valid when used
// in stake ticket purchase and treasury add transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) StakeChangeScript() (uint16, []byte) {
	// A stake change script to this address type is of the form:
	//  SSTXCHANGE [standard pay-to-script-hash script]
	var script [p2shPaymentScriptLen + 1]byte
	script[0] = opSSTxChange
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayVoteCommitmentScript returns the script version associated with the
// address along with a script to pay the original funds locked to purchase a
// ticket plus the reward to the address.  The address must have previously been
// committed to by the ticket purchase.  The script is only valid when used in
// stake vote transactions whose associated tickets are eligible to vote.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) PayVoteCommitmentScript() (uint16, []byte) {
	// A script that pays a ticket commitment as part of a vote to this address
	// type is of the form:
	//  SSGEN [standard pay-to-script-hash script]
	var script [p2shPaymentScriptLen + 1]byte
	script[0] = opSSGen
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayRevokeCommitmentScript returns the script version associated with the
// address along with a script to revoke an expired or missed ticket which pays
// the original funds locked to purchase a ticket to the address.  The address
// must have previously been committed to by the ticket purchase.  The script is
// only valid when used in stake revocation transactions whose associated
// tickets have been missed or expired.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) PayRevokeCommitmentScript() (uint16, []byte) {
	// A ticket revocation script to this address type is of the form:
	//  SSRTX [standard pay-to-script-hash script]
	var script [p2shPaymentScriptLen + 1]byte
	script[0] = opSSRTx
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// PayFromTreasuryScript returns the script version associated with the address
// along with a script that pays funds from the treasury to the address.  The
// script is only valid when used in treasury spend transactions.
//
// This is part of the StakeAddress interface implementation.
func (addr *AddressScriptHashV0) PayFromTreasuryScript() (uint16, []byte) {
	// A script that pays from the treasury as a part of a treasury spend to
	// this address type is of the form:
	//  TGEN [standard pay-to-script-hash script]
	var script [p2shPaymentScriptLen + 1]byte
	script[0] = opTGen
	addr.putPaymentScript(script[1:])
	return 0, script[:]
}

// Hash160 returns the underlying script hash.  This can be useful when an array
// is more appropriate than a slice (for example, when used as map keys).
func (addr *AddressScriptHashV0) Hash160() *[ripemd160.Size]byte {
	return &addr.hash
}

// String returns a human-readable string for the address.
//
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (addr *AddressScriptHashV0) String() string {
	return addr.Address()
}

// DecodeAddressV0 decodes the string encoding of an address and returns the
// relevant Address if it is a valid encoding for a known version 0 address type
// and is for the network identified by the provided parameters.
func DecodeAddressV0(addr string, params AddressParamsV0) (Address, error) {
	// Attempt to decode the address and address type.
	decoded, addrID, err := base58.CheckDecode(addr)
	if err != nil {
		kind := ErrMalformedAddress
		if errors.Is(err, base58.ErrChecksum) {
			kind = ErrBadAddressChecksum
		}
		str := fmt.Sprintf("failed to decoded address %q: %v", addr, err)
		return nil, makeError(kind, str)
	}

	// Decode the address according to the address type.
	switch addrID {
	case params.AddrIDScriptHashV0():
		return NewAddressScriptHashFromHash(0, decoded, params)

	case params.AddrIDPubKeyHashECDSAV0():
		return NewAddressPubKeyHashEcdsaSecp256k1(0, decoded, params)

	case params.AddrIDPubKeyHashSchnorrV0():
		return NewAddressPubKeyHashSchnorrSecp256k1(0, decoded, params)

	case params.AddrIDPubKeyHashEd25519V0():
		return NewAddressPubKeyHashEd25519(0, decoded, params)

	case params.AddrIDPubKeyV0():
		// Ensure the decoded data has the expected signature type identifier
		// byte.
		if len(decoded) < 1 {
			str := fmt.Sprintf("address %q decoded data is empty", addr)
			return nil, makeError(ErrMalformedAddressData, str)
		}

		// Decode according to the crypto algorithm and signature scheme.
		sigType := decoded[0] & ^sigTypeSecp256k1PubKeyCompOddFlag
		switch dcrec.SignatureType(sigType) {
		case dcrec.STEcdsaSecp256k1:
			// The encoded data for this case is the 32-byte X coordinate for a
			// secp256k1 public key along with the oddness of the Y coordinate
			// encoded via the high bit of the first byte.
			//
			// Reconstruct the standard compressed serialized public key format
			// by choosing the correct prefix byte depending on the encoded
			// Y-coordinate oddness pass it along to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			const reqPubKeyLen = 33
			if len(decoded) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(decoded), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}
			isOddY := decoded[0]&sigTypeSecp256k1PubKeyCompOddFlag != 0
			prefix := secp256k1.PubKeyFormatCompressedEven
			if isOddY {
				prefix = secp256k1.PubKeyFormatCompressedOdd
			}
			decoded[0] = prefix
			return NewAddressPubKeyEcdsaSecp256k1Raw(0, decoded, params)

		case dcrec.STEd25519:
			const reqPubKeyLen = 32
			pubKey := decoded[1:]
			if len(pubKey) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(pubKey), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}

			// The encoded data for this case is the actual Ed25519 public key,
			// so just pass it along unaltered to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			return NewAddressPubKeyEd25519Raw(0, pubKey, params)

		case dcrec.STSchnorrSecp256k1:
			// The encoded data for this case is the 32-byte X coordinate for a
			// secp256k1 public key along with the oddness of the Y coordinate
			// encoded via the high bit of the first byte.
			//
			// Reconstruct the standard compressed serialized public key format
			// by choosing the correct prefix byte depending on the encoded
			// Y-coordinate oddness pass it along to the constructor of the
			// appropriate type to validate and return the relevant address
			// instance.
			const reqPubKeyLen = 33
			if len(decoded) != reqPubKeyLen {
				str := fmt.Sprintf("public key is %d bytes vs required %d bytes",
					len(decoded), reqPubKeyLen)
				return nil, makeError(ErrMalformedAddressData, str)
			}
			isOddY := decoded[0]&sigTypeSecp256k1PubKeyCompOddFlag != 0
			prefix := secp256k1.PubKeyFormatCompressedEven
			if isOddY {
				prefix = secp256k1.PubKeyFormatCompressedOdd
			}
			decoded[0] = prefix
			return NewAddressPubKeySchnorrSecp256k1Raw(0, decoded, params)
		}
	}

	str := fmt.Sprintf("address %q is not a supported type", addr)
	return nil, makeError(ErrUnsupportedAddress, str)
}

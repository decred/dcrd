// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"errors"
	"fmt"

	"github.com/decred/base58"
	"github.com/decred/dcrd/chaincfg/v2/chainec"
	"github.com/decred/dcrd/crypto/ripemd160"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/edwards"
	"github.com/decred/dcrd/dcrec/secp256k1/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v2/schnorr"
)

var (
	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrUnknownAddressType describes an error where an address can not be
	// decoded as a specific address type due to the string encoding
	// beginning with unrecognized values that identify the network, type,
	// and signature algorithm.
	ErrUnknownAddressType = errors.New("unknown address type")
)

// encodeAddress returns a human-readable payment address given a ripemd160 hash
// and netID which encodes the network and address type.  It is used in both
// pay-to-pubkey-hash (P2PKH) and pay-to-script-hash (P2SH) address encoding.
func encodeAddress(hash160 []byte, netID [2]byte) string {
	// Format is 2 bytes for a network and address class (i.e. P2PKH vs
	// P2SH), 20 bytes for a RIPEMD160 hash, and 4 bytes of checksum.
	return base58.CheckEncode(hash160[:ripemd160.Size], netID)
}

// encodePKAddress returns a human-readable payment address to a public key
// given a serialized public key, a netID, and a signature suite.
func encodePKAddress(serializedPK []byte, netID [2]byte, algo dcrec.SignatureType) string {
	pubKeyBytes := []byte{0x00}

	switch algo {
	case dcrec.STEcdsaSecp256k1:
		pubKeyBytes[0] = byte(dcrec.STEcdsaSecp256k1)
	case dcrec.STEd25519:
		pubKeyBytes[0] = byte(dcrec.STEd25519)
	case dcrec.STSchnorrSecp256k1:
		pubKeyBytes[0] = byte(dcrec.STSchnorrSecp256k1)
	}

	// Pubkeys are encoded as [0] = type/ybit, [1:33] = serialized pubkey
	compressed := serializedPK
	if algo == dcrec.STEcdsaSecp256k1 || algo == dcrec.STSchnorrSecp256k1 {
		pub, err := secp256k1.ParsePubKey(serializedPK)
		if err != nil {
			return ""
		}
		pubSerComp := pub.SerializeCompressed()

		// Set the y-bit if needed.
		if pubSerComp[0] == 0x03 {
			pubKeyBytes[0] |= (1 << 7)
		}

		compressed = pubSerComp[1:]
	}

	pubKeyBytes = append(pubKeyBytes, compressed...)
	return base58.CheckEncode(pubKeyBytes, netID)
}

// AddressParams defines an interface that is used to provide the parameters
// required when encoding and decoding addresses.  These values are typically
// well-defined and unique per network.
type AddressParams interface {
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

// Address is an interface type for any type of destination a transaction
// output may spend to.  This includes pay-to-pubkey (P2PK), pay-to-pubkey-hash
// (P2PKH), and pay-to-script-hash (P2SH).  Address is designed to be generic
// enough that other kinds of addresses may be added in the future without
// changing the decoding and encoding API.
type Address interface {
	// String returns the string encoding of the transaction output
	// destination.
	//
	// Please note that String differs subtly from Address: String will
	// return the value as a string without any conversion, while Address
	// may convert destination types (for example, converting pubkeys to
	// P2PKH addresses) before encoding as a payment address string.
	String() string

	// Address returns the string encoding of the payment address associated
	// with the Address value.  See the comment on String for how this
	// method differs from String.
	Address() string

	// ScriptAddress returns the raw bytes of the address to be used
	// when inserting the address into a txout's script.
	ScriptAddress() []byte

	// Hash160 returns the Hash160(data) where data is the data normally
	// hashed to 160 bits from the respective address type.
	Hash160() *[ripemd160.Size]byte
}

// NewAddressPubKey returns a new Address. decoded must
// be 33 bytes.
func NewAddressPubKey(decoded []byte, net AddressParams) (Address, error) {
	if len(decoded) == 33 {
		// First byte is the signature suite and ybit.
		suite := decoded[0]
		suite &= ^uint8(1 << 7)
		ybit := !(decoded[0]&(1<<7) == 0)
		toAppend := uint8(0x02)
		if ybit {
			toAppend = 0x03
		}

		switch dcrec.SignatureType(suite) {
		case dcrec.STEcdsaSecp256k1:
			return NewAddressSecpPubKey(
				append([]byte{toAppend}, decoded[1:]...),
				net)
		case dcrec.STEd25519:
			return NewAddressEdwardsPubKey(decoded, net)
		case dcrec.STSchnorrSecp256k1:
			return NewAddressSecSchnorrPubKey(
				append([]byte{toAppend}, decoded[1:]...),
				net)
		}
		return nil, ErrUnknownAddressType
	}
	return nil, ErrUnknownAddressType
}

// DecodeAddress decodes the string encoding of an address and returns the
// Address if it is a valid encoding for a known address type and is for the
// provided network.
func DecodeAddress(addr string, net AddressParams) (Address, error) {
	// Switch on decoded length to determine the type.
	decoded, netID, err := base58.CheckDecode(addr)
	if err != nil {
		if err == base58.ErrChecksum {
			return nil, ErrChecksumMismatch
		}
		return nil, fmt.Errorf("decoded address is of unknown format: %v", err)
	}

	switch netID {
	case net.AddrIDPubKeyV0():
		return NewAddressPubKey(decoded, net)

	case net.AddrIDPubKeyHashECDSAV0():
		return NewAddressPubKeyHash(decoded, net, dcrec.STEcdsaSecp256k1)

	case net.AddrIDPubKeyHashEd25519V0():
		return NewAddressPubKeyHash(decoded, net, dcrec.STEd25519)

	case net.AddrIDPubKeyHashSchnorrV0():
		return NewAddressPubKeyHash(decoded, net, dcrec.STSchnorrSecp256k1)

	case net.AddrIDScriptHashV0():
		return NewAddressScriptHashFromHash(decoded, net)

	default:
		return nil, ErrUnknownAddressType
	}
}

// AddressPubKeyHash is an Address for a pay-to-pubkey-hash (P2PKH)
// transaction.
type AddressPubKeyHash struct {
	hash  [ripemd160.Size]byte
	netID [2]byte
	dsa   dcrec.SignatureType
}

// NewAddressPubKeyHash returns a new AddressPubKeyHash.  pkHash must
// be 20 bytes.
func NewAddressPubKeyHash(pkHash []byte, net AddressParams, algo dcrec.SignatureType) (*AddressPubKeyHash, error) {
	// Ensure the provided signature algo is supported.
	var addrID [2]byte
	switch algo {
	case dcrec.STEcdsaSecp256k1:
		addrID = net.AddrIDPubKeyHashECDSAV0()
	case dcrec.STEd25519:
		addrID = net.AddrIDPubKeyHashEd25519V0()
	case dcrec.STSchnorrSecp256k1:
		addrID = net.AddrIDPubKeyHashSchnorrV0()
	default:
		return nil, errors.New("unknown signature algorithm")
	}

	// Ensure the provided pubkey hash length is valid.
	if len(pkHash) != ripemd160.Size {
		return nil, errors.New("pkHash must be 20 bytes")
	}
	addr := &AddressPubKeyHash{netID: addrID, dsa: algo}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// Address returns the string encoding of a pay-to-pubkey-hash address.
//
// Part of the Address interface.
func (a *AddressPubKeyHash) Address() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a pubkey hash.  Part of the Address interface.
func (a *AddressPubKeyHash) ScriptAddress() []byte {
	return a.hash[:]
}

// String returns a human-readable string for the pay-to-pubkey-hash address.
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (a *AddressPubKeyHash) String() string {
	return a.Address()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressPubKeyHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// DSA returns the digital signature algorithm for the associated public key
// hash.
func (a *AddressPubKeyHash) DSA() dcrec.SignatureType {
	return a.dsa
}

// AddressScriptHash is an Address for a pay-to-script-hash (P2SH)
// transaction.
type AddressScriptHash struct {
	hash  [ripemd160.Size]byte
	netID [2]byte
}

// NewAddressScriptHash returns a new AddressScriptHash.
func NewAddressScriptHash(serializedScript []byte, net AddressParams) (*AddressScriptHash, error) {
	scriptHash := Hash160(serializedScript)
	return newAddressScriptHashFromHash(scriptHash, net.AddrIDScriptHashV0())
}

// NewAddressScriptHashFromHash returns a new AddressScriptHash.  scriptHash
// must be 20 bytes.
func NewAddressScriptHashFromHash(scriptHash []byte, net AddressParams) (*AddressScriptHash, error) {
	ash, err := newAddressScriptHashFromHash(scriptHash, net.AddrIDScriptHashV0())
	if err != nil {
		return nil, err
	}

	return ash, nil
}

// newAddressScriptHashFromHash is the internal API to create a script hash
// address with a known leading identifier byte for a network, rather than
// looking it up through its parameters.  This is useful when creating a new
// address structure from a string encoding where the identifier byte is already
// known.
func newAddressScriptHashFromHash(scriptHash []byte, netID [2]byte) (*AddressScriptHash, error) {
	// Check for a valid script hash length.
	if len(scriptHash) != ripemd160.Size {
		return nil, errors.New("scriptHash must be 20 bytes")
	}

	addr := &AddressScriptHash{netID: netID}
	copy(addr.hash[:], scriptHash)
	return addr, nil
}

// Address returns the string encoding of a pay-to-script-hash address.
//
// Part of the Address interface.
func (a *AddressScriptHash) Address() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a script hash.  Part of the Address interface.
func (a *AddressScriptHash) ScriptAddress() []byte {
	return a.hash[:]
}

// String returns a human-readable string for the pay-to-script-hash address.
// This is equivalent to calling Address, but is provided so the type can be
// used as a fmt.Stringer.
func (a *AddressScriptHash) String() string {
	return a.Address()
}

// Hash160 returns the underlying array of the script hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressScriptHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// PubKeyFormat describes what format to use for a pay-to-pubkey address.
type PubKeyFormat int

const (
	// PKFUncompressed indicates the pay-to-pubkey address format is an
	// uncompressed public key.
	PKFUncompressed PubKeyFormat = iota

	// PKFCompressed indicates the pay-to-pubkey address format is a
	// compressed public key.
	PKFCompressed
)

// ErrInvalidPubKeyFormat indicates that a serialized pubkey is unusable as it
// is neither in the uncompressed or compressed format.
var ErrInvalidPubKeyFormat = errors.New("invalid pubkey format")

// AddressSecpPubKey is an Address for a secp256k1 pay-to-pubkey transaction.
type AddressSecpPubKey struct {
	pubKeyFormat PubKeyFormat
	pubKey       chainec.PublicKey
	pubKeyID     [2]byte
	pubKeyHashID [2]byte
}

// NewAddressSecpPubKey returns a new AddressSecpPubKey which represents a
// pay-to-pubkey address, using a secp256k1 pubkey.  The serializedPubKey
// parameter must be a valid pubkey and must be uncompressed or compressed.
func NewAddressSecpPubKey(serializedPubKey []byte, net AddressParams) (*AddressSecpPubKey, error) {
	pubKey, err := secp256k1.ParsePubKey(serializedPubKey)
	if err != nil {
		return nil, err
	}

	// Set the format of the pubkey.  This probably should be returned
	// from dcrec, but do it here to avoid API churn.  We already know the
	// pubkey is valid since it parsed above, so it's safe to simply examine
	// the leading byte to get the format.
	var pkFormat PubKeyFormat
	switch serializedPubKey[0] {
	case 0x02, 0x03:
		pkFormat = PKFCompressed
	case 0x04:
		pkFormat = PKFUncompressed
	default:
		return nil, ErrInvalidPubKeyFormat
	}

	return &AddressSecpPubKey{
		pubKeyFormat: pkFormat,
		pubKey:       pubKey,
		pubKeyID:     net.AddrIDPubKeyV0(),
		pubKeyHashID: net.AddrIDPubKeyHashECDSAV0(),
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressSecpPubKey) serialize() []byte {
	switch a.pubKeyFormat {
	default:
		fallthrough
	case PKFUncompressed:
		return a.pubKey.SerializeUncompressed()

	case PKFCompressed:
		return a.pubKey.SerializeCompressed()
	}
}

// Address returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the compressed public key.
//
// Part of the Address interface.
func (a *AddressSecpPubKey) Address() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressSecpPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressSecpPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.SerializeCompressed())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling Address.
func (a *AddressSecpPubKey) String() string {
	return encodePKAddress(a.serialize(), a.pubKeyID, dcrec.STEcdsaSecp256k1)
}

// Format returns the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressSecpPubKey) Format() PubKeyFormat {
	return a.pubKeyFormat
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
func (a *AddressSecpPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{netID: a.pubKeyHashID,
		dsa: dcrec.STEcdsaSecp256k1}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// PubKey returns the underlying public key for the address.
func (a *AddressSecpPubKey) PubKey() chainec.PublicKey {
	return a.pubKey
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressSecpPubKey) DSA() dcrec.SignatureType {
	return dcrec.STEcdsaSecp256k1
}

// NewAddressSecpPubKeyCompressed creates a new address using a compressed public key
func NewAddressSecpPubKeyCompressed(pubkey chainec.PublicKey, params AddressParams) (*AddressSecpPubKey, error) {
	return NewAddressSecpPubKey(pubkey.SerializeCompressed(), params)
}

// AddressEdwardsPubKey is an Address for an Ed25519 pay-to-pubkey transaction.
type AddressEdwardsPubKey struct {
	pubKey       chainec.PublicKey
	pubKeyID     [2]byte
	pubKeyHashID [2]byte
}

// NewAddressEdwardsPubKey returns a new AddressEdwardsPubKey which represents a
// pay-to-pubkey address, using an Ed25519 pubkey.  The serializedPubKey
// parameter must be a valid 32 byte serialized public key.
func NewAddressEdwardsPubKey(serializedPubKey []byte, net AddressParams) (*AddressEdwardsPubKey, error) {
	pubKey, err := edwards.ParsePubKey(edwards.Edwards(), serializedPubKey)
	if err != nil {
		return nil, err
	}

	return &AddressEdwardsPubKey{
		pubKey:       pubKey,
		pubKeyID:     net.AddrIDPubKeyV0(),
		pubKeyHashID: net.AddrIDPubKeyHashEd25519V0(),
	}, nil
}

// serialize returns the serialization of the public key.
func (a *AddressEdwardsPubKey) serialize() []byte {
	return a.pubKey.Serialize()
}

// Address returns the string encoding of the public key as a
// pay-to-pubkey-hash.
//
// Part of the Address interface.
func (a *AddressEdwardsPubKey) Address() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressEdwardsPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressEdwardsPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.Serialize())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling Address.
func (a *AddressEdwardsPubKey) String() string {
	return encodePKAddress(a.serialize(), a.pubKeyID, dcrec.STEd25519)
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.
func (a *AddressEdwardsPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{netID: a.pubKeyHashID, dsa: dcrec.STEd25519}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// PubKey returns the underlying public key for the address.
func (a *AddressEdwardsPubKey) PubKey() chainec.PublicKey {
	return a.pubKey
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressEdwardsPubKey) DSA() dcrec.SignatureType {
	return dcrec.STEd25519
}

// AddressSecSchnorrPubKey is an Address for a secp256k1 pay-to-pubkey
// transaction.
type AddressSecSchnorrPubKey struct {
	pubKey       chainec.PublicKey
	pubKeyID     [2]byte
	pubKeyHashID [2]byte
}

// NewAddressSecSchnorrPubKey returns a new AddressSecpPubKey which represents a
// pay-to-pubkey address, using a secp256k1 pubkey.  The serializedPubKey
// parameter must be a valid pubkey and must be compressed.
func NewAddressSecSchnorrPubKey(serializedPubKey []byte, net AddressParams) (*AddressSecSchnorrPubKey, error) {
	pubKey, err := schnorr.ParsePubKey(serializedPubKey)
	if err != nil {
		return nil, err
	}

	return &AddressSecSchnorrPubKey{
		pubKey:       pubKey,
		pubKeyID:     net.AddrIDPubKeyV0(),
		pubKeyHashID: net.AddrIDPubKeyHashSchnorrV0(),
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressSecSchnorrPubKey) serialize() []byte {
	return a.pubKey.Serialize()
}

// Address returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Decred addresses
// are pay-to-pubkey-hash constructed from the compressed public key.
//
// Part of the Address interface.
func (a *AddressSecSchnorrPubKey) Address() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressSecSchnorrPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropriate than a slice (for example, when used as map
// keys).
func (a *AddressSecSchnorrPubKey) Hash160() *[ripemd160.Size]byte {
	h160 := Hash160(a.pubKey.Serialize())
	array := new([ripemd160.Size]byte)
	copy(array[:], h160)

	return array
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling Address.
func (a *AddressSecSchnorrPubKey) String() string {
	return encodePKAddress(a.serialize(), a.pubKeyID, dcrec.STSchnorrSecp256k1)
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.
func (a *AddressSecSchnorrPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{netID: a.pubKeyHashID,
		dsa: dcrec.STSchnorrSecp256k1}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// DSA returns the underlying digital signature algorithm for the
// address.
func (a *AddressSecSchnorrPubKey) DSA() dcrec.SignatureType {
	return dcrec.STSchnorrSecp256k1
}

// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"hash"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

const (
	// MaxMixPairReqScriptClassLen is the maximum length allowable for a
	// MsgMixPairReq script class.
	MaxMixPairReqScriptClassLen = 32

	// MaxMixPairReqUTXOs is the maximum number of unspent transaction
	// outputs that may be contributed in a single mixpairreq message.
	// This value is an high estimate of what a large mix may look like,
	// based on statistics from the centralized mixing server.
	MaxMixPairReqUTXOs = 512

	// MaxMixPairReqUTXOScriptLen is the maximum length allowed for the
	// unhashed P2SH script of a UTXO ownership proof.
	MaxMixPairReqUTXOScriptLen = 16384 // txscript.MaxScriptSize

	// MaxMixPairReqUTXOPubKeyLen is the maximum length allowed for the
	// pubkey of a UTXO ownership proof.
	MaxMixPairReqUTXOPubKeyLen = 33

	// MaxMixPairReqUTXOSignatureLen is the maximum length allowed for the
	// signature of a UTXO ownership proof.
	MaxMixPairReqUTXOSignatureLen = 64
)

// MixPairReqUTXO describes an unspent transaction output to be spent in a
// mix.  It includes a proof that the output is able to be spent, by
// containing a signature and the necessary data needed to prove key
// possession.
type MixPairReqUTXO struct {
	OutPoint  OutPoint
	Script    []byte // Only used for P2SH
	PubKey    []byte
	Signature []byte
}

// MsgMixPairReq implements the Message interface and represents a mixing pair
// request message.  It describes a type of coinjoin to be created, unmixed
// data being contributed to the coinjoin, and proof of ability to sign the
// resulting coinjoin.
type MsgMixPairReq struct {
	Signature    [64]byte
	Identity     [33]byte
	Expiry       uint32
	MixAmount    int64
	ScriptClass  string
	TxVersion    uint16
	LockTime     uint32
	MessageCount uint32
	InputValue   int64
	UTXOs        []MixPairReqUTXO
	Change       *TxOut

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// Pairing returns a description of the coinjoin transaction being created.
// Different mixpairreq messages are compatible to perform a mix together if
// their pairing descriptions are identical.
func (msg *MsgMixPairReq) Pairing() ([]byte, error) {
	bufLen := 8 + // Mix amount
		VarIntSerializeSize(uint64(len(msg.ScriptClass))) + // Script class
		len(msg.ScriptClass) +
		2 + // Tx version
		4 // Locktime
	w := bytes.NewBuffer(make([]byte, 0, bufLen))

	err := writeElement(w, msg.MixAmount)
	if err != nil {
		return nil, err
	}

	err = WriteVarString(w, MixVersion, msg.ScriptClass)
	if err != nil {
		return nil, err
	}

	err = writeElements(w, msg.TxVersion, msg.LockTime)
	if err != nil {
		return nil, err
	}

	return w.Bytes(), nil
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixPairReq) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixPairReq.BtcDecode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := readElements(r, &msg.Signature, &msg.Identity, &msg.Expiry,
		&msg.MixAmount)
	if err != nil {
		return err
	}

	if msg.MixAmount < 0 {
		msg := "mixing pair request contains negative mixed amount"
		return messageError(op, ErrInvalidMsg, msg)
	}

	sc, err := ReadAsciiVarString(r, pver, MaxMixPairReqScriptClassLen)
	if err != nil {
		return err
	}
	msg.ScriptClass = sc

	err = readElements(r, &msg.TxVersion, &msg.LockTime,
		&msg.MessageCount, &msg.InputValue)
	if err != nil {
		return err
	}

	if msg.InputValue < 0 {
		msg := "mixing pair request contains negative input value"
		return messageError(op, ErrInvalidMsg, msg)
	}

	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxMixPairReqUTXOs {
		msg := fmt.Sprintf("too many UTXOs in message [count %v, max %v]",
			count, MaxMixPairReqUTXOs)
		return messageError(op, ErrTooManyMixPairReqUTXOs, msg)
	}
	utxos := make([]MixPairReqUTXO, count)
	for i := range utxos {
		utxo := &utxos[i]

		err := ReadOutPoint(r, pver, msg.TxVersion, &utxo.OutPoint)
		if err != nil {
			return err
		}

		script, err := ReadVarBytes(r, pver, MaxMixPairReqUTXOScriptLen,
			"MixPairReqUTXO.Script")
		if err != nil {
			return err
		}
		utxo.Script = script

		pubkey, err := ReadVarBytes(r, pver, MaxMixPairReqUTXOPubKeyLen,
			"MixPairReqUTXO.PubKey")
		if err != nil {
			return err
		}
		utxo.PubKey = pubkey

		sig, err := ReadVarBytes(r, pver, MaxMixPairReqUTXOSignatureLen,
			"MixPairReqUTXO.Signature")
		if err != nil {
			return err
		}
		utxo.Signature = sig
	}
	msg.UTXOs = utxos

	var hasChange uint8
	err = readElement(r, &hasChange)
	if err != nil {
		return err
	}
	switch hasChange {
	case 0:
	case 1:
		change := new(TxOut)
		err := readTxOut(r, pver, msg.TxVersion, change)
		if err != nil {
			return err
		}
		msg.Change = change
	default:
		msg := "invalid change TxOut encoding"
		return messageError(op, ErrInvalidMsg, msg)
	}

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixPairReq) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixPairReq.BtcEncode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := writeElement(w, &msg.Signature)
	if err != nil {
		return err
	}

	err = msg.writeMessageNoSignature(op, w, pver)
	if err != nil {
		return err
	}

	return nil
}

// Hash returns the message hash calculated by WriteHash.
//
// Hash returns an invalid or zero hash if WriteHash has not been called yet.
//
// This method is not safe while concurrently calling WriteHash.
func (msg *MsgMixPairReq) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixPairReq) WriteHash(h hash.Hash) {
	h.Reset()
	writeElement(h, &msg.Signature)
	msg.writeMessageNoSignature("", h, MixVersion)
	sum := h.Sum(msg.hash[:0])
	if len(sum) != len(msg.hash) {
		s := fmt.Sprintf("hasher type %T has invalid Size() for chainhash.Hash", h)
		panic(s)
	}
}

// writeMessageNoSignature serializes all elements of the message except for
// the signature.  This allows code reuse between message serialization, and
// signing and verifying these message contents.
//
// If w implements hash.Hash, no errors will be returned for invalid message
// construction.
func (msg *MsgMixPairReq) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	// Require script class to be strict ASCII and not exceed the maximum
	// length.
	lenScriptClass := len(msg.ScriptClass)
	if lenScriptClass > MaxMixPairReqScriptClassLen {
		msg := fmt.Sprintf("script class length is too long "+
			"[len %d, max %d]", lenScriptClass,
			MaxMixPairReqScriptClassLen)
		return messageError(op, ErrMixPairReqScriptClassTooLong, msg)
	}
	if !isStrictAscii(msg.ScriptClass) {
		msg := "script class string is not strict ASCII"
		return messageError(op, ErrMalformedStrictString, msg)
	}

	// Limit to max UTXOs per message.
	count := len(msg.UTXOs)
	if !hashing && count > MaxMixPairReqUTXOs {
		msg := fmt.Sprintf("too many UTXOs in message [%v]", count)
		return messageError(op, ErrTooManyMixPairReqUTXOs, msg)
	}

	err := writeElements(w, &msg.Identity, msg.Expiry, msg.MixAmount)
	if err != nil {
		return err
	}

	err = WriteVarString(w, pver, msg.ScriptClass)
	if err != nil {
		return err
	}

	err = writeElements(w, msg.TxVersion, msg.LockTime, msg.MessageCount,
		msg.InputValue)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for i := range msg.UTXOs {
		utxo := &msg.UTXOs[i]

		err := WriteOutPoint(w, pver, msg.TxVersion, &utxo.OutPoint)
		if err != nil {
			return err
		}

		if l := len(utxo.Script); !hashing && l > MaxMixPairReqUTXOScriptLen {
			msg := fmt.Sprintf("UTXO script is too long [len %v, max %v]",
				l, MaxMixPairReqUTXOScriptLen)
			return messageError(op, ErrVarBytesTooLong, msg)
		}
		err = WriteVarBytes(w, pver, utxo.Script)
		if err != nil {
			return err
		}

		if l := len(utxo.PubKey); !hashing && l > MaxMixPairReqUTXOPubKeyLen {
			msg := fmt.Sprintf("UTXO public key is too long [len %v, max %v]",
				l, MaxMixPairReqUTXOPubKeyLen)
			return messageError(op, ErrVarBytesTooLong, msg)
		}
		err = WriteVarBytes(w, pver, utxo.PubKey)
		if err != nil {
			return err
		}

		if l := len(utxo.Signature); !hashing && l > MaxMixPairReqUTXOSignatureLen {
			msg := fmt.Sprintf("UTXO signature is too long [len %v, max %v]",
				l, MaxMixPairReqUTXOSignatureLen)
			return messageError(op, ErrVarBytesTooLong, msg)
		}
		err = WriteVarBytes(w, pver, utxo.Signature)
		if err != nil {
			return err
		}
	}

	var hasChange uint8
	if msg.Change != nil {
		hasChange = 1
	}
	err = writeElement(w, hasChange)
	if err != nil {
		return err
	}
	if msg.Change != nil {
		err = writeTxOut(w, pver, msg.TxVersion, msg.Change)
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixPairReq) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixPairReq+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixPairReq) Command() string {
	return CmdMixPairReq
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixPairReq) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 8476336
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixPairReq) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixPairReq) Sig() []byte {
	return msg.Signature[:]
}

// Expires returns the block height at which the message expires.
func (msg *MsgMixPairReq) Expires() uint32 {
	return msg.Expiry
}

// PrevMsgs always returns nil.  Pair request messages are the initial message.
func (msg *MsgMixPairReq) PrevMsgs() []chainhash.Hash {
	return nil
}

// Sid always returns nil.  Pair request messages do not belong to a session.
func (msg *MsgMixPairReq) Sid() []byte {
	return nil
}

// GetRun always returns 0.  Pair request messages do not belong to a session.
func (msg *MsgMixPairReq) GetRun() uint32 {
	return 0
}

// NewMsgMixPairReq returns a new mixpairreq message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixPairReq(identity [33]byte, expiry uint32, mixAmount int64,
	scriptClass string, txVersion uint16, lockTime, messageCount uint32,
	inputValue int64, utxos []MixPairReqUTXO, change *TxOut) (*MsgMixPairReq, error) {

	const op = "NewMsgMixPairReq"
	lenScriptClass := len(scriptClass)
	if lenScriptClass > MaxMixPairReqScriptClassLen {
		msg := fmt.Sprintf("script class length is too long "+
			"[len %d, max %d]", lenScriptClass,
			MaxMixPairReqScriptClassLen)
		return nil, messageError(op, ErrMixPairReqScriptClassTooLong, msg)
	}

	if !isStrictAscii(scriptClass) {
		msg := "script class string is not strict ASCII"
		return nil, messageError(op, ErrMalformedStrictString, msg)
	}

	if len(utxos) > MaxMixPairReqUTXOs {
		msg := fmt.Sprintf("too many input UTXOs [len %d, max %d]",
			len(utxos), MaxMixPairReqUTXOs)
		return nil, messageError(op, ErrTooManyMixPairReqUTXOs, msg)
	}

	msg := &MsgMixPairReq{
		Identity:     identity,
		Expiry:       expiry,
		MixAmount:    mixAmount,
		ScriptClass:  scriptClass,
		TxVersion:    txVersion,
		LockTime:     lockTime,
		MessageCount: messageCount,
		InputValue:   inputValue,
		UTXOs:        utxos,
		Change:       change,
	}
	return msg, nil
}

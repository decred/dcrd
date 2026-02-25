// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MessageHeaderSize is the number of bytes in a Decred message header.
// Decred network (magic) 4 bytes + command 12 bytes + payload length 4 bytes +
// checksum 4 bytes.
const MessageHeaderSize = 24

// CommandSize is the fixed size of all commands in the common Decred message
// header.  Shorter commands must be zero padded.
const CommandSize = 12

// MaxMessagePayload is the maximum bytes a message can be regardless of other
// individual limits imposed by messages themselves.
const MaxMessagePayload = (1024 * 1024 * 32) // 32MB

// Commands used in message headers which describe the type of message.
const (
	CmdVersion         = "version"
	CmdVerAck          = "verack"
	CmdGetAddr         = "getaddr"
	CmdAddr            = "addr"
	CmdAddrV2          = "addrv2"
	CmdGetBlocks       = "getblocks"
	CmdInv             = "inv"
	CmdGetData         = "getdata"
	CmdNotFound        = "notfound"
	CmdBlock           = "block"
	CmdTx              = "tx"
	CmdGetHeaders      = "getheaders"
	CmdHeaders         = "headers"
	CmdPing            = "ping"
	CmdPong            = "pong"
	CmdMemPool         = "mempool"
	CmdMiningState     = "miningstate"
	CmdGetMiningState  = "getminings"
	CmdReject          = "reject"
	CmdSendHeaders     = "sendheaders"
	CmdFeeFilter       = "feefilter"
	CmdGetCFilterV2    = "getcfilterv2"
	CmdCFilterV2       = "cfilterv2"
	CmdGetInitState    = "getinitstate"
	CmdInitState       = "initstate"
	CmdMixPairReq      = "mixpairreq"
	CmdMixKeyExchange  = "mixkeyxchg"
	CmdMixCiphertexts  = "mixcphrtxt"
	CmdMixSlotReserve  = "mixslotres"
	CmdMixFactoredPoly = "mixfactpoly"
	CmdMixDCNet        = "mixdcnet"
	CmdMixConfirm      = "mixconfirm"
	CmdMixSecrets      = "mixsecrets"
	CmdGetCFiltersV2   = "getcfsv2"
	CmdCFiltersV2      = "cfiltersv2"
)

const (
	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdGetCFilter = "getcfilter"

	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdGetCFHeaders = "getcfheaders"

	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdGetCFTypes = "getcftypes"

	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdCFilter = "cfilter"

	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdCFHeaders = "cfheaders"

	// Deprecated: This command is no longer valid as of protocol version
	// CFilterV2Version.
	CmdCFTypes = "cftypes"
)

// Message is an interface that describes a Decred message.  A type that
// implements Message has complete control over the representation of its data
// and may therefore contain additional or fewer fields than those which
// are used directly in the protocol encoded message.
type Message interface {
	BtcDecode(io.Reader, uint32) error
	BtcEncode(io.Writer, uint32) error
	Command() string
	MaxPayloadLength(uint32) uint32
}

// makeEmptyMessage creates a message of the appropriate concrete type based
// on the command.
func makeEmptyMessage(command []byte) (Message, error) {
	const op = "makeEmptyMessage"

	var msg Message
	switch string(command) {
	case CmdVersion:
		msg = &MsgVersion{}

	case CmdVerAck:
		msg = &MsgVerAck{}

	case CmdGetAddr:
		msg = &MsgGetAddr{}

	case CmdAddr:
		msg = &MsgAddr{}

	case CmdAddrV2:
		msg = &MsgAddrV2{}

	case CmdGetBlocks:
		msg = &MsgGetBlocks{}

	case CmdBlock:
		msg = &MsgBlock{}

	case CmdInv:
		msg = &MsgInv{}

	case CmdGetData:
		msg = &MsgGetData{}

	case CmdNotFound:
		msg = &MsgNotFound{}

	case CmdTx:
		msg = &MsgTx{}

	case CmdPing:
		msg = &MsgPing{}

	case CmdPong:
		msg = &MsgPong{}

	case CmdGetHeaders:
		msg = &MsgGetHeaders{}

	case CmdHeaders:
		msg = &MsgHeaders{}

	case CmdMemPool:
		msg = &MsgMemPool{}

	case CmdMiningState:
		msg = &MsgMiningState{}

	case CmdGetMiningState:
		msg = &MsgGetMiningState{}

	case CmdSendHeaders:
		msg = &MsgSendHeaders{}

	case CmdFeeFilter:
		msg = &MsgFeeFilter{}

	case CmdGetCFilter:
		msg = &MsgGetCFilter{}

	case CmdGetCFHeaders:
		msg = &MsgGetCFHeaders{}

	case CmdGetCFTypes:
		msg = &MsgGetCFTypes{}

	case CmdCFilter:
		msg = &MsgCFilter{}

	case CmdCFHeaders:
		msg = &MsgCFHeaders{}

	case CmdCFTypes:
		msg = &MsgCFTypes{}

	case CmdGetCFilterV2:
		msg = &MsgGetCFilterV2{}

	case CmdCFilterV2:
		msg = &MsgCFilterV2{}

	case CmdGetInitState:
		msg = &MsgGetInitState{}

	case CmdInitState:
		msg = &MsgInitState{}

	case CmdMixPairReq:
		msg = &MsgMixPairReq{}

	case CmdMixKeyExchange:
		msg = &MsgMixKeyExchange{}

	case CmdMixCiphertexts:
		msg = &MsgMixCiphertexts{}

	case CmdMixSlotReserve:
		msg = &MsgMixSlotReserve{}

	case CmdMixFactoredPoly:
		msg = &MsgMixFactoredPoly{}

	case CmdMixDCNet:
		msg = &MsgMixDCNet{}

	case CmdMixConfirm:
		msg = &MsgMixConfirm{}

	case CmdMixSecrets:
		msg = &MsgMixSecrets{}

	case CmdGetCFiltersV2:
		msg = &MsgGetCFsV2{}

	case CmdCFiltersV2:
		msg = &MsgCFiltersV2{}

	default:
		str := fmt.Sprintf("unhandled command [%s]", string(command))
		return nil, messageError(op, ErrUnknownCmd, str)
	}
	return msg, nil
}

// WriteMessageN writes a Decred Message to w including the necessary header
// information and returns the number of bytes written.    This function is the
// same as WriteMessage except it also returns the number of bytes written.
func WriteMessageN(w io.Writer, msg Message, pver uint32, dcrnet CurrencyNet) (int, error) {
	const op = "WriteMessage"

	var (
		command  [CommandSize]byte
		lenp     uint32
		checksum [4]byte
	)

	// Enforce max command size.
	cmd := msg.Command()
	if len(cmd) > CommandSize {
		msg := fmt.Sprintf("command [%s] is too long [max %v]", cmd, CommandSize)
		return 0, messageError(op, ErrCmdTooLong, msg)
	}
	copy(command[:], []byte(cmd))

	// Allocate enough buffer space for the entire message size if it is
	// known.  When it is not known, use an extra size hint of 64 bytes,
	// which matches the default small allocation size of a bytes.Buffer
	// as of Go 1.25.
	extraCap := 64
	switch msg := msg.(type) {
	case interface{ SerializeSize() int }:
		extraCap = msg.SerializeSize()
	}

	// Initialize buffer with zeroed bytes for the message header (to be
	// filled in, with checksum, after appending the payload
	// serialization), plus additional capacity for writing the payload.
	buf := bytes.NewBuffer(make([]byte, MessageHeaderSize, MessageHeaderSize+extraCap))

	// Encode the message payload.
	err := msg.BtcEncode(buf, pver)
	if err != nil {
		return 0, err
	}
	bufBytes := buf.Bytes()
	payload := bufBytes[MessageHeaderSize:]

	// Enforce maximum overall message payload.
	if len(payload) > MaxMessagePayload {
		msg := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			len(payload), MaxMessagePayload)
		return 0, messageError(op, ErrPayloadTooLarge, msg)
	}
	lenp = uint32(len(payload))

	// Enforce maximum message payload based on the message type.
	mpl := msg.MaxPayloadLength(pver)
	if lenp > mpl {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload size for "+
			"messages of type [%s] is %d.", lenp, cmd, mpl)
		return 0, messageError(op, ErrPayloadTooLarge, str)
	}

	// Encode the message header.
	cksumHash := chainhash.HashH(payload)
	copy(checksum[:], cksumHash[0:4])
	buf.Reset()
	writeUint32LE(buf, uint32(dcrnet))
	buf.Write(command[:])
	writeUint32LE(buf, lenp)
	buf.Write(checksum[:])
	if buf.Len() != MessageHeaderSize {
		// The length of data written for the header is always
		// constant, is not dependent on the message being serialized,
		// and any implementation errors that cause an incorrect
		// length to be written would be discovered by tests.
		str := fmt.Sprintf("wrote unexpected message header length - "+
			"encoded %d bytes, but message header size is %d.",
			buf.Len(), MessageHeaderSize)
		panic(str)
	}

	// Write header + payload.
	return w.Write(bufBytes)
}

// WriteMessage writes a Decred Message to w including the necessary header
// information.  This function is the same as WriteMessageN except it doesn't
// return the number of bytes written.  This function is mainly provided for
// backwards compatibility with the original API, but it's also useful for
// callers that don't care about byte counts.
func WriteMessage(w io.Writer, msg Message, pver uint32, dcrnet CurrencyNet) error {
	_, err := WriteMessageN(w, msg, pver, dcrnet)
	return err
}

// wireBuffer is a bytes.Buffer uniquely used by ReadMessageN.  The distinct
// type is used to optimize reads of transaction scripts by avoiding the
// scriptPool freelist when the buffer is known to not be clobbered by the
// caller.
type wireBuffer bytes.Buffer

func (b *wireBuffer) Bytes() []byte {
	return (*bytes.Buffer)(b).Bytes()
}

func (b *wireBuffer) Grow(n int) {
	(*bytes.Buffer)(b).Grow(n)
}

func (b *wireBuffer) Len() int {
	return (*bytes.Buffer)(b).Len()
}

func (b *wireBuffer) Next(n int) []byte {
	return (*bytes.Buffer)(b).Next(n)
}

func (b *wireBuffer) Read(p []byte) (int, error) {
	return (*bytes.Buffer)(b).Read(p)
}

func (b *wireBuffer) ReadFrom(r io.Reader) (int64, error) {
	return (*bytes.Buffer)(b).ReadFrom(r)
}

// ReadMessageN reads, validates, and parses the next Decred Message from r for
// the provided protocol version and Decred network.  It returns the number of
// bytes read in addition to the parsed Message and raw bytes which comprise the
// message.  This function is the same as ReadMessage except it also returns the
// number of bytes read.
func ReadMessageN(r io.Reader, pver uint32, dcrnet CurrencyNet) (int, Message, []byte, error) {
	const op = "ReadMessage"
	totalBytes := 0

	lr := &io.LimitedReader{R: r}

	// Read the bytes of the message header to the unread portion of a
	// buffer (with some additional extra capacity to read short payloads
	// without a realloc).
	buf := (*wireBuffer)(bytes.NewBuffer(make([]byte, 0, bytes.MinRead*2)))
	lr.N = MessageHeaderSize
	read, err := buf.ReadFrom(lr)
	totalBytes += int(read)
	if lr.N > 0 {
		err = io.EOF
	}
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Read the message header from the buffer.
	// This should consume all of the current buffer's length.
	var (
		magic      CurrencyNet
		command    [CommandSize]byte
		payloadLen uint32
		checksum   [4]byte
	)
	readUint32LE(buf, (*uint32)(&magic))
	buf.Read(command[:])
	readUint32LE(buf, &payloadLen)
	// Only check the final header field read for error.
	_, err = buf.Read(checksum[:])
	// The correct header length has already been read from the input
	// reader to the buffer.  This length is a constant and would not
	// change based on the message or inputs.  Any read errors or
	// remaining unread bytes in the buffer would be discovered by tests.
	if err != nil {
		str := fmt.Sprintf("unexpected read error deserializing message "+
			"header: %v", err)
		panic(str)
	}
	if buf.Len() != 0 {
		str := fmt.Sprintf("read unexpected message header length - "+
			"%d unread message header bytes remaining", buf.Len())
		panic(str)
	}

	// Enforce maximum message payload.
	if payloadLen > MaxMessagePayload {
		msg := fmt.Sprintf("message payload is too large - header "+
			"indicates %d bytes, but max message payload is %d bytes.",
			payloadLen, MaxMessagePayload)
		return totalBytes, nil, nil, messageError(op, ErrPayloadTooLarge, msg)
	}

	// Check for messages from the wrong Decred network.
	if magic != dcrnet {
		msg := fmt.Sprintf("message from other network [%v]", magic)
		return totalBytes, nil, nil, messageError(op, ErrWrongNetwork, msg)
	}

	// Check for malformed commands.
	trimmedCommand := bytes.TrimRight(command[:], string(rune(0)))
	if !isStrictAscii(string(trimmedCommand)) {
		msg := fmt.Sprintf("invalid command %q", string(trimmedCommand))
		return totalBytes, nil, nil, messageError(op, ErrMalformedCmd, msg)
	}

	// Create struct of appropriate message type based on the command.
	msg, err := makeEmptyMessage(trimmedCommand)
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Check for maximum length based on the message type as a malicious client
	// could otherwise create a well-formed header and set the length to max
	// numbers in order to exhaust the machine's memory.
	mpl := msg.MaxPayloadLength(pver)
	if payloadLen > mpl {
		msg := fmt.Sprintf("payload exceeds max length - header "+
			"indicates %v bytes, but max payload size for messages of "+
			"type [%v] is %v.", payloadLen, msg.Command(), mpl)
		return totalBytes, nil, nil, messageError(op, ErrPayloadTooLarge, msg)
	}

	// Read payload into unread portion of the buffer.
	grow := int(payloadLen)
	if grow < bytes.MinRead {
		grow = bytes.MinRead
	}
	buf.Grow(grow)
	lr.N = int64(payloadLen)
	read, err = buf.ReadFrom(lr)
	totalBytes += int(read)
	if lr.N > 0 {
		err = io.EOF
	}
	if err != nil {
		return totalBytes, nil, nil, err
	}
	// The Buffer.Bytes documentation states that this slice is not valid
	// after the next read, however, the buffer is only invalid to access
	// after the next write, reset, or truncate.  See
	// https://github.com/golang/go/commit/5270b57e51b71f2b3410b601a9ba9f0a7a3d8441.
	payload := buf.Bytes()

	// Test checksum.
	payloadHash := chainhash.HashH(payload)
	if !bytes.Equal(payloadHash[:4], checksum[:]) {
		// Create heap copies to avoid leaking the originals in the
		// fmt.Sprintf varargs.
		payloadHash := payloadHash
		checksum := checksum
		msg := fmt.Sprintf("payload checksum failed - header indicates %x, "+
			"but actual checksum is %x.", checksum[:], payloadHash[:4])
		return totalBytes, nil, nil, messageError(op, ErrPayloadChecksum, msg)
	}

	// Unmarshal message using the unread payload in the buffer.
	err = msg.BtcDecode(buf, pver)
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, msg, payload, nil
}

// ReadMessage reads, validates, and parses the next Decred Message from r for
// the provided protocol version and Decred network.  It returns the parsed
// Message and raw bytes which comprise the message.  This function only differs
// from ReadMessageN in that it doesn't return the number of bytes read.  This
// function is mainly provided for backwards compatibility with the original
// API, but it's also useful for callers that don't care about byte counts.
func ReadMessage(r io.Reader, pver uint32, dcrnet CurrencyNet) (Message, []byte, error) {
	_, msg, buf, err := ReadMessageN(r, pver, dcrnet)
	return msg, buf, err
}

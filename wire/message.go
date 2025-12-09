// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
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
func makeEmptyMessage(command string) (Message, error) {
	const op = "makeEmptyMessage"

	var msg Message
	switch command {
	case CmdVersion:
		msg = &MsgVersion{}

	case CmdVerAck:
		msg = &MsgVerAck{}

	case CmdGetAddr:
		msg = &MsgGetAddr{}

	case CmdAddr:
		msg = &MsgAddr{}

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
		str := fmt.Sprintf("unhandled command [%s]", command)
		return nil, messageError(op, ErrUnknownCmd, str)
	}
	return msg, nil
}

// messageHeader defines the header structure for all Decred protocol messages.
type messageHeader struct {
	magic    CurrencyNet // 4 bytes
	command  string      // 12 bytes
	length   uint32      // 4 bytes
	checksum [4]byte     // 4 bytes
}

// readMessageHeader reads a Decred message header from r.
func readMessageHeader(r io.Reader) (int, *messageHeader, error) {
	// Since readElements doesn't return the amount of bytes read, attempt
	// to read the entire header into a buffer first in case there is a
	// short read so the proper amount of read bytes are known.  This works
	// since the header is a fixed size.
	var headerBytes [MessageHeaderSize]byte
	n, err := io.ReadFull(r, headerBytes[:])
	if err != nil {
		return n, nil, err
	}
	hr := bytes.NewReader(headerBytes[:])

	// Create and populate a messageHeader struct from the raw header bytes.
	hdr := messageHeader{}
	var command [CommandSize]byte
	readElements(hr, &hdr.magic, &command, &hdr.length, &hdr.checksum)

	// Strip trailing zeros from command string.
	hdr.command = string(bytes.TrimRight(command[:], string(rune(0))))

	return n, &hdr, nil
}

// WriteMessageN writes a Decred Message to w including the necessary header
// information and returns the number of bytes written.    This function is the
// same as WriteMessage except it also returns the number of bytes written.
func WriteMessageN(w io.Writer, msg Message, pver uint32, dcrnet CurrencyNet) (int, error) {
	const op = "WriteMessage"
	totalBytes := 0

	var elems struct {
		dcrnet   CurrencyNet
		command  [CommandSize]byte
		lenp     uint32
		checksum [4]byte
	}
	elems.dcrnet = dcrnet

	// Enforce max command size.
	cmd := msg.Command()
	if len(cmd) > CommandSize {
		msg := fmt.Sprintf("command [%s] is too long [max %v]", cmd, CommandSize)
		return totalBytes, messageError(op, ErrCmdTooLong, msg)
	}
	copy(elems.command[:], []byte(cmd))

	// Encode the message payload.
	var bw bytes.Buffer
	err := msg.BtcEncode(&bw, pver)
	if err != nil {
		return totalBytes, err
	}
	payload := bw.Bytes()

	// Enforce maximum overall message payload.
	if len(payload) > MaxMessagePayload {
		msg := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			len(payload), MaxMessagePayload)
		return totalBytes, messageError(op, ErrPayloadTooLarge, msg)
	}
	elems.lenp = uint32(len(payload))

	// Enforce maximum message payload based on the message type.
	mpl := msg.MaxPayloadLength(pver)
	if elems.lenp > mpl {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload size for "+
			"messages of type [%s] is %d.", elems.lenp, cmd, mpl)
		return totalBytes, messageError(op, ErrPayloadTooLarge, str)
	}

	// Encode the header for the message.  This is done to a buffer
	// rather than directly to the writer since writeElements doesn't
	// return the number of bytes written.
	cksumHash := chainhash.HashH(payload)
	copy(elems.checksum[:], cksumHash[0:4])
	var buf [MessageHeaderSize]byte
	hw := bytes.NewBuffer(buf[:0])
	writeElements(hw, &elems.dcrnet, &elems.command, &elems.lenp, &elems.checksum)

	// Write header.
	n, err := w.Write(hw.Bytes())
	totalBytes += n
	if err != nil {
		return totalBytes, err
	}

	// Write payload.
	n, err = w.Write(payload)
	totalBytes += n
	return totalBytes, err
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

// ReadMessageN reads, validates, and parses the next Decred Message from r for
// the provided protocol version and Decred network.  It returns the number of
// bytes read in addition to the parsed Message and raw bytes which comprise the
// message.  This function is the same as ReadMessage except it also returns the
// number of bytes read.
func ReadMessageN(r io.Reader, pver uint32, dcrnet CurrencyNet) (int, Message, []byte, error) {
	const op = "ReadMessage"
	totalBytes := 0
	n, hdr, err := readMessageHeader(r)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Enforce maximum message payload.
	if hdr.length > MaxMessagePayload {
		msg := fmt.Sprintf("message payload is too large - header "+
			"indicates %d bytes, but max message payload is %d bytes.",
			hdr.length, MaxMessagePayload)
		return totalBytes, nil, nil, messageError(op, ErrPayloadTooLarge, msg)
	}

	// Check for messages from the wrong Decred network.
	if hdr.magic != dcrnet {
		msg := fmt.Sprintf("message from other network [%v]", hdr.magic)
		return totalBytes, nil, nil, messageError(op, ErrWrongNetwork, msg)
	}

	// Check for malformed commands.
	command := hdr.command
	if !isStrictAscii(command) {
		msg := fmt.Sprintf("invalid command %v", []byte(command))
		return totalBytes, nil, nil, messageError(op, ErrMalformedCmd, msg)
	}

	// Create struct of appropriate message type based on the command.
	msg, err := makeEmptyMessage(command)
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Check for maximum length based on the message type as a malicious client
	// could otherwise create a well-formed header and set the length to max
	// numbers in order to exhaust the machine's memory.
	mpl := msg.MaxPayloadLength(pver)
	if hdr.length > mpl {
		msg := fmt.Sprintf("payload exceeds max length - header "+
			"indicates %v bytes, but max payload size for messages of "+
			"type [%v] is %v.", hdr.length, command, mpl)
		return totalBytes, nil, nil, messageError(op, ErrPayloadTooLarge, msg)
	}

	// Read payload.
	payload := make([]byte, hdr.length)
	n, err = io.ReadFull(r, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Test checksum.
	checksum := chainhash.HashB(payload)[0:4]
	if !bytes.Equal(checksum, hdr.checksum[:]) {
		msg := fmt.Sprintf("payload checksum failed - header indicates %v, "+
			"but actual checksum is %v.", hdr.checksum, checksum)
		return totalBytes, nil, nil, messageError(op, ErrPayloadChecksum, msg)
	}

	// Unmarshal message.  NOTE: This must be a *bytes.Buffer since the
	// MsgVersion BtcDecode function requires it.
	pr := bytes.NewBuffer(payload)
	err = msg.BtcDecode(pr, pver)
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

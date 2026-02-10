// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"lukechampine.com/blake3"
)

const (
	// MaxVarIntPayload is the maximum payload size for a variable length integer.
	MaxVarIntPayload = 9

	// binaryFreeListMaxItems is the number of buffers to keep in the free
	// list to use for binary serialization and deserialization.
	binaryFreeListMaxItems = 1024

	// strictAsciiRangeLower is the lower limit of the strict ASCII range.
	strictAsciiRangeLower = 0x20

	// strictAsciiRangeUpper is the upper limit of the strict ASCII range.
	strictAsciiRangeUpper = 0x7e
)

var (
	// littleEndian is a convenience variable since binary.LittleEndian is
	// quite long.
	littleEndian = binary.LittleEndian

	// bigEndian is a convenience variable since binary.BigEndian is quite
	// long.
	bigEndian = binary.BigEndian
)

// binaryFreeList defines a concurrent safe free list of byte slices (up to the
// maximum number defined by the binaryFreeListMaxItems constant) that have a
// cap of 8 (thus it supports up to a uint64).  It is used to provide temporary
// buffers for serializing and deserializing primitive numbers to and from their
// binary encoding in order to greatly reduce the number of allocations
// required.
//
// For convenience, functions are provided for each of the primitive unsigned
// integers that automatically obtain a buffer from the free list, perform the
// necessary binary conversion, read from or write to the given io.Reader or
// io.Writer, and return the buffer to the free list.
type binaryFreeList chan []byte

// Borrow returns a byte slice from the free list with a length of 8.  A new
// buffer is allocated if there are not any available on the free list.
func (l binaryFreeList) Borrow() []byte {
	var buf []byte
	select {
	case buf = <-l:
	default:
		buf = make([]byte, 8)
	}
	return buf[:8]
}

// Return puts the provided byte slice back on the free list.  The buffer MUST
// have been obtained via the Borrow function and therefore have a cap of 8.
func (l binaryFreeList) Return(buf []byte) {
	select {
	case l <- buf:
	default:
		// Let it go to the garbage collector.
	}
}

// Uint8 reads a single byte from the provided reader using a buffer from the
// free list and returns it as a uint8.
func (l binaryFreeList) Uint8(r io.Reader) (uint8, error) {
	buf := l.Borrow()[:1]
	if _, err := io.ReadFull(r, buf); err != nil {
		l.Return(buf)
		return 0, err
	}
	rv := buf[0]
	l.Return(buf)
	return rv, nil
}

// Uint16 reads two bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint16.
func (l binaryFreeList) Uint16(r io.Reader, byteOrder binary.ByteOrder) (uint16, error) {
	buf := l.Borrow()[:2]
	if _, err := io.ReadFull(r, buf); err != nil {
		l.Return(buf)
		return 0, err
	}
	rv := byteOrder.Uint16(buf)
	l.Return(buf)
	return rv, nil
}

// Uint32 reads four bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint32.
func (l binaryFreeList) Uint32(r io.Reader, byteOrder binary.ByteOrder) (uint32, error) {
	buf := l.Borrow()[:4]
	if _, err := io.ReadFull(r, buf); err != nil {
		l.Return(buf)
		return 0, err
	}
	rv := byteOrder.Uint32(buf)
	l.Return(buf)
	return rv, nil
}

// Uint64 reads eight bytes from the provided reader using a buffer from the
// free list, converts it to a number using the provided byte order, and returns
// the resulting uint64.
func (l binaryFreeList) Uint64(r io.Reader, byteOrder binary.ByteOrder) (uint64, error) {
	buf := l.Borrow()[:8]
	if _, err := io.ReadFull(r, buf); err != nil {
		l.Return(buf)
		return 0, err
	}
	rv := byteOrder.Uint64(buf)
	l.Return(buf)
	return rv, nil
}

// binarySerializer provides a free list of buffers to use for serializing and
// deserializing primitive integer values to and from io.Readers and io.Writers.
var binarySerializer binaryFreeList = make(chan []byte, binaryFreeListMaxItems)

// errNonCanonicalVarInt is the common format string used for non-canonically
// encoded variable length integer errors.
var nonCanonicalVarIntFormat = "non-canonical varint %x - discriminant " +
	"%x must encode a value greater than %x"

// uint32Time represents a unix timestamp encoded with a uint32.  It is used as
// a way to signal the readElement function how to decode a timestamp into a Go
// time.Time since it is otherwise ambiguous.
type uint32Time time.Time

// int64Time represents a unix timestamp encoded with an int64.  It is used as
// a way to signal the readElement function how to decode a timestamp into a Go
// time.Time since it is otherwise ambiguous.
type int64Time time.Time

// uint64Time represents a unix timestamp encoded with a uint64.  It is used as
// a way to signal the readElement function how to decode a timestamp into a Go
// time.Time since it is otherwise ambiguous.  The uint64 value is rejected if
// it is larger than the maximum int64 value since it would overflow when
// converted to an int64 for the time.Unix call.
type uint64Time time.Time

// readElement reads the next sequence of bytes from r using little endian
// depending on the concrete type of element pointed to.
func readElement(r io.Reader, element interface{}) error {
	// Attempt to read the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case *uint8:
		rv, err := binarySerializer.Uint8(r)
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *uint16:
		rv, err := binarySerializer.Uint16(r, littleEndian)
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *int32:
		rv, err := binarySerializer.Uint32(r, littleEndian)
		if err != nil {
			return err
		}
		*e = int32(rv)
		return nil

	case *uint32:
		rv, err := binarySerializer.Uint32(r, littleEndian)
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *int64:
		rv, err := binarySerializer.Uint64(r, littleEndian)
		if err != nil {
			return err
		}
		*e = int64(rv)
		return nil

	case *uint64:
		rv, err := binarySerializer.Uint64(r, littleEndian)
		if err != nil {
			return err
		}
		*e = rv
		return nil

	case *bool:
		rv, err := binarySerializer.Uint8(r)
		if err != nil {
			return err
		}
		if rv == 0x00 {
			*e = false
		} else {
			*e = true
		}
		return nil

	// Unix timestamp encoded as a uint32.
	case *uint32Time:
		rv, err := binarySerializer.Uint32(r, binary.LittleEndian)
		if err != nil {
			return err
		}
		*e = uint32Time(time.Unix(int64(rv), 0))
		return nil

	// Unix timestamp encoded as an int64.
	case *int64Time:
		rv, err := binarySerializer.Uint64(r, binary.LittleEndian)
		if err != nil {
			return err
		}
		*e = int64Time(time.Unix(int64(rv), 0))
		return nil

	case *uint64Time:
		rv, err := binarySerializer.Uint64(r, binary.LittleEndian)
		if err != nil {
			return err
		}
		// Reject timestamps that would overflow when converted to int64.
		if rv > math.MaxInt64 {
			return messageError("readElement", ErrInvalidMsg,
				"timestamp exceeds maximum allowed value")
		}
		*e = uint64Time(time.Unix(int64(rv), 0))
		return nil

	// Message header checksum.
	case *[4]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *[6]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Message header command.
	case *[CommandSize]uint8:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case *[16]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Mixed message
	case *[MixMsgSize]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *[32]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *chainhash.Hash:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Mix identity
	case *[33]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// Mix signature
	case *[64]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// sntrup4591761 ciphertext
	case *[1047]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	// sntrup4591761 public key
	case *[1218]byte:
		_, err := io.ReadFull(r, e[:])
		if err != nil {
			return err
		}
		return nil

	case *ServiceFlag:
		rv, err := binarySerializer.Uint64(r, littleEndian)
		if err != nil {
			return err
		}
		*e = ServiceFlag(rv)
		return nil

	case *InvType:
		rv, err := binarySerializer.Uint32(r, littleEndian)
		if err != nil {
			return err
		}
		*e = InvType(rv)
		return nil

	case *CurrencyNet:
		rv, err := binarySerializer.Uint32(r, littleEndian)
		if err != nil {
			return err
		}
		*e = CurrencyNet(rv)
		return nil

	case *RejectCode:
		rv, err := binarySerializer.Uint8(r)
		if err != nil {
			return err
		}
		*e = RejectCode(rv)
		return nil

	case *NetAddressType:
		rv, err := binarySerializer.Uint8(r)
		if err != nil {
			return err
		}
		*e = NetAddressType(rv)
		return nil
	}

	// Fall back to the slower binary.Read if a fast path was not available
	// above.
	return binary.Read(r, littleEndian, element)
}

// readElements reads multiple items from r.  It is equivalent to multiple
// calls to readElement.
func readElements(r io.Reader, elements ...interface{}) error {
	for _, element := range elements {
		err := readElement(r, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// shortWrite optimizes short (<= 8 byte) writes to w by special casing
// buffer allocations for specific writer types.
//
// The callback returns a short buffer to 8 bytes in length and a size
// specifying how much of the buffer to write.
//
// For longer writes and writes of byte arrays, dynamic dispatch to w.Write
// should be used instead.
func shortWrite(w io.Writer, cb func() (data [8]byte, size int)) error {
	data, size := cb()

	switch w := w.(type) {
	// The most common case (called through WriteMessageN) is that the writer is a
	// *bytes.Buffer.  Optimize for that case by appending binary serializations
	// to its existing capacity instead of paying the synchronization cost to
	// serialize to temporary buffers pulled from the binary freelist.
	case *bytes.Buffer:
		w.Write(data[:size])
		return nil

	// Hashing transactions can be optimized by writing directly to the
	// BLAKE-256 hasher.
	case *blake256.Hasher256:
		w.Write(data[:size])
		return nil

	// Hashing block headers can be optimized by writing directly to the
	// BLAKE-3 hasher.
	case *blake3.Hasher:
		w.Write(data[:size])
		return nil

	default:
		p := binarySerializer.Borrow()[:size]
		copy(p, data[:size])
		_, err := w.Write(p)
		return err
	}
}

// writeUint8 writes the byte value to the writer.
func writeUint8(w io.Writer, value uint8) error {
	return shortWrite(w, func() (buf [8]byte, size int) {
		buf[0] = value
		return buf, 1
	})
}

// writeUint16LE writes the little endian encoding of value to the writer.
func writeUint16LE(w io.Writer, value uint16) error {
	return shortWrite(w, func() (buf [8]byte, size int) {
		littleEndian.PutUint16(buf[:], value)
		return buf, 2
	})
}

// writeUint16BE writes the big endian encoding of value to the writer.
func writeUint16BE(w io.Writer, value uint16) error {
	return shortWrite(w, func() (buf [8]byte, size int) {
		bigEndian.PutUint16(buf[:], value)
		return buf, 2
	})
}

// writeUint32LE writes the little endian encoding of value to the writer.
func writeUint32LE(w io.Writer, value uint32) error {
	return shortWrite(w, func() (buf [8]byte, size int) {
		littleEndian.PutUint32(buf[:], value)
		return buf, 4
	})
}

// writeUint64LE writes the little endian encoding of value to the writer.
func writeUint64LE(w io.Writer, value uint64) error {
	return shortWrite(w, func() (buf [8]byte, size int) {
		littleEndian.PutUint64(buf[:], value)
		return buf, 8
	})
}

// writeElement writes the little endian representation of element to w.
func writeElement(w io.Writer, element interface{}) error {
	// Attempt to write the element based on the concrete type via fast
	// type assertions first.
	switch e := element.(type) {
	case *uint8:
		err := writeUint8(w, *e)
		if err != nil {
			return err
		}
		return nil

	case *NetAddressType:
		err := writeUint8(w, uint8(*e))
		if err != nil {
			return err
		}
		return nil

	case *uint16:
		err := writeUint16LE(w, *e)
		if err != nil {
			return err
		}
		return nil

	// IPv4 address.
	case *[4]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *int32:
		err := writeUint32LE(w, uint32(*e))
		if err != nil {
			return err
		}
		return nil

	case *uint32:
		err := writeUint32LE(w, *e)
		if err != nil {
			return err
		}
		return nil

	case *int64:
		err := writeUint64LE(w, uint64(*e))
		if err != nil {
			return err
		}
		return nil

	case *uint64:
		err := writeUint64LE(w, *e)
		if err != nil {
			return err
		}
		return nil

	case *uint64Time:
		err := writeUint64LE(w, uint64(time.Time(*e).Unix()))
		if err != nil {
			return err
		}
		return nil

	case *bool:
		var err error
		if *e {
			err = writeUint8(w, 0x01)
		} else {
			err = writeUint8(w, 0x00)
		}
		if err != nil {
			return err
		}
		return nil

	// Block header final state.
	case *[6]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// IP address.
	case *[16]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// Mixed message
	case *[MixMsgSize]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *[32]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *chainhash.Hash:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// Mix identity
	case *[33]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// Mix signature
	case *[64]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// sntrup4591761 ciphertext
	case *[1047]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	// sntrup4591761 public key
	case *[1218]byte:
		_, err := w.Write(e[:])
		if err != nil {
			return err
		}
		return nil

	case *ServiceFlag:
		err := writeUint64LE(w, uint64(*e))
		if err != nil {
			return err
		}
		return nil

	case *InvType:
		err := writeUint32LE(w, uint32(*e))
		if err != nil {
			return err
		}
		return nil

	case *CurrencyNet:
		err := writeUint32LE(w, uint32(*e))
		if err != nil {
			return err
		}
		return nil

	case *RejectCode:
		err := writeUint8(w, uint8(*e))
		if err != nil {
			return err
		}
		return nil
	}

	// Fall back to the slower binary.Write if a fast path was not available
	// above.
	return binary.Write(w, littleEndian, element)
}

// writeElements writes multiple items to w.  It is equivalent to multiple
// calls to writeElement.
func writeElements(w io.Writer, elements ...interface{}) error {
	for _, element := range elements {
		err := writeElement(w, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadVarInt reads a variable length integer from r and returns it as a uint64.
func ReadVarInt(r io.Reader, pver uint32) (uint64, error) {
	const op = "ReadVarInt"
	discriminant, err := binarySerializer.Uint8(r)
	if err != nil {
		return 0, err
	}

	var rv uint64
	switch discriminant {
	case 0xff:
		sv, err := binarySerializer.Uint64(r, littleEndian)
		if err != nil {
			return 0, err
		}
		rv = sv

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0x100000000)
		if rv < min {
			msg := fmt.Sprintf(nonCanonicalVarIntFormat, rv, discriminant, min)
			return 0, messageError(op, ErrNonCanonicalVarInt, msg)
		}

	case 0xfe:
		sv, err := binarySerializer.Uint32(r, littleEndian)
		if err != nil {
			return 0, err
		}
		rv = uint64(sv)

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0x10000)
		if rv < min {
			msg := fmt.Sprintf(nonCanonicalVarIntFormat, rv, discriminant, min)
			return 0, messageError(op, ErrNonCanonicalVarInt, msg)
		}

	case 0xfd:
		sv, err := binarySerializer.Uint16(r, littleEndian)
		if err != nil {
			return 0, err
		}
		rv = uint64(sv)

		// The encoding is not canonical if the value could have been
		// encoded using fewer bytes.
		min := uint64(0xfd)
		if rv < min {
			msg := fmt.Sprintf(nonCanonicalVarIntFormat, rv, discriminant, min)
			return 0, messageError(op, ErrNonCanonicalVarInt, msg)
		}

	default:
		rv = uint64(discriminant)
	}

	return rv, nil
}

// WriteVarInt serializes val to w using a variable number of bytes depending
// on its value.
func WriteVarInt(w io.Writer, pver uint32, val uint64) error {
	if val < 0xfd {
		return writeUint8(w, uint8(val))
	}

	if val <= math.MaxUint16 {
		return shortWrite(w, func() (p [8]byte, size int) {
			p[0] = 0xfd
			littleEndian.PutUint16(p[1:], uint16(val))
			return p, 3
		})
	}

	if val <= math.MaxUint32 {
		return shortWrite(w, func() (p [8]byte, size int) {
			p[0] = 0xfe
			littleEndian.PutUint32(p[1:], uint32(val))
			return p, 5
		})
	}

	// shortWrite is not designed for writes > 8 bytes.
	err := writeUint8(w, 0xff)
	if err != nil {
		return err
	}
	return writeUint64LE(w, val)
}

// VarIntSerializeSize returns the number of bytes it would take to serialize
// val as a variable length integer.
func VarIntSerializeSize(val uint64) int {
	// The value is small enough to be represented by itself, so it's
	// just 1 byte.
	if val < 0xfd {
		return 1
	}

	// Discriminant 1 byte plus 2 bytes for the uint16.
	if val <= math.MaxUint16 {
		return 3
	}

	// Discriminant 1 byte plus 4 bytes for the uint32.
	if val <= math.MaxUint32 {
		return 5
	}

	// Discriminant 1 byte plus 8 bytes for the uint64.
	return 9
}

// ReadVarString reads a variable length string from r and returns it as a Go
// string.  A variable length string is encoded as a variable length integer
// containing the length of the string followed by the bytes that represent the
// string itself.  An error is returned if the length is greater than the
// maximum block payload size since it helps protect against memory exhaustion
// attacks and forced panics through malformed messages.
func ReadVarString(r io.Reader, pver uint32) (string, error) {
	const op = "ReadVarString"
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return "", err
	}

	// Prevent variable length strings that are larger than the maximum
	// message size.  It would be possible to cause memory exhaustion and
	// panics without a sane upper bound on this count.
	if count > MaxMessagePayload {
		msg := fmt.Sprintf("variable length string is too long "+
			"[count %d, max %d]", count, MaxMessagePayload)
		return "", messageError(op, ErrVarStringTooLong, msg)
	}

	buf := make([]byte, count)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// ReadAsciiVarString reads a variable length string from r and returns it as a
// Go string.  A variable length string is encoded as a variable length integer
// containing the length of the string followed by the bytes that represent the
// string itself.  An error is returned if the length is greater than the
// specified maxAllowed argument, greater than the global maximum message
// payload length or if the decoded string is not strictly an ascii string.
func ReadAsciiVarString(r io.Reader, pver uint32, maxAllowed uint64) (string, error) {
	const op = "ReadAsciiVarString"
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return "", err
	}

	// Prevent variable length strings that are larger than the specified
	// size or the global maximum payload (whichever is lower).  It would
	// be possible to cause memory exhaustion and panics without a sane
	// upper bound on this count.
	max := maxAllowed
	if maxAllowed > MaxMessagePayload {
		max = MaxMessagePayload
	}
	if count > max {
		msg := fmt.Sprintf("variable length string is too long "+
			"[count %d, max %d]", count, max)
		return "", messageError(op, ErrVarStringTooLong, msg)
	}

	buf := make([]byte, count)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return "", err
	}

	s := string(buf)
	if !isStrictAscii(s) {
		msg := "string is not strict ASCII"
		return "", messageError(op, ErrMalformedStrictString, msg)
	}

	return s, nil
}

// WriteVarString serializes str to w as a variable length integer containing
// the length of the string followed by the bytes that represent the string
// itself.
func WriteVarString(w io.Writer, pver uint32, str string) error {
	err := WriteVarInt(w, pver, uint64(len(str)))
	if err != nil {
		return err
	}

	switch w := w.(type) {
	case *bytes.Buffer:
		_, err = w.WriteString(str)
	case *blake256.Hasher256:
		w.WriteString(str)
	default:
		_, err = w.Write([]byte(str))
	}
	return err
}

// ReadVarBytes reads a variable length byte array.  A byte array is encoded
// as a varInt containing the length of the array followed by the bytes
// themselves.  An error is returned if the length is greater than the
// passed maxAllowed parameter which helps protect against memory exhaustion
// attacks and forced panics through malformed messages.  The fieldName
// parameter is only used for the error message so it provides more context in
// the error.
func ReadVarBytes(r io.Reader, pver uint32, maxAllowed uint32,
	fieldName string) ([]byte, error) {
	const op = "ReadVarBytes"
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return nil, err
	}

	// Prevent byte array larger than the max message size.  It would
	// be possible to cause memory exhaustion and panics without a sane
	// upper bound on this count.
	if count > uint64(maxAllowed) {
		msg := fmt.Sprintf("%s is larger than the max allowed size "+
			"[count %d, max %d]", fieldName, count, maxAllowed)
		return nil, messageError(op, ErrVarBytesTooLong, msg)
	}

	b := make([]byte, count)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// WriteVarBytes serializes a variable length byte array to w as a varInt
// containing the number of bytes, followed by the bytes themselves.
func WriteVarBytes(w io.Writer, pver uint32, bytes []byte) error {
	slen := uint64(len(bytes))
	err := WriteVarInt(w, pver, slen)
	if err != nil {
		return err
	}

	_, err = w.Write(bytes)
	return err
}

// randomUint64 returns a cryptographically random uint64 value.  This
// unexported version takes a reader primarily to ensure the error paths
// can be properly tested by passing a fake reader in the tests.
func randomUint64(r io.Reader) (uint64, error) {
	rv, err := binarySerializer.Uint64(r, bigEndian)
	if err != nil {
		return 0, err
	}
	return rv, nil
}

// RandomUint64 returns a cryptographically random uint64 value.
func RandomUint64() (uint64, error) {
	return randomUint64(rand.Reader)
}

// isStrictAscii determines returns true if the provided string only contains
// runes that are within the strict ASCII range.
func isStrictAscii(s string) bool {
	for _, r := range s {
		if r < strictAsciiRangeLower || r > strictAsciiRangeUpper {
			return false
		}
	}

	return true
}

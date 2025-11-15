// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// newNetAddressV2 is a convenience function for constructing a new v2 network
// address.
func newNetAddressV2(addrType NetAddressType, addrBytes []byte, port uint16) NetAddressV2 {
	timestamp := time.Unix(0x495fab29, 0) // 2009-01-03 12:15:05 -0600 CST
	netAddr := NewNetAddressV2(addrType, addrBytes, port, timestamp,
		SFNodeNetwork)
	return netAddr
}

var (
	ipv4IpBytes = []byte{0x7f, 0x00, 0x00, 0x01}
	ipv6IpBytes = []byte{
		0x26, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}

	ipv4NetAddress = newNetAddressV2(IPv4Address, ipv4IpBytes, 8333)
	ipv6NetAddress = newNetAddressV2(IPv6Address, ipv6IpBytes, 8333)

	serializedIPv4NetAddressBytes = []byte{
		0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Services
		0x01,                   // Type (IPv4)
		0x7f, 0x00, 0x00, 0x01, // IP
		0x8d, 0x20, // Port 8333 (little-endian)
	}
	serializedIPv6NetAddressBytes = []byte{
		0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Services
		0x02,                                           // Type (IPv6)
		0x26, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // IP (upper)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // IP (lower)
		0x8d, 0x20, // Port 8333 (little-endian)
	}
	serializedUnknownNetAddressBytes = []byte{
		0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Services
		0x00,                   // Type (Unknown)
		0x7f, 0x00, 0x00, 0x01, // IP
		0x8d, 0x20, // Port 8333 (little-endian)
	}
)

// TestAddrV2MaxPayloadLength verifies the maximum payload length equals the
// expected value at various protocol versions and does not exceed the maximum
// message size for any protocol message.
func TestAddrV2MaxPayloadLength(t *testing.T) {
	tests := []struct {
		name string
		pver uint32
		want uint32
	}{{
		name: "protocol version 11",
		pver: AddrV2Version - 1,
		want: 0,
	}, {
		name: "protocol version 12",
		pver: AddrV2Version,
		want: 35003,
	}, {
		name: "latest protocol version",
		pver: ProtocolVersion,
		want: 35003,
	}}

	for _, test := range tests {
		// Ensure max payload is expected value for latest protocol version.
		msg := NewMsgAddrV2()
		result := msg.MaxPayloadLength(test.pver)
		if result != test.want {
			t.Errorf("%s: wrong max payload length - got %v, want %d",
				test.name, result, test.want)
			continue
		}

		// Ensure max payload length is not more than the maximum allowed for
		// any protocol message.
		if result > MaxMessagePayload {
			t.Errorf("%s: payload length exceeds maximum message payload - "+
				"got %d, want less than %d.", test.name, result,
				MaxMessagePayload)
			continue
		}
	}
}

// TestAddrV2 tests the MsgAddrV2 API.
func TestAddrV2(t *testing.T) {
	// Ensure the command is expected value.
	wantCmd := "addrv2"
	msg := NewMsgAddrV2()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgAddrV2: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure NetAddresses are added properly.
	err := msg.AddAddress(ipv4NetAddress)
	if err != nil {
		t.Errorf("AddAddress: %v", err)
	}
	if !reflect.DeepEqual(msg.AddrList[0], ipv4NetAddress) {
		t.Errorf("AddAddress: wrong address added - got %v, want %v",
			spew.Sprint(msg.AddrList[0]), spew.Sprint(ipv4NetAddress))
	}

	// Ensure the address list is cleared properly.
	msg.ClearAddresses()
	if len(msg.AddrList) != 0 {
		t.Errorf("ClearAddresses: address list is not empty - "+
			"got %v [%v], want %v", len(msg.AddrList),
			spew.Sprint(msg.AddrList[0]), 0)
	}

	// Ensure adding more than the max allowed addresses per message returns
	// error.
	for i := 0; i < MaxAddrPerV2Msg+1; i++ {
		err = msg.AddAddress(ipv4NetAddress)
	}
	if !errors.Is(err, ErrTooManyAddrs) {
		t.Errorf("AddAddress: expected ErrTooManyAddrs, got %v", err)
	}

	// Make sure adding multiple addresses also returns an error when the
	// message is at max capacity.
	err = msg.AddAddresses(ipv4NetAddress)
	if !errors.Is(err, ErrTooManyAddrs) {
		t.Errorf("AddAddresses: expected ErrTooManyAddrs, got %v", err)
	}
}

// TestAddrV2Wire tests the MsgAddrV2 wire encode and decode for various
// numbers of addresses at the latest protocol version.
func TestAddrV2Wire(t *testing.T) {
	pver := ProtocolVersion
	tests := []struct {
		name      string
		addrs     []NetAddressV2
		wantBytes []byte
	}{{
		name: "latest protocol version with one address",
		addrs: []NetAddressV2{
			ipv4NetAddress,
		},
		wantBytes: bytes.Join([][]byte{
			{0x01},
			serializedIPv4NetAddressBytes,
		}, []byte{}),
	}, {
		name: "latest protocol version with multiple addresses",
		addrs: []NetAddressV2{
			ipv4NetAddress,
			ipv6NetAddress,
		},
		wantBytes: bytes.Join([][]byte{
			{0x02},
			serializedIPv4NetAddressBytes,
			serializedIPv6NetAddressBytes,
		}, []byte{}),
	}, {
		name: "latest protocol version with maximum addresses",
		addrs: func() []NetAddressV2 {
			var addrs []NetAddressV2
			for i := 0; i < MaxAddrPerV2Msg; i++ {
				addrs = append(addrs, ipv6NetAddress)
			}
			return addrs
		}(),
		wantBytes: func() []byte {
			parts := [][]byte{{0xfd, 0xe8, 0x03}} // Varint address count: 1000
			for i := 0; i < MaxAddrPerV2Msg; i++ {
				parts = append(parts, serializedIPv6NetAddressBytes)
			}
			return bytes.Join(parts, []byte{})
		}(),
	}}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		subject := NewMsgAddrV2()
		subject.AddAddresses(test.addrs...)

		// Encode the message to the wire format and ensure it serializes
		// correctly.
		var buf bytes.Buffer
		err := subject.BtcEncode(&buf, pver)
		if err != nil {
			t.Errorf("%q: error encoding message - %v", test.name, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.wantBytes) {
			t.Errorf("%q: mismatched bytes -- got: %s want: %s", test.name,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.wantBytes))
			continue
		}

		// Decode the message from the wire format and ensure it deserializes
		// correctly.
		var msg MsgAddrV2
		rbuf := bytes.NewReader(test.wantBytes)
		err = msg.BtcDecode(rbuf, pver)
		if err != nil {
			t.Errorf("%q: error decoding message - %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(&msg, subject) {
			t.Errorf("%q: mismatched message - got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(subject))
			continue
		}
	}
}

// TestAddrV2BtcDecode verifies decode behavior for various error conditions.
func TestAddrV2BtcDecode(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		name      string
		pver      uint32
		wireBytes []byte
		wantAddrs []NetAddressV2
		wantErr   error
	}{{
		name: "addrv2 message invalid for pver 11",
		pver: AddrV2Version - 1,
		wireBytes: bytes.Join([][]byte{
			{0x01},
			serializedIPv4NetAddressBytes,
		}, []byte{}),
		wantAddrs: nil,
		wantErr:   ErrMsgInvalidForPVer,
	}, {
		name: "message with no addresses",
		pver: pver,
		wireBytes: []byte{
			0x00, // Varint address count
		},
		wantAddrs: nil,
		wantErr:   ErrTooFewAddrs,
	}, {
		name: "message missing expected addresses",
		pver: pver,
		wireBytes: []byte{
			0x01, // Varint address count
		},
		wantAddrs: nil,
		wantErr:   io.EOF,
	}, {
		name: "message with too many addresses",
		pver: pver,
		wireBytes: []byte{
			0xfd, 0xe9, 0x03, // Varint address count: MaxAddrPerV2Msg+1
		},
		wantAddrs: nil,
		wantErr:   ErrTooManyAddrs,
	}, {
		name: "address with overflowed timestamp",
		pver: pver,
		wireBytes: []byte{
			0x01,                                           // Varint address count
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, // Timestamp (MaxInt64+1)
			0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Services
			0x01,                   // Type (IPv4)
			0x7f, 0x00, 0x00, 0x01, // IP
			0x8d, 0x20, // Port 8333 (little-endian)
		},
		wantAddrs: nil,
		wantErr:   ErrInvalidMsg,
	}, {
		name: "message with valid types and unknown type",
		pver: pver,
		wireBytes: bytes.Join([][]byte{
			{0x03},
			serializedIPv4NetAddressBytes,
			serializedIPv6NetAddressBytes,
			serializedUnknownNetAddressBytes,
		}, []byte{}),
		wantAddrs: nil,
		wantErr:   ErrUnknownNetAddrType,
	}, {
		name: "message with multiple valid addresses",
		pver: pver,
		wireBytes: bytes.Join([][]byte{
			{0x02},
			serializedIPv4NetAddressBytes,
			serializedIPv6NetAddressBytes,
		}, []byte{}),
		wantAddrs: []NetAddressV2{ipv4NetAddress, ipv6NetAddress},
		wantErr:   nil,
	}}

	for _, test := range tests {
		var msg MsgAddrV2
		rbuf := bytes.NewReader(test.wireBytes)
		err := msg.BtcDecode(rbuf, test.pver)

		if !errors.Is(err, test.wantErr) {
			t.Errorf("%q: wrong error - got: %v, want: %v", test.name, err, test.wantErr)
			continue
		}

		if test.wantErr == nil && !reflect.DeepEqual(msg.AddrList, test.wantAddrs) {
			t.Errorf("%q: expected %d addresses, got %d - want: %s, got: %s",
				test.name, len(test.wantAddrs), len(msg.AddrList),
				spew.Sdump(test.wantAddrs), spew.Sdump(msg.AddrList))
		}
	}
}

// TestAddrV2BtcEncode performs negative tests against wire encoding
// of MsgAddrV2 to confirm error paths work correctly.
func TestAddrV2BtcEncode(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		name    string
		addrs   []NetAddressV2
		pver    uint32
		wantErr error
	}{{
		name: "addrv2 message invalid for pver 11",
		pver: AddrV2Version - 1,
		addrs: []NetAddressV2{{
			Timestamp: time.Unix(0x495fab29, 0),
			Services:  SFNodeNetwork,
			Type:      IPv4Address,
			IP:        ipv4IpBytes,
			Port:      8333,
		}},
		wantErr: ErrMsgInvalidForPVer,
	}, {
		name:    "message with no addresses",
		pver:    pver,
		addrs:   nil,
		wantErr: ErrTooFewAddrs,
	}, {
		name:    "message with too many addresses",
		pver:    pver,
		addrs:   make([]NetAddressV2, MaxAddrPerV2Msg+1),
		wantErr: ErrTooManyAddrs,
	}, {
		name: "message with wrong size IPv4 address",
		pver: pver,
		addrs: []NetAddressV2{{
			Timestamp: time.Unix(0x495fab29, 0),
			Services:  SFNodeNetwork,
			Type:      IPv4Address,
			IP:        make([]byte, 1),
			Port:      8333,
		}},
		wantErr: ErrInvalidMsg,
	}, {
		name: "message with wrong size IPv6 address",
		pver: pver,
		addrs: []NetAddressV2{{
			Timestamp: time.Unix(0x495fab29, 0),
			Services:  SFNodeNetwork,
			Type:      IPv6Address,
			IP:        make([]byte, 1),
			Port:      8333,
		}},
		wantErr: ErrInvalidMsg,
	}, {
		name: "message with unknown address type",
		pver: pver,
		addrs: []NetAddressV2{{
			Timestamp: time.Unix(0x495fab29, 0),
			Services:  SFNodeNetwork,
			Type:      UnknownAddressType,
			IP:        make([]byte, 1),
			Port:      8333,
		}},
		wantErr: ErrUnknownNetAddrType,
	}}

	for _, test := range tests {
		msg := NewMsgAddrV2()
		msg.AddrList = test.addrs
		ioLimit := int(msg.MaxPayloadLength(test.pver))

		// Encode to wire format.
		w := newFixedWriter(ioLimit)
		err := msg.BtcEncode(w, test.pver)
		if !errors.Is(err, test.wantErr) {
			t.Errorf("%q: wrong error - got: %v, want: %v", test.name, err,
				test.wantErr)
			continue
		}
	}
}

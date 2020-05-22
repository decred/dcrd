// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"encoding/binary"
	"net"
)

const (
	torGeneralError      = 0x01
	torNotAllowed        = 0x02
	torNetUnreachable    = 0x03
	torHostUnreachable   = 0x04
	torConnectionRefused = 0x05
	torTTLExpired        = 0x06
	torCmdNotSupported   = 0x07
	torAddrNotSupported  = 0x08

	torATypeIPv4       = 1
	torATypeDomainName = 3
	torATypeIPv6       = 4

	torCmdResolve = 240
)

var (
	torStatusErrors = map[byte]error{
		torGeneralError:      MakeError(ErrTorGeneralError, "tor general error"),
		torNotAllowed:        MakeError(ErrTorNotAllowed, "tor not allowed"),
		torNetUnreachable:    MakeError(ErrTorNetUnreachable, "tor network is unreachable"),
		torHostUnreachable:   MakeError(ErrTorHostUnreachable, "tor host is unreachable"),
		torConnectionRefused: MakeError(ErrTorConnectionRefused, "tor connection refused"),
		torTTLExpired:        MakeError(ErrTorTTLExpired, "tor TTL expired"),
		torCmdNotSupported:   MakeError(ErrTorCmdNotSupported, "tor command not supported"),
		torAddrNotSupported:  MakeError(ErrTorAddrNotSupported, "tor address type not supported"),
	}
)

// TorLookupIP uses Tor to resolve DNS via the passed SOCKS proxy.
func TorLookupIP(ctx context.Context, host, proxy string) ([]net.IP, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", proxy)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	buf := []byte{0x05, 0x01, 0x00}
	_, err = conn.Write(buf)
	if err != nil {
		return nil, err
	}

	buf = make([]byte, 2)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if buf[0] != 0x05 {
		return nil, MakeError(ErrTorInvalidProxyResponse,
			"invalid SOCKS proxy version")
	}
	if buf[1] != 0x00 {
		return nil, MakeError(ErrTorUnrecognizedAuthMethod,
			"invalid proxy authentication method")
	}

	buf = make([]byte, 7+len(host))
	buf[0] = 5 // socks protocol version
	buf[1] = torCmdResolve
	buf[2] = 0 // reserved
	buf[3] = torATypeDomainName
	buf[4] = byte(len(host))
	copy(buf[5:], host)
	buf[5+len(host)] = 0 // Port 0

	_, err = conn.Write(buf)
	if err != nil {
		return nil, err
	}

	buf = make([]byte, 4)
	_, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if buf[0] != 5 {
		return nil, MakeError(ErrTorInvalidProxyResponse,
			"invalid SOCKS proxy version")
	}
	if buf[1] != 0 {
		err, exists := torStatusErrors[buf[1]]
		if !exists {
			err = MakeError(ErrTorInvalidProxyResponse,
				"invalid SOCKS proxy version")
		}
		return nil, err
	}
	if buf[3] != torATypeIPv4 && buf[3] != torATypeIPv6 {
		return nil, MakeError(ErrTorInvalidAddressResponse,
			"invalid IP address")
	}

	var reply [32 + 2]byte
	replyLen, err := conn.Read(reply[:])
	if err != nil {
		return nil, err
	}

	var addr net.IP
	switch buf[3] {
	case torATypeIPv4:
		if replyLen != 4+2 {
			return nil, MakeError(ErrTorInvalidAddressResponse,
				"invalid IPV4 address")
		}
		r := binary.BigEndian.Uint32(reply[0:4])
		addr = net.IPv4(byte(r>>24), byte(r>>16),
			byte(r>>8), byte(r))
	case torATypeIPv6:
		if replyLen <= 4+2 {
			return nil, MakeError(ErrTorInvalidAddressResponse,
				"invalid IPV6 address")
		}
		addr = net.IP(reply[0 : replyLen-2])
	default:
		return nil, MakeError(ErrTorInvalidAddressResponse,
			"unknown address type")
	}

	return []net.IP{addr}, nil
}

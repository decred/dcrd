// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
)

const (
	torSucceeded         = 0x00
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
	// ErrTorInvalidAddressResponse indicates an invalid address was
	// returned by the Tor DNS resolver.
	ErrTorInvalidAddressResponse = errors.New("invalid address response")

	// ErrTorInvalidProxyResponse indicates the Tor proxy returned a
	// response in an unexpected format.
	ErrTorInvalidProxyResponse = errors.New("invalid proxy response")

	// ErrTorUnrecognizedAuthMethod indicates the authentication method
	// provided is not recognized.
	ErrTorUnrecognizedAuthMethod = errors.New("invalid proxy authentication method")

	torStatusErrors = map[byte]error{
		torSucceeded:         errors.New("tor succeeded"),
		torGeneralError:      errors.New("tor general error"),
		torNotAllowed:        errors.New("tor not allowed"),
		torNetUnreachable:    errors.New("tor network is unreachable"),
		torHostUnreachable:   errors.New("tor host is unreachable"),
		torConnectionRefused: errors.New("tor connection refused"),
		torTTLExpired:        errors.New("tor TTL expired"),
		torCmdNotSupported:   errors.New("tor command not supported"),
		torAddrNotSupported:  errors.New("tor address type not supported"),
	}
)

// TorLookupIPContext uses Tor to resolve DNS via the passed SOCKS proxy.
func TorLookupIPContext(ctx context.Context, host, proxy string) ([]net.IP, error) {
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
		return nil, ErrTorInvalidProxyResponse
	}
	if buf[1] != 0x00 {
		return nil, ErrTorUnrecognizedAuthMethod
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
		return nil, ErrTorInvalidProxyResponse
	}
	if buf[1] != 0 {
		err, exists := torStatusErrors[buf[1]]
		if !exists {
			err = ErrTorInvalidProxyResponse
		}
		return nil, err
	}
	if buf[3] != torATypeIPv4 && buf[3] != torATypeIPv6 {
		return nil, ErrTorInvalidAddressResponse
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
			return nil, ErrTorInvalidAddressResponse
		}
		r := binary.BigEndian.Uint32(reply[0:4])
		addr = net.IPv4(byte(r>>24), byte(r>>16),
			byte(r>>8), byte(r))
	case torATypeIPv6:
		if replyLen <= 4+2 {
			return nil, ErrTorInvalidAddressResponse
		}
		addr = net.IP(reply[0 : replyLen-2])
	default:
		return nil, ErrTorInvalidAddressResponse
	}

	return []net.IP{addr}, nil
}

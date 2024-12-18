// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build linux && go1.24

package rand

import (
	"bytes"
	"strconv"

	"golang.org/x/sys/unix"
)

func init() {
	var utsname unix.Utsname
	err := unix.Uname(&utsname)
	if err != nil {
		return
	}
	if usesVDSO(utsname.Release[:]) {
		readCryptoRand = true
		globalMutex = nil
	}
}

func usesVDSO(kernelVersion []byte) bool {
	var v = kernelVersion

	dot := bytes.IndexByte(v, '.')
	if dot == -1 {
		return false
	}
	maj, err := strconv.Atoi(string(v[:dot]))
	if err != nil {
		return false
	}
	v = v[dot+1:]
	dot = bytes.IndexByte(v, '.')
	if dot == -1 {
		return false
	}
	min, err := strconv.Atoi(string(v[:dot]))
	if err != nil {
		return false
	}

	// crypto/rand is implemented by the vDSO on Linux >= 6.11.
	return maj >= 7 || (maj == 6 && min >= 11)
}

// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
//go:build windows || aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package main

import (
	"syscall"
)

func init() {
	interruptSignals = append(interruptSignals, syscall.SIGTERM, syscall.SIGHUP)
}

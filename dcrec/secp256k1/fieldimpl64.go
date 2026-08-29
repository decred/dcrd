// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build amd64 || arm64 || ppc64 || ppc64le || s390x || riscv64

package secp256k1

import "github.com/decred/dcrd/dcrec/secp256k1/v4/field4x64"

// fieldImpl defines the concrete finite field implementation.  It is set to
// the 64-bit backend on 64-bit hardware.
type fieldImpl = field4x64.Element

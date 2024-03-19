// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"strings"
)

const MixMsgSize = 20

// MixVect is vector of padded or unpadded DC-net messages.
type MixVect [][MixMsgSize]byte

func (v MixVect) String() string {
	b := new(strings.Builder)
	b.Grow(2 + len(v)*(2*MixMsgSize+1))
	b.WriteString("[")
	for i := range v {
		if i != 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(b, "%x", v[i][:])
	}
	b.WriteString("]")
	return b.String()
}

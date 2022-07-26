// Copyright (c) 2022 The Decred developers
// Originally written in 2011-2012 by Dmitry Chestnykh.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blake256

func FuzzSum256(data []byte) int {
	if len(data) < 10 {
		return 0
	}
	switch data[0] % 3 {
	case 0:
		_ = Sum224(data[1:])
	case 1:
		_ = Sum256(data[1:])
	case 2:
		if len(data) < 18 {
			return 0
		}
		_ = New224Salt(data[1:17])
	}
	return 1
}

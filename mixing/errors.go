// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"errors"
	"fmt"
)

var (
	errInvalidPROrder = errors.New("invalid pair request order")

	errInvalidSessionID = errors.New("invalid session ID")
)

// DecapsulateError identifies the unmixed peer position of a peer who
// submitted an undecryptable ciphertext.
type DecapsulateError struct {
	SubmittingIndex uint32
}

// Error satisifies the error interface.
func (e *DecapsulateError) Error() string {
	return fmt.Sprintf("decapsulate failure of ciphertext by peer %d",
		e.SubmittingIndex)
}

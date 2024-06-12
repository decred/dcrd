// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"github.com/decred/slog"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
// The default amount of logging is none.
var log = slog.Disabled

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using slog.
func UseLogger(logger slog.Logger) {
	log = logger
}

// pickNoun returns the singular or plural form of a noun depending on the count
// n.
func pickNoun[T ~uint32 | ~uint64](n T, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

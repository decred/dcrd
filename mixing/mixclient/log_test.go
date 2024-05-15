// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import (
	"testing"

	"github.com/decred/dcrd/mixing/mixpool"
	"github.com/decred/slog"
)

type testLog struct {
	*testing.T
}

func (t *testLog) Write(b []byte) (int, error) {
	t.Logf("%s", b)
	return len(b), nil
}

// useTestLogger sets the package-level loggers to a backend that writes
// trace-level logs to the test log.  A function is returned to set the logger
// back to Disabled when finished.
//
// Due to the use of global logger variables that must write to test logs to
// individual test variables, it is not possible to parallelize tests.
func useTestLogger(t *testing.T) (slog.Logger, func()) {
	backend := slog.NewBackend(&testLog{T: t})
	l := backend.Logger("TEST")
	l.SetLevel(slog.LevelTrace)
	mixpool.UseLogger(l)
	return l, func() {
		mixpool.UseLogger(slog.Disabled)
	}
}

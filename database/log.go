// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2016-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"github.com/decred/slog"
)

// UseLogger uses a specified Logger to output package logging info.
func UseLogger(logger slog.Logger) {
	// Update the logger for the registered drivers.
	for _, drv := range drivers {
		if drv.UseLogger != nil {
			drv.UseLogger(logger)
		}
	}
}

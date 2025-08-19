// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file changes the default GODEBUG values when building with newer
// releases of Go to enable as many of the new features and security updates
// that are not strictly backwards compatible as possible.
//
// For reference, in order to avoid breaking backwards compatibility, newer
// versions of Go toolchains automatically set GODEBUG flags to disable any
// changes that are not strictly backwards compatible when compiling old code.
// However, it is often the case that older code will work properly with the
// new features and security updates enabled and those updates are generally
// desirable.
//
// WARNING: Do not blindly update this with each new Go release.  It needs to
// be analyzed with each new release before updating to ensure none of the
// changes in the newer versions of Go that are disabled by default due to not
// being strictly backwards compatible will break the existing code.

//go:build go1.25

//go:debug default=go1.25

package main

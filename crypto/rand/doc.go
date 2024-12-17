// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package rand implements a fast userspace CSPRNG that is periodically
// reseeded with entropy obtained from crypto/rand.  The PRNG can be used to
// obtain random bytes as well as generating uniformly-distributed integers in
// a full or limited range.
//
// The default global PRNG will never panic after package init and is safe for
// concurrent access.  Additional PRNGs which avoid the locking overhead can
// be created by calling NewPRNG.
//
// On select operating systems and Go versions, this package may fallback to
// crypto/rand when it is already implemented by a fast userspace CSPRNG.
package rand

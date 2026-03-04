// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package connmgr implements a generic Decred network connection manager.

# Deprecated

This module is deprecated and is no longer maintained.  Callers are encouraged
to use github.com/decred/dcrd/addrmgr/vX for methods that were moved to it
instead.

# Connection Manager Overview

Connection manager handles all the general connection concerns such as
maintaining a set number of outbound connections, sourcing peers, banning,
limiting max connections, tor lookup, etc.
*/
package connmgr

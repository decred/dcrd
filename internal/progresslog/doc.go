// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package progresslog provides periodic logging for block processing.

Tests are included to ensure proper functionality.

## Feature Overview

- Maintains cumulative totals about blocks between each logging interval
  - Total number of blocks
  - Total number of transactions
  - Total number of votes
  - Total number of tickets
  - Total number of revocations
- Logs all cumulative data every 10 seconds
- Immediately logs any outstanding data when the provided sync height is reached
*/
package progresslog

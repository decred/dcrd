progresslog
===========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/progresslog)

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

## License

Package progresslog is licensed under the [copyfree](http://copyfree.org) ISC
License.

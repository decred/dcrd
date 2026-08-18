# dcrd v2.1.6 Release Notes

This is a patch release of dcrd which includes the following changes:

- Critical consensus-related security fix
- Prevents a potential periodic deanonymization mixing attack
- Several fixes for potential network-related denial-of-service (DoS) attacks
- Improved mixing session expiry

## Upgrade Mandatory

This release contains a fix for a critical consensus-related security
vulnerability.  Everyone is required to upgrade or risk being forked from the
network.  This is particularly important for individual stakeholders, Voting
Service Providers, PoW miners, and exchanges.

## Changelog

This patch release consists of 23 commits from 3 contributors which total to 20
files changed, 795 additional lines of code, and 392 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.1.5...release-v2.1.6).

### Protocol and network:

- [release-v2.1] server: Ban miningstate msgs on old protocol vers ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] server: Ban peers for multiple initial state msgs ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] netsync: Limit manual request maps ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Explicitly reject sb stake spends ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))

### Mixing message relay (mix pool):

- [release-v2.1] mixpool: Properly calculate session expiry ([decred/dcrd#3766](https://github.com/decred/dcrd/pull/3766))

### Developer-related package and module changes:

- [release-v2.1] standalone: Reject bad input values in tx sanity ([decred/dcrd#3767](https://github.com/decred/dcrd/pull/3767))
- [release-v2.1] blockchain:  Early null revocation input check ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] netsync: Mark blocks as known via headers ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Cleanup proof of stake checks ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: New add funcs with overflow detection ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] multi: Unsigned sigop counts and cleaner overflow ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Enforce trsybase fee in inputs check ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Enforce trsy spend amt in input checks ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Don't recalculate stake tree fees ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Cleanup tx input and fee overflow ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Reduce in-flight utxo add cost to O(1) ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))

### Developer-related module management:

- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3766](https://github.com/decred/dcrd/pull/3766))
- [release-v2.1] main: Use backported standalone updates ([decred/dcrd#3766](https://github.com/decred/dcrd/pull/3766))
- [release-v2.1] main: Use backported blockchain updates ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))

### Testing and Quality Assurance:

- [release-v2.1] standalone: Add tests for new sanity checks ([decred/dcrd#3767](https://github.com/decred/dcrd/pull/3767))
- [release-v2.1] blockchain: Add tests for new add funcs ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))
- [release-v2.1] blockchain: Add a couple of trsybase corner tests ([decred/dcrd#3770](https://github.com/decred/dcrd/pull/3770))

### Misc:

- [release-v2.1] release: Bump for 2.1.6 ([decred/dcrd#3772](https://github.com/decred/dcrd/pull/3772))

### Code Contributors (alphabetical order):

- Dave Collins
- Jamie Holdstock
- Josh Rickmar

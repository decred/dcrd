# dcrd v2.0.6 Release Notes

This is a patch release of dcrd which includes the following changes:

- Improved reliability for wallets participating in StakeShuffle mixing
- Minor performance enhancement for random number generation on OpenBSD

## Changelog

This patch release consists of 3 commits from 2 contributors which total to 8
files changed, 119 additional lines of code, and 68 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.0.5...release-v2.0.6).

### Developer-related package and module changes:

- [release-v2.0] rand: Use stdlib crypto/rand.Read on OpenBSD + Go 1.24 ([decred/dcrd#3473](https://github.com/decred/dcrd/pull/3473))

### Developer-related module management:

- [release-v2.0] main: Use backported rand updates ([decred/dcrd#3473](https://github.com/decred/dcrd/pull/3473))

### Misc:

- [release-v2.0] release: Bump for 2.0.6 ([decred/dcrd#3474](https://github.com/decred/dcrd/pull/3474))

### Code Contributors (alphabetical order):

- Dave Collins
- Josh Rickmar

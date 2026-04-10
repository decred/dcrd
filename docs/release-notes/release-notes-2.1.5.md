# dcrd v2.1.5 Release Notes

This is a patch release of dcrd which includes the following changes:

- Correct a potential node crash that only affects the v2.1.4 release
- Coins excluded from a mixing session that exceeds new limits are no longer greylisted

## Changelog

This patch release consists of 4 commits from 2 contributors which total to 5
files changed, 36 additional lines of code, and 35 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.1.4...release-v2.1.5).

### Protocol and network:

- [release-v2.1] server: Revert Don't consider services before handshake ([decred/dcrd#3670](https://github.com/decred/dcrd/pull/3670))

### Mixing message relay (mix pool):

- [release-v2.1] mixpool: Avoid greylist strikes on size excluded peers ([decred/dcrd#3675](https://github.com/decred/dcrd/pull/3675))

### Developer-related module management:

- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3675](https://github.com/decred/dcrd/pull/3675))

### Misc:

- [release-v2.1] release: Bump for 2.1.5 ([decred/dcrd#3671](https://github.com/decred/dcrd/pull/3671))

### Code Contributors (alphabetical order):

- Dave Collins
- Josh Rickmar

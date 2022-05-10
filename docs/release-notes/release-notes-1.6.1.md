# dcrd v1.6.1

This is a patch release of dcrd which includes the following changes:

- Correct a hard to hit issue where connections might not be reestablished after
  a network outage under some rare circumstances
- Allow stakeholders to make use of the staking system to force proof-of-work
  miners to upgrade to the latest version so voting on the new consensus changes
  can commence

## Changelog

This patch release consists of 3 commits from 1 contributor which total to 3
files changed, 30 additional lines of code, and 9 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.6.0...release-v1.6.1).

### Protocol and network:

- server: Notify block mgr later and track ntfn ([decred/dcrd#2588](https://github.com/decred/dcrd/pull/2588))
- server: Force PoW upgrade to v8 ([decred/dcrd#2597](https://github.com/decred/dcrd/pull/2597))

### Misc:

- release: Bump for 1.6.1 ([decred/dcrd#2600](https://github.com/decred/dcrd/pull/2600))

### Code Contributors (alphabetical order):

- Dave Collins

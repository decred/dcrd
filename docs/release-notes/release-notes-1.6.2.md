# dcrd v1.6.2

This is a patch release of dcrd to introduce a quality of life change for
lightweight clients, such as SPV wallets, by not sending them a certain class
of announcements that only full nodes are equiped to handle.

## Changelog

This patch release consists of 2 commits from 1 contributor which total to 3
files changed, 55 additional lines of code, and 31 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.6.1...release-v1.6.2).

### Protocol and network:

- server: Only send fast block anns to full nodes ([decred/dcrd#2609](https://github.com/decred/dcrd/pull/2609))

### Misc:

- release: Bump for 1.6.2 ([decred/dcrd#2629](https://github.com/decred/dcrd/pull/2629))

### Code Contributors (alphabetical order):

- Dave Collins

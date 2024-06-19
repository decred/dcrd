# dcrd v2.0.3 Release Notes

This is a patch release of dcrd which includes the following changes:

- Improved sender privacy for transactions and mix messages via randomized
  announcements
- Nodes now prefer to maintain at least three mixing-capable outbound connections
- Recent transactions and mix messages will now be available to serve for longer
- Reduced memory usage during periods of lower activity
- Mixing-related performance enhancements

## Changelog

This patch release consists of 26 commits from 2 contributors which total to 37
files changed, 4527 additional lines of code, and 499 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.0.2...release-v2.0.3).

### Protocol and network:

- [release-v2.0] peer: Randomize inv batching delays ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] server: Cache advertised txns ([decred/dcrd#3392](https://github.com/decred/dcrd/pull/3392))
- [release-v2.0] server: Prefer 3 min mix capable outbound peers ([decred/dcrd#3392](https://github.com/decred/dcrd/pull/3392))

### Mixing message relay (mix pool):

- [release-v2.0] mixpool: Cache recently removed msgs ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixclient: Introduce random message jitter ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] netsync: Remove spent PRs from tip block txns ([decred/dcrd#3392](https://github.com/decred/dcrd/pull/3392))

### Documentation:

- [release-v2.0] docs: Update for container/lru module ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Add README.md ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))

### Developer-related package and module changes:

- [release-v2.0] container/lru: Implement type safe generic LRUs ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] peer: Use container/lru module ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] crypto/rand: Implement module ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Add BigInt ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Uppercase N in half-open-range funcs ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Add rand.N generic func ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Add ShuffleSlice ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] rand: Add benchmarks ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] peer: Use new crypto/rand module ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] mixing: Prealloc buffers ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixpool: Remove Receive expectedMessages argument ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixing: Use new crypto/rand module ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixpool: Remove run from conflicting msg err ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixpool: Remove more references to runs ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))
- [release-v2.0] mixing: Reduce slot reservation mix pads allocs ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))

### Developer-related module management:

- [release-v2.0] main: Use backported peer updates ([decred/dcrd#3390](https://github.com/decred/dcrd/pull/3390))
- [release-v2.0] main: Use backported mixing updates ([decred/dcrd#3391](https://github.com/decred/dcrd/pull/3391))

### Misc:

- [release-v2.0] release: Bump for 2.0.3 ([decred/dcrd#3393](https://github.com/decred/dcrd/pull/3393))

### Code Contributors (alphabetical order):

- Dave Collins
- Josh Rickmar

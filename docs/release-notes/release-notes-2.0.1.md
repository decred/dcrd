# dcrd v2.0.1 Release Notes

This is a patch release of dcrd which includes the following key changes:

* Provides a new JSON-RPC API method named `getmixmessage` that can be used to
  query decentralized StakeShuffle mixing messages
* No longer relays mixing messages when transaction relay is disabled
* Transaction outputs with one confirmation may now be used as part of a mix
* Improves best network address candidate selection
* More consistent logging of banned peers along with the reason they were banned

## Changelog

This patch release consists of 19 commits from 3 contributors which total to 18
files changed, 388 additional lines of code, and 187 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.0.0...release-v2.0.1).

### Protocol and network:

- netsync: Request new headers on rejected tip ([decred/dcrd#3315](https://github.com/decred/dcrd/pull/3315))
- dcrd: Make DisableRelayTx also include mixing messages ([decred/dcrd#3315](https://github.com/decred/dcrd/pull/3315))
- server: Add logs on why a peer is banned ([decred/dcrd#3318](https://github.com/decred/dcrd/pull/3318))

### RPC:

- dcrjson,rpcserver,types: Add getmixmessage ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- rpcserver: Allow getmixmessage from limited users ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))

### Mixing message relay (mix pool):

- mixpool: Require 1 block confirmation ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))

### Documentation:

- docs: Add getmixmessage ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))

### Developer-related package and module changes:

- addrmgr: Give unattempted addresses a chance ([decred/dcrd#3316](https://github.com/decred/dcrd/pull/3316))
- mixpool: Add missing mutex acquire ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- mixing: Use stdaddr.Hash160 instead of dcrutil ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- mixing: Reduce a couple of allocation cases ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- mixpool: Wrap non-bannable errors with RuleError ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- mixclient: Do not remove PRs from failed runs ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- mixclient: Improve error returning ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))
- server: Make peer banning synchronous ([decred/dcrd#3318](https://github.com/decred/dcrd/pull/3318))
- server: Consolidate ban reason logging ([decred/dcrd#3318](https://github.com/decred/dcrd/pull/3318))

### Developer-related module management:

- main: Use backported addrmgr updates ([decred/dcrd#3316](https://github.com/decred/dcrd/pull/3316))
- main: Use backported module updates ([decred/dcrd#3317](https://github.com/decred/dcrd/pull/3317))

### Misc:

- release: Bump for 2.0.1 ([decred/dcrd#3319](https://github.com/decred/dcrd/pull/3319))

### Code Contributors (alphabetical order):

- Dave Collins
- David Hill
- Josh Rickmar

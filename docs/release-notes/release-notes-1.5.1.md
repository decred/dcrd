# dcrd v1.5.1

This is a patch release of dcrd to address a minor memory leak with authenticated RPC websocket clients on intermittent connections.   It also updates the `dcrctl` utility to include the new `auditreuse` dcrwallet command.

## Changelog

This patch release consists of 4 commits from 3 contributors which total to 4 files changed, 27 additional lines of code, and 6 deleted lines of code.

All commits since the last release may be viewed on GitHub [here](https://github.com/decred/dcrd/compare/release-v1.5.0...release-v1.5.1).

### RPC:

- rpcwebsocket: Remove client from missed maps ([decred/dcrd#2049](https://github.com/decred/dcrd/pull/2049))
- rpcwebsocket: Use nonblocking messages and ntfns ([decred/dcrd#2050](https://github.com/decred/dcrd/pull/2050))

### dcrctl utility changes:

- dcrctl: Update dcrwallet RPC types package ([decred/dcrd#2051](https://github.com/decred/dcrd/pull/2051))

### Misc:

- release: Bump for 1.5.1([decred/dcrd#2052](https://github.com/decred/dcrd/pull/2052))

### Code Contributors (alphabetical order):

- Dave Collins
- Josh Rickmar
- Matheus Degiovani

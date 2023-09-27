# dcrd v1.8.1

This is a patch release of dcrd that includes some updates to the RPC server and
JSON-RPC API in light of the changes made by
[DCP0011](https://github.com/decred/dcps/blob/master/dcp-0011/dcp-0011.mediawiki) as follows:

* The `getblock` and `getblockheader` RPCs now have an additional `powhash`
  field for the new Proof-of-Work hash
* The `getnetworkhashps` RPC now treats -1 for the blocks parameter as the
  default number of blocks versus the previous behavior that is no longer
  applicable to the new difficulty adjustment algorithm

The RPC server version as of this release is 8.1.0.

## Changelog

This patch release consists of 5 commits from 2 contributors which total to 7
files changed, 47 additional lines of code, and 29 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.8.0...release-v1.8.1).

### RPC:

- rpc: Add PoWHash to getblock/getblockheader (verbose) results ([decred/dcrd#3192](https://github.com/decred/dcrd/pull/3192))
- rpcserver: Modify getnetworkhashps -1 blocks logic ([decred/dcrd#3193](https://github.com/decred/dcrd/pull/3193))

### Developer-related package and module changes:

- jsonrpc/types: Add powhash to verbose block output ([decred/dcrd#3192](https://github.com/decred/dcrd/pull/3192))
- main: Use backported rpc types updates ([decred/dcrd#3192](https://github.com/decred/dcrd/pull/3192))

### Misc:

- release: Bump for 1.8.1 ([decred/dcrd#3194](https://github.com/decred/dcrd/pull/3194))

### Code Contributors (alphabetical order):

- Dave Collins
- Jonathan Chappelow

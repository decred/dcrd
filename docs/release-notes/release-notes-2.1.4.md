# dcrd v2.1.4 Release Notes

This is a patch release of dcrd which includes the following main changes:

- Various fixes for potential denial-of-service attacks
- RPC server now additionally rejects cross origin requests from reverse proxies
- RPC server auth behavior for limit users with an extremely unlikely
  combination of config settings now behaves as intended
- Peers will no longer consider services before handshake completion
- Reduced memory allocations for peer-to-peer network operations
- More efficient use of mixing message dimensions
- Improved handling of mixing message orphans

## Upgrade Highly Recommended

Everyone is strongly encouraged to upgrade their software to this latest patch
release.  It contains various fixes for potential denial-of-service (DoS)
attacks that could possibly be used by malicious actors to disrupt service.

## Changelog

This patch release consists of 46 commits from 3 contributors which total to 51
files changed, 2176 additional lines of code, and 1142 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.1.3...release-v2.1.4).

### Protocol and network:

- [release-v2.1] wire: Optimize writes to bytes.Buffer ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Optimize writes to hashers ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Clamp max allowed timestamp values ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Optimize message reads ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Reject messages with trailing bytes ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] server: Don't consider services before handshake ([decred/dcrd#3660](https://github.com/decred/dcrd/pull/3660))

### Mixing message relay (mix pool):

- [release-v2.1] mixing: Optimize message signing and sig verification ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Implement message source tracking ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Proactive orphan limit eviction ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixing: Impose stricter message limits ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Reject PRs duplicating inputs ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))

### RPC:

- [release-v2.1] rpcserver: Clean up existing authentication tests ([decred/dcrd#3660](https://github.com/decred/dcrd/pull/3660))
- [release-v2.1] rpcserver: Ensure limited user is always limited ([decred/dcrd#3660](https://github.com/decred/dcrd/pull/3660))
- [release-v2.1] rpcserver: Fix CheckOrigin inverted err check ([decred/dcrd#3660](https://github.com/decred/dcrd/pull/3660))

### Developer-related package and module changes:

- [release-v2.1] wire: Encode header and payload to single buffer ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Do not read discarded input ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Fix {read,write}Element *[4]byte case ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Remove unnecessary nil check from test ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Clear tx inputs/outputs before deserialize ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Use io.ReadFull in shortRead slow path ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] mixing: Remove reference to x25519 ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixing: Avoid hash.Hash and Sum(nil) calls ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] rpcserver: Rename mix msg acceptance iface method ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Use slices for containment check ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Rename orphan type to orphanMsg ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Consolidate orphan addition ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Consolidate orphan removal ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixpool: Store orphan struct vs msg in id map ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] blockchain: Cleanup input checks ([decred/dcrd#3660](https://github.com/decred/dcrd/pull/3660))
- [release-v2.1] mixing: Make MaxMixAmount a typed int64 const ([decred/dcrd#3664](https://github.com/decred/dcrd/pull/3664))
- [release-v2.1] mixing: Fix P2SH script check ([decred/dcrd#3665](https://github.com/decred/dcrd/pull/3665))

### Developer-related module management:

- [release-v2.1] wire: Prepare v1.7.4 ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] main: Use backported wire updates ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3664](https://github.com/decred/dcrd/pull/3664))
- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3665](https://github.com/decred/dcrd/pull/3665))

### Testing and Quality Assurance:

- [release-v2.1] wire: Add a couple of missing err stringer tests ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Add tests for short reads ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] wire: Improve read message error path tests ([decred/dcrd#3658](https://github.com/decred/dcrd/pull/3658))
- [release-v2.1] mixing/utxoproof: Add benchmarks ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixing: Add signature tests and benchmarks ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixing: Increment PRNG seed nonce for each test ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixclient: Disable logs to backend after test finish ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] mixclient: Avoid test hangs caused by epoch ticker ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))

### Misc:

- [release-v2.1] server: Update mempool orphan eviction debug log ([decred/dcrd#3659](https://github.com/decred/dcrd/pull/3659))
- [release-v2.1] release: Bump for 2.1.4 ([decred/dcrd#3661](https://github.com/decred/dcrd/pull/3661))

### Code Contributors (alphabetical order):

- Dave Collins
- Jamie Holdstock
- Josh Rickmar

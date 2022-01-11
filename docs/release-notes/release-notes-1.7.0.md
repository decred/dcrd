# dcrd v1.7.0

This is a new major release of dcrd.  Some of the key highlights are:

* Four new consensus vote agendas which allow stakeholders to decide whether or
  not to activate support for the following:
  * Reverting the Treasury maximum expenditure policy
  * Enforcing explicit version upgrades
  * Support for automatic ticket revocations for missed votes
  * Changing the Proof-of-Work and Proof-of-Stake subsidy split from 60%/30% to 10%/80%
* Substantially reduced initial sync time
* Major performance enhancements to unspent transaction output handling
* Faster cryptographic signature validation
* Significant improvements to network synchronization
* Support for a configurable assumed valid block
* Block index memory usage reduction
* Asynchronous indexing
* Version 1 block filters removal
* Various updates to the RPC server:
  * Additional per-connection read limits
  * A more strict cross origin request policy
  * A new alternative client authentication mechanism based on TLS certificates
  * Availability of the scripting language version for transaction outputs
  * Several other notable updates, additions, and removals related to the JSON-RPC API
* New developer modules:
  * Age-Partitioned Bloom Filters
  * Fixed-Precision Unsigned 256-bit Integers
  * Standard Scripts
  * Standard Addresses
* Infrastructure improvements
* Quality assurance changes

For those unfamiliar with the
[voting process](https://docs.decred.org/governance/consensus-rule-voting/overview/)
in Decred, all code needed in order to support each of the aforementioned
consensus changes is already included in this release, however it will remain
dormant until the stakeholders vote to activate it.

For reference, the consensus change work for each of the four changes was
originally proposed and approved for initial implementation via the following
Politeia proposals:
- [Decentralized Treasury Spending](https://proposals-archive.decred.org/proposals/c96290a)
- [Explicit Version Upgrades Consensus Change](https://proposals.decred.org/record/3a98861)
- [Automatic Ticket Revocations Consensus Change](https://proposals.decred.org/record/e2d7b7d)
- [Change PoW/PoS Subsidy Split From 60/30 to 10/80](https://proposals.decred.org/record/427e1d4)

The following Decred Change Proposals (DCPs) describe the proposed changes in
detail and provide full technical specifications:
- [DCP0007](https://github.com/decred/dcps/blob/master/dcp-0007/dcp-0007.mediawiki)
- [DCP0008](https://github.com/decred/dcps/blob/master/dcp-0008/dcp-0008.mediawiki)
- [DCP0009](https://github.com/decred/dcps/blob/master/dcp-0009/dcp-0009.mediawiki)
- [DCP0010](https://github.com/decred/dcps/blob/master/dcp-0010/dcp-0010.mediawiki)

## Upgrade Required

**It is extremely important for everyone to upgrade their software to this
latest release even if you don't intend to vote in favor of the agenda.  This
particularly applies to PoW miners as failure to upgrade will result in lost
rewards after block height 635775.  That is estimated to be around Feb 21st,
2022.**

## Downgrade Warning

The database format in v1.7.0 is not compatible with previous versions of the
software.  This only affects downgrades as users upgrading from previous
versions will see a one time database migration.

Once this migration has been completed, it will no longer be possible to
downgrade to a previous version of the software without having to delete the
database and redownload the chain.

The database migration typically takes around 40-50 minutes on HDDs and 20-30
minutes on SSDs.

## Notable Changes

### Four New Consensus Change Votes

Four new consensus change votes are now available as of this release.  After
upgrading, stakeholders may set their preferences through their wallet.

#### Revert Treasury Maximum Expenditure Policy Vote

The first new vote available as of this release has the id `reverttreasurypolicy`.

The primary goal of this change is to revert the currently active maximum
expenditure policy of the decentralized Treasury to the one specified in the
[original Politeia proposal](https://proposals-archive.decred.org/proposals/c96290a).

See [DCP0007](https://github.com/decred/dcps/blob/master/dcp-0007/dcp-0007.mediawiki) for
the full technical specification.

#### Explicit Version Upgrades Vote

The second new vote available as of this release has the id `explicitverupgrades`.

The primary goals of this change are to:

* Provide an easy, reliable, and efficient method for software and hardware to
  determine exactly which rules should be applied to transaction and script
  versions
* Further embrace the increased security and other desirable properties that
  hard forks provide over soft forks

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/3a98861)
* [DCP0008](https://github.com/decred/dcps/blob/master/dcp-0008/dcp-0008.mediawiki)

#### Automatic Ticket Revocations Vote

The third new vote available as of this release has the id `autorevocations`.

The primary goals of this change are to:

* Improve the Decred stakeholder user experience by removing the requirement for
  stakeholders to manually revoke missed and expired tickets
* Enable the recovery of funds for users who lost their redeem script for the
  legacy VSP system (before the release of vspd, which removed the need for the
  redeem script)

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/e2d7b7d)
* [DCP0009](https://github.com/decred/dcps/blob/master/dcp-0009/dcp-0009.mediawiki)

#### Change PoW/PoS Subsidy Split to 10/80 Vote

The fourth new vote available as of this release has the id `changesubsidysplit`.

The proposed modification to the subsidy split is intended to substantially
diminish the ability to attack Decred's markets with mined coins and improve
decentralization of the issuance process.

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/427e1d4)
* [DCP0010](https://github.com/decred/dcps/blob/master/dcp-0010/dcp-0010.mediawiki)

### Substantially Reduced Initial Sync Time

The amount of time it takes to complete the initial chain synchronization
process has been substantially reduced.  With default settings, it is around 48%
faster versus the previous release.

### Unspent Transaction Output Overhaul

The way unspent transaction outputs (UTXOs) are handled has been significantly
reworked to provide major performance enhancements to both steady-state
operation as well as the initial chain sync process as follows:

* Each UTXO is now tracked independently on a per-output basis
* The UTXOs now reside in a dedicated database
* All UTXO reads and writes now make use of a cache

#### Unspent Transaction Output Cache

All reads and writes of unspent transaction outputs (utxos) now go through a
cache that sits on top of the utxo set database which drastically reduces the
amount of reading and writing to disk, especially during the initial sync
process when a very large number of blocks are being processed in quick
succession.

This utxo cache provides significant runtime performance benefits at the cost of
some additional memory usage.  The maximum size of the cache can be configured
with the new `--utxocachemaxsize` command-line configuration option.  The
default value is 150 MiB, the minimum value is 25 MiB, and the maximum value is
32768 MiB (32 GiB).

Some key properties of the cache are as follows:

* For reads, the UTXO cache acts as a read-through cache
  * All UTXO reads go through the cache
  * Cache misses load the missing data from the disk and cache it for future lookups
* For writes, the UTXO cache acts as a write-back cache
  * Writes to the cache are acknowledged by the cache immediately, but are only
    periodically flushed to disk
* Allows intermediate steps to effectively be skipped thereby avoiding the need
  to write millions of entries to disk
* On average, recent UTXOs are much more likely to be spent in upcoming blocks
  than older UTXOs, so only the oldest UTXOs are evicted as needed in order to
  maximize the hit ratio of the cache
* The cache is periodically flushed with conditional eviction:
  * When the cache is NOT full, nothing is evicted, but the changes are still
    written to the disk set to allow for a quicker reconciliation in the case of
    an unclean shutdown
  * When the cache is full, 15% of the oldest UTXOs are evicted

### Faster Cryptographic Signature Validation

Some aspects of the underlying crypto code has been updated to further improve
its execution speed and reduce the number of memory allocations resulting in
about a 1% reduction to signature verification time.

The primary benefits are:

* Improved vote times since blocks and transactions propagate more quickly
  throughout the network
* Approximately a 2% reduction to the duration of the initial sync process

### Significant Improvements to Network Synchronization

The method used to obtain blocks from other peers on the network is now guided
entirely by block headers.  This provides a wide variety of benefits, but the
most notable ones for most users are:

* Faster initial synchronization
* Reduced bandwidth usage
* Enhanced protection against attempted DoS attacks
* Percentage-based progress reporting
* Improved steady state logging

### Support for Configurable Assumed Valid Block

This release introduces a new model for deciding when several historical
validation checks may be skipped for blocks that are an ancestor of a known good
block.

Specifically, a new `AssumeValid` parameter is now used to specify the
aforementioned known good block.  The default value of the parameter is updated
with each release to a recent block that is part of the main chain.

The default value of the parameter can be overridden with the `--assumevalid`
command-line option by setting it as follows:

* `--assumevalid=0`: Disable the feature resulting in no skipped validation checks
* `--assumevalid=[blockhash]`:  Set `AssumeValid` to the specified block hash

Specifying a block hash closer to the current best chain tip allows for faster
syncing.  This is useful since the validation requirements increase the longer a
particular release build is out as the default known good block becomes deeper
in the chain.

### Block Index Memory Usage Reduction

The block index that keeps track of block status and connectivity now occupies
around 30MiB less memory and scales better as more blocks are added to the
chain.

### Asynchronous Indexing

The various optional indexes are now created asynchronously versus when
blocks are processed as was previously the case.

This permits blocks to be validated more quickly when the indexes are enabled
since the validation no longer needs to wait for the indexing operations to
complete.

In order to help keep consistent behavior for RPC clients, RPCs that involve
interacting with the indexes will not return results until the associated
indexing operation completes when the indexing tip is close to the current best
chain tip.

One side effect of this change that RPC clients should be aware of is that it is
now possible to receive sync timeout errors on RPCs that involve interacting
with the indexes if the associated indexing tip gets so far behind it would end
up delaying results for too long.  In practice, errors of this type are rare and
should only ever be observed during the initial sync process before the
associated indexes are current.  However, callers should be aware of the
possibility and handle the error accordingly.

The following RPCs are affected:

* `existsaddress`
* `existsaddresses`
* `getrawtransaction`
* `searchrawtransactions`

### Version 1 Block Filters Removal

The previously deprecated version 1 block filters are no longer available on the
peer-to-peer network.  Use
[version 2 block filters](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#version-2-block-filters)
with their associated
[block header commitment](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#block-header-commitments)
and [inclusion proof](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#verifying-commitment-root-inclusion-proofs)
instead.

### RPC Server Changes

The RPC server version as of this release is 7.0.0.

#### Max Request Limits

The RPC server now imposes the following additional per-connection read limits
to help further harden it against potential abuse in non-standard configurations
on poorly-configured networks:

* 0 B / 8 MiB for pre and post auth HTTP connections
* 4 KiB / 16 MiB for pre and post auth WebSocket connections

In practice, these changes will not have any noticeable effect for the vast
majority of nodes since the RPC server is not publicly accessible by default and
also requires authentication.

Nevertheless, it can still be useful for scenarios such as authenticated fuzz
testing and improperly-configured networks that have disabled all other security
measures.

#### More Strict Cross Origin Request (CORS) Policy

The CORS policy for WebSocket clients is now more strict and rejects requests
from other domains.

In practice, CORS requests will be rejected before ever reaching that point due
to the use of a self-signed TLS certificate and the requirement for
authentication to issue any commands.  However, additional protection mechanisms
make it that much more difficult to attack by providing defense in depth.

#### Alternative Client Authentication Method Based on TLS Certificates

A new alternative method for TLS clients to authenticate to the RPC server by
presenting a client certificate in the TLS handshake is now available.

Under this authentication method, the certificate authority for a client
certificate must be added to the RPC server as a trusted root in order for it to
trust the client.  Once activated, clients will no longer be required to provide
HTTP Basic authentication nor use the `authenticate` RPC in the case of
WebSocket clients.

Note that while TLS client authentication has the potential to ultimately allow
more fine grained access controls on a per-client basis, it currently only
supports clients with full administrative privileges.  In other words, it is not
currently compatible with the `--rpclimituser` and `--rpclimitpass` mechanism,
so users depending on the limited user settings should avoid the new
authentication method for now.

The new authentication type can be activated with the `--authtype=clientcert`
configuration option.

By default, the trusted roots are loaded from the `clients.pem` file in dcrd's
application data directory, however, that location can be modified via the
`--clientcafile` option if desired.

#### Updates to Transaction Output Query RPC (`gettxout`)

The `gettxout` RPC has the following modifications:

* An additional `tree` parameter is now required in order to explicitly identify
  the exact transaction output being requested
* The transaction `version` field is no longer available in the primary JSON
  object of the results
* The child `scriptPubKey` JSON object in the results now includes a new
  `version` field that identifies the scripting language version

See the
[gettxout JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#gettxout)
for API details.

#### Removal of Stake Difficulty Notification RPCs (`notifystakedifficulty` and `stakedifficulty`)

The deprecated `notifystakedifficulty` and `stakedifficulty` WebSocket-only RPCs
are no longer available.  This notification is unnecessary since the difficulty
change interval is well defined.  Callers may obtain the difficulty via
`getstakedifficulty` at the appropriate difficulty change intervals instead.

See the
[getstakedifficulty JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getstakedifficulty)
for API details.

#### Removal of Version 1 Filter RPCs (`getcfilter` and `getcfilterheader`)

The deprecated `getcfilter` and `getcfilterheader` RPCs, which were previously
used to obtain version 1 block filters via RPC are no longer available. Use
`getcfilterv2` instead.

See the
[getcfilterv2 JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getcfilterv2)
for API details.

#### New Median Time Field on Block Query RPCs (`getblock` and `getblockheader`)

The verbose results of the `getblock` and `getblockheader` RPCs now include a
`mediantime` field that specifies the median block time associated with the
block.

See the following for API details:

* [getblock JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getblock)
* [getblockheader JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getblockheader)

#### New Scripting Language Version Field on Raw Transaction RPCs (`getrawtransaction`, `decoderawtransaction`, `searchrawtransactions`, and `getblock`)

The verbose results of the `getrawtransaction`, `decoderawtransaction`,
`searchrawtransactions`, and `getblock` RPCs now include a `version` field in
the child `scriptPubKey` JSON object that identifies the scripting language
version.

See the following for API details:

* [getrawtransaction JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getrawtransaction)
* [decoderawtransaction JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#decoderawtransaction)
* [searchrawtransactions JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#searchrawtransactions)
* [getblock JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getblock)

#### New Treasury Add Transaction Filter on Mempool Query RPC (`getrawmempool`)

The transaction type parameter of the `getrawmempool` RPC now accepts `tadd` to
only include treasury add transactions in the results.

See the
[getrawmempool JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getrawmempool)
for API details.

#### New Manual Block Invalidation and Reconsideration RPCs (`invalidateblock` and `reconsiderblock`)

A new pair of RPCs named `invalidateblock` and `reconsiderblock` are now
available.  These RPCs can be used to manually invalidate a block as if it had
violated consensus rules and reconsider a block for validation and best chain
selection by removing any invalid status from it and its ancestors, respectively.

This capability is provided for development, testing, and debugging.  It can be
particularly useful when developing services that build on top of Decred to more
easily ensure edge conditions associated with invalid blocks and chain
reorganization are being handled properly.

These RPCs do not apply to regular users and can safely be ignored outside of
development.

See the following for API details:

* [invalidateblock JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#invalidateblock)
* [reconsiderblock JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#reconsiderblock)

### Reject Protocol Message Deprecated (`reject`)

The `reject` peer-to-peer protocol message is now deprecated and is scheduled to
be removed in a future release.

This message is a holdover from the original codebase where it was required, but
it really is not a useful message and has several downsides:

* Nodes on the network must be trustless, which means anything relying on such a
  message is setting itself up for failure because nodes are not obligated to
  send it at all nor be truthful as to the reason
* It can be harmful to privacy as it allows additional node fingerprinting
* It can lead to security issues for implementations that don't handle it with
  proper sanitization practices
* It can easily give software implementations the fully incorrect impression
  that it can be relied on for determining if transactions and blocks are valid
* The only way it is actually used currently is to show a debug log message,
  however, all of that information is already available via the peer and/or wire
  logging anyway
* It carries a non-trivial amount of development overhead to continue to support
  it when nothing actually uses it

### No DNS Seeds Command-Line Option Deprecated (`--nodnsseed`)

The `--nodnsseed` command-line configuration option is now deprecated and will
be removed in a future release.  Use `--noseeders` instead.

DNS seeding has not been used since the previous release.

## Notable New Developer Modules

### Age-Partitioned Bloom Filters

A new `github.com/decred/dcrd/container/apbf` module is now available that
provides Age-Partitioned Bloom Filters (APBFs).

An APBF is a probabilistic lookup device that can quickly determine if it
contains an element.  It permits tracking large amounts of data while using very
little memory at the cost of a controlled rate of false positives.  Unlike
classic Bloom filters, it is able to handle an unbounded amount of data by aging
and discarding old items.

For a concrete example of actual savings achieved in Decred by making use of an
APBF, the memory to track addresses known by 125 peers was reduced from ~200 MiB
to ~5 MiB.

See the
[apbf module documentation](https://pkg.go.dev/github.com/decred/dcrd/container/apbf)
for full details on usage, accuracy under workloads, expected memory usage, and
performance benchmarks.

### Fixed-Precision Unsigned 256-bit Integers

A new `github.com/decred/dcrd/math/uint256` module is now available that provides
highly optimized allocation free fixed precision unsigned 256-bit integer
arithmetic.

The package has a strong focus on performance and correctness and features
arithmetic, boolean comparison, bitwise logic, bitwise shifts, conversion
to/from relevant types, and full formatting support - all served with an
ergonomic API, full test coverage, and benchmarks.

Every operation is faster than the standard library `big.Int` equivalent and the
primary math operations provide reductions of over 90% in the calculation time.
Most other operations are also significantly faster.

See the
[uint256 module documentation](https://pkg.go.dev/github.com/decred/dcrd/math/uint256)
for full details on usage, including a categorized summary, and performance
benchmarks.

### Standard Scripts

A new `github.com/decred/dcrd/txscript/v4/stdscript` package is now available
that provides facilities for identifying and extracting data from transaction
scripts that are considered standard by the default policy of most nodes.

The package is part of the `github.com/decred/dcrd/txscript/v4` module.

See the
[stdscript package documentation](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdscript)
for full details on usage and a list of the recognized standard scripts.

### Standard Addresses

A new `github.com/decred/dcrd/txscript/v4/stdaddr` package is now available that
provides facilities for working with human-readable Decred payment addresses.

The package is part of the `github.com/decred/dcrd/txscript/v4` module.

See the
[stdaddr package documentation](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4/stdaddr)
for full details on usage and a list of the supported addresses.

## Changelog

This release consists of 877 commits from 16 contributors which total to 492
files changed, 77937 additional lines of code, and 30961 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.6.0...release-v1.7.0).

### Protocol and network:

- chaincfg: Add extra seeders ([decred/dcrd#2532](https://github.com/decred/dcrd/pull/2532))
- server: Stop serving v1 cfilters over p2p ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- blockchain: Decouple processing and download logic ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- blockchain: Improve current detection ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- netsync: Rework inventory announcement handling ([decred/dcrd#2548](https://github.com/decred/dcrd/pull/2548))
- peer: Add inv type summary to debug message ([decred/dcrd#2556](https://github.com/decred/dcrd/pull/2556))
- netsync: Remove unused submit block flags param ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- netsync: Remove submit/processblock orphan flag ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- netsync: Remove orphan block handling ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- netsync: Rework sync model to use hdr annoucements ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- progresslog: Add support for header sync progress ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- netsync: Add header sync progress log ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- multi: Add chain verify progress percentage ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- peer: Remove getheaders response deadline ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- chaincfg: Update seed URL ([decred/dcrd#2564](https://github.com/decred/dcrd/pull/2564))
- upnp: Don't return loopback IPs in getOurIP ([decred/dcrd#2566](https://github.com/decred/dcrd/pull/2566))
- server: Prevent duplicate pending conns ([decred/dcrd#2563](https://github.com/decred/dcrd/pull/2563))
- multi: Use an APBF for recently confirmed txns ([decred/dcrd#2580](https://github.com/decred/dcrd/pull/2580))
- multi: Use an APBF for per peer known addrs ([decred/dcrd#2583](https://github.com/decred/dcrd/pull/2583))
- peer: Stop sending and logging reject messages ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- netsync: Stop sending reject messages ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- server: Stop sending reject messages ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- peer: Remove deprecated onversion reject return ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- peer: Remove unneeded PushRejectMsg ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- wire: Deprecate reject message ([decred/dcrd#2586](https://github.com/decred/dcrd/pull/2586))
- server: Respond to getheaders when same chain tip ([decred/dcrd#2587](https://github.com/decred/dcrd/pull/2587))
- netsync: Use an APBF for recently rejected txns ([decred/dcrd#2590](https://github.com/decred/dcrd/pull/2590))
- server: Only send fast block anns to full nodes ([decred/dcrd#2606](https://github.com/decred/dcrd/pull/2606))
- upnp: More accurate getOurIP ([decred/dcrd#2571](https://github.com/decred/dcrd/pull/2571))
- server: Correct tx not found ban reason ([decred/dcrd#2677](https://github.com/decred/dcrd/pull/2677))
- chaincfg: Add DCP0007 deployment ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- chaincfg: Introduce explicit ver upgrades agenda ([decred/dcrd#2713](https://github.com/decred/dcrd/pull/2713))
- blockchain: Implement reject new tx vers vote ([decred/dcrd#2716](https://github.com/decred/dcrd/pull/2716))
- blockchain: Implement reject new script vers vote ([decred/dcrd#2716](https://github.com/decred/dcrd/pull/2716))
- chaincfg: Add agenda for auto ticket revocations ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- multi: DCP0009 Auto revocations consensus change ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- chaincfg: Use single latest checkpoint ([decred/dcrd#2762](https://github.com/decred/dcrd/pull/2762))
- peer: Offset ping interval from idle timeout ([decred/dcrd#2796](https://github.com/decred/dcrd/pull/2796))
- chaincfg: Update checkpoint for upcoming release ([decred/dcrd#2794](https://github.com/decred/dcrd/pull/2794))
- chaincfg: Update min known chain work for release ([decred/dcrd#2795](https://github.com/decred/dcrd/pull/2795))
- netsync: Request init state immediately upon sync ([decred/dcrd#2812](https://github.com/decred/dcrd/pull/2812))
- blockchain: Reject old block vers for HFV ([decred/dcrd#2752](https://github.com/decred/dcrd/pull/2752))
- netsync: Rework next block download logic ([decred/dcrd#2828](https://github.com/decred/dcrd/pull/2828))
- chaincfg: Add AssumeValid param ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- chaincfg: Introduce subsidy split change agenda ([decred/dcrd#2847](https://github.com/decred/dcrd/pull/2847))
- multi: Implement DCP0010 subsidy consensus vote ([decred/dcrd#2848](https://github.com/decred/dcrd/pull/2848))
- server: Force PoW upgrade to v9 ([decred/dcrd#2875](https://github.com/decred/dcrd/pull/2875))

### Transaction relay (memory pool):

- mempool: Limit ancestor tracking in mempool ([decred/dcrd#2458](https://github.com/decred/dcrd/pull/2458))
- mempool: Remove old fix sequence lock rejection ([decred/dcrd#2496](https://github.com/decred/dcrd/pull/2496))
- mempool: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- mempool: Enforce explicit versions ([decred/dcrd#2716](https://github.com/decred/dcrd/pull/2716))
- mempool: Remove unneeded max tx ver std checks ([decred/dcrd#2716](https://github.com/decred/dcrd/pull/2716))
- mempool: Update fraud proof data ([decred/dcrd#2804](https://github.com/decred/dcrd/pull/2804))
- mempool: CheckTransactionInputs check fraud proof ([decred/dcrd#2804](https://github.com/decred/dcrd/pull/2804))

### Mining:

- mining: Move txPriorityQueue to a separate file ([decred/dcrd#2431](https://github.com/decred/dcrd/pull/2431))
- mining: Move interfaces to mining/interface.go ([decred/dcrd#2431](https://github.com/decred/dcrd/pull/2431))
- mining: Add method comments to blockManagerFacade ([decred/dcrd#2431](https://github.com/decred/dcrd/pull/2431))
- mining: Move BgBlkTmplGenerator to separate file ([decred/dcrd#2431](https://github.com/decred/dcrd/pull/2431))
- mining: Prevent panic in child prio item handling ([decred/dcrd#2434](https://github.com/decred/dcrd/pull/2434))
- mining: Add Config struct to house mining params ([decred/dcrd#2436](https://github.com/decred/dcrd/pull/2436))
- mining: Move block chain functions to Config ([decred/dcrd#2436](https://github.com/decred/dcrd/pull/2436))
- mining: Move txMiningView from mempool package ([decred/dcrd#2467](https://github.com/decred/dcrd/pull/2467))
- mining: Switch to custom waitGroup impl ([decred/dcrd#2477](https://github.com/decred/dcrd/pull/2477))
- mining: Remove leftover block manager facade iface ([decred/dcrd#2510](https://github.com/decred/dcrd/pull/2510))
- mining: No error log on expected head reorg errors ([decred/dcrd#2560](https://github.com/decred/dcrd/pull/2560))
- mining: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- mining: Add error kinds for auto revocations ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- mining: Add auto revocation priority to tx queue ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- mining: Add HeaderByHash to Config ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- mining: Prevent unnecessary reorg with equal votes ([decred/dcrd#2840](https://github.com/decred/dcrd/pull/2840))
- mining: Update to latest block vers for HFV ([decred/dcrd#2753](https://github.com/decred/dcrd/pull/2753))

### RPC:

- rpcserver: Upgrade is deprecated; switch to Upgrader ([decred/dcrd#2409](https://github.com/decred/dcrd/pull/2409))
- multi: Add TAdd support to getrawmempool ([decred/dcrd#2448](https://github.com/decred/dcrd/pull/2448))
- rpcserver: Update getrawmempool txtype help ([decred/dcrd#2452](https://github.com/decred/dcrd/pull/2452))
- rpcserver: Hash auth using random-keyed MAC ([decred/dcrd#2486](https://github.com/decred/dcrd/pull/2486))
- rpcserver: Use next stake diff from snapshot ([decred/dcrd#2493](https://github.com/decred/dcrd/pull/2493))
- rpcserver: Make authenticate match header auth ([decred/dcrd#2502](https://github.com/decred/dcrd/pull/2502))
- rpcserver: Check unauthorized access in const time ([decred/dcrd#2509](https://github.com/decred/dcrd/pull/2509))
- multi: Subscribe for work ntfns in rpcserver ([decred/dcrd#2501](https://github.com/decred/dcrd/pull/2501))
- rpcserver: Prune block templates in websocket path ([decred/dcrd#2503](https://github.com/decred/dcrd/pull/2503))
- rpcserver: Remove version from gettxout result ([decred/dcrd#2517](https://github.com/decred/dcrd/pull/2517))
- rpcserver: Add tree param to gettxout ([decred/dcrd#2517](https://github.com/decred/dcrd/pull/2517))
- rpcserver/netsync: Remove notifystakedifficulty ([decred/dcrd#2519](https://github.com/decred/dcrd/pull/2519))
- rpcserver: Remove v1 getcfilter{,header} ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- rpcserver: Remove unused Filterer interface ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- rpcserver: Update getblockchaininfo best header ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- rpcserver: Remove unused LocateBlocks iface method ([decred/dcrd#2538](https://github.com/decred/dcrd/pull/2538))
- rpcserver: Allow TLS client cert authentication ([decred/dcrd#2482](https://github.com/decred/dcrd/pull/2482))
- rpcserver: Add invalidate/reconsiderblock support ([decred/dcrd#2536](https://github.com/decred/dcrd/pull/2536))
- rpcserver: Support getblockchaininfo genesis block ([decred/dcrd#2550](https://github.com/decred/dcrd/pull/2550))
- rpcserver: Calc verify progress based on best hdr ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- rpcserver: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- rpcserver: Allow gettreasurybalance empty blk str ([decred/dcrd#2640](https://github.com/decred/dcrd/pull/2640))
- rpcserver: Add median time to verbose results ([decred/dcrd#2638](https://github.com/decred/dcrd/pull/2638))
- rpcserver: Allow interface names for dial addresses ([decred/dcrd#2623](https://github.com/decred/dcrd/pull/2623))
- rpcserver: Add script version to gettxout ([decred/dcrd#2650](https://github.com/decred/dcrd/pull/2650))
- rpcserver: Remove unused help entry ([decred/dcrd#2648](https://github.com/decred/dcrd/pull/2648))
- rpcserver: Set script version in raw tx results ([decred/dcrd#2663](https://github.com/decred/dcrd/pull/2663))
- rpcserver: Impose additional read limits ([decred/dcrd#2675](https://github.com/decred/dcrd/pull/2675))
- rpcserver: Add more strict request origin check ([decred/dcrd#2676](https://github.com/decred/dcrd/pull/2676))
- rpcserver: Use duplicate tx error for recently mined transactions ([decred/dcrd#2705](https://github.com/decred/dcrd/pull/2705))
- rpcserver: Wait for sync on rpc request ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- rpcserver: Update websocket ping timeout handling ([decred/dcrd#2866](https://github.com/decred/dcrd/pull/2866))

### dcrd command-line flags and configuration:

- multi: Rename BMGR subsystem to SYNC ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- server/indexers: Remove v1 cfilter indexing support ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- config: Add utxocachemaxsize ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- main: Update slog for LOGFLAGS=nodatetime support ([decred/dcrd#2608](https://github.com/decred/dcrd/pull/2608))
- config: Allow interface names for listener addresses ([decred/dcrd#2623](https://github.com/decred/dcrd/pull/2623))
- config: Correct dir create failure error message ([decred/dcrd#2682](https://github.com/decred/dcrd/pull/2682))
- config: Add logsize config option ([decred/dcrd#2711](https://github.com/decred/dcrd/pull/2711))
- config: conditionally generate rpc credentials ([decred/dcrd#2779](https://github.com/decred/dcrd/pull/2779))
- multi: Add assumevalid config option ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))

### gencerts utility changes:

- gencerts: Add certificate authority capabilities ([decred/dcrd#2478](https://github.com/decred/dcrd/pull/2478))
- gencerts: Add RSA support (4096 bit keys only) ([decred/dcrd#2551](https://github.com/decred/dcrd/pull/2551))

### addblock utility changes:

- cmd/addblock: update block importer ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- addblock: Run index subscriber as a goroutine ([decred/dcrd#2760](https://github.com/decred/dcrd/pull/2760))
- addblock: Fix blockchain initialization ([decred/dcrd#2760](https://github.com/decred/dcrd/pull/2760))
- addblock: Use chain bulk import mode ([decred/dcrd#2782](https://github.com/decred/dcrd/pull/2782))

### findcheckpoint utility changes:

- findcheckpoint: Fix blockchain initialization ([decred/dcrd#2759](https://github.com/decred/dcrd/pull/2759))

### Documentation:

- docs: Fix JSON-RPC API gettxoutsetinfo description ([decred/dcrd#2443](https://github.com/decred/dcrd/pull/2443))
- docs: Add JSON-RPC API getpeerinfo missing fields ([decred/dcrd#2443](https://github.com/decred/dcrd/pull/2443))
- docs: Fix JSON-RPC API gettreasurybalance fmt ([decred/dcrd#2443](https://github.com/decred/dcrd/pull/2443))
- docs: Fix JSON-RPC API gettreasuryspendvotes fmt ([decred/dcrd#2443](https://github.com/decred/dcrd/pull/2443))
- docs: Add JSON-RPC API searchrawtxns req limit ([decred/dcrd#2443](https://github.com/decred/dcrd/pull/2443))
- docs: Update JSON-RPC API getrawmempool ([decred/dcrd#2453](https://github.com/decred/dcrd/pull/2453))
- progresslog: Add package documentation ([decred/dcrd#2499](https://github.com/decred/dcrd/pull/2499))
- netsync: Add package documentation ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- multi: update error code related documentation ([decred/dcrd#2515](https://github.com/decred/dcrd/pull/2515))
- docs: Update JSON-RPC API getwork to match reality ([decred/dcrd#2526](https://github.com/decred/dcrd/pull/2526))
- docs: Remove notifystakedifficulty JSON-RPC API ([decred/dcrd#2519](https://github.com/decred/dcrd/pull/2519))
- docs: Remove v1 getcfilter{,header} JSON-RPC API ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- chaincfg: Update doc.go ([decred/dcrd#2528](https://github.com/decred/dcrd/pull/2528))
- blockchain: Update README.md and doc.go ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- docs: Add invalidate/reconsiderblock JSON-RPC API ([decred/dcrd#2536](https://github.com/decred/dcrd/pull/2536))
- docs: Add release notes for v1.6.0 ([decred/dcrd#2451](https://github.com/decred/dcrd/pull/2451))
- multi: Update README.md files for go modules ([decred/dcrd#2559](https://github.com/decred/dcrd/pull/2559))
- apbf: Add README.md ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- docs: Add release notes for v1.6.1 ([decred/dcrd#2601](https://github.com/decred/dcrd/pull/2601))
- docs: Update min recommended specs in README.md ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- stdaddr: Add README.md ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add serialized pubkey info to README.md ([decred/dcrd#2619](https://github.com/decred/dcrd/pull/2619))
- docs: Add release notes for v1.6.2 ([decred/dcrd#2630](https://github.com/decred/dcrd/pull/2630))
- docs: Add scriptpubkey json returns ([decred/dcrd#2650](https://github.com/decred/dcrd/pull/2650))
- stdscript: Add README.md ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stake: Comment on max SSGen outputs with treasury ([decred/dcrd#2664](https://github.com/decred/dcrd/pull/2664))
- docs: Update JSON-RPC API for script version ([decred/dcrd#2663](https://github.com/decred/dcrd/pull/2663))
- docs: Update JSON-RPC API for max request limits ([decred/dcrd#2675](https://github.com/decred/dcrd/pull/2675))
- docs: Add SECURITY.md file ([decred/dcrd#2717](https://github.com/decred/dcrd/pull/2717))
- sampleconfig: Add missing log options ([decred/dcrd#2723](https://github.com/decred/dcrd/pull/2723))
- docs: Update go versions in README.md ([decred/dcrd#2722](https://github.com/decred/dcrd/pull/2722))
- docs: Correct generate description ([decred/dcrd#2724](https://github.com/decred/dcrd/pull/2724))
- database: Correct README rpcclient link ([decred/dcrd#2725](https://github.com/decred/dcrd/pull/2725))
- docs: Add accuracy and reliability to README.md ([decred/dcrd#2726](https://github.com/decred/dcrd/pull/2726))
- sampleconfig: Update for deprecated nodnsseed ([decred/dcrd#2728](https://github.com/decred/dcrd/pull/2728))
- docs: Update for secp256k1 v4 module ([decred/dcrd#2732](https://github.com/decred/dcrd/pull/2732))
- docs: Update for new modules ([decred/dcrd#2744](https://github.com/decred/dcrd/pull/2744))
- sampleconfig: update rpc credentials documentation ([decred/dcrd#2779](https://github.com/decred/dcrd/pull/2779))
- docs: Update for addrmgr v2 module ([decred/dcrd#2797](https://github.com/decred/dcrd/pull/2797))
- docs: Update for rpc/jsonrpc/types v3 module ([decred/dcrd#2801](https://github.com/decred/dcrd/pull/2801))
- stdscript: Update README.md for provably pruneable ([decred/dcrd#2803](https://github.com/decred/dcrd/pull/2803))
- docs: Update for txscript v3 module ([decred/dcrd#2815](https://github.com/decred/dcrd/pull/2815))
- docs: Update for dcrutil v4 module ([decred/dcrd#2818](https://github.com/decred/dcrd/pull/2818))
- uint256: Add README.md ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- docs: Update for peer v3 module ([decred/dcrd#2820](https://github.com/decred/dcrd/pull/2820))
- docs: Update for database v3 module ([decred/dcrd#2822](https://github.com/decred/dcrd/pull/2822))
- docs: Update for blockchain/stake v4 module ([decred/dcrd#2824](https://github.com/decred/dcrd/pull/2824))
- docs: Update for gcs v3 module ([decred/dcrd#2830](https://github.com/decred/dcrd/pull/2830))
- docs: Fix typos and trailing whitespace ([decred/dcrd#2843](https://github.com/decred/dcrd/pull/2843))
- docs: Add max line length and wrapping guidelines ([decred/dcrd#2843](https://github.com/decred/dcrd/pull/2843))
- docs: Update for math/uint256 module ([decred/dcrd#2842](https://github.com/decred/dcrd/pull/2842))
- docs: Update simnet env docs for subsidy split ([decred/dcrd#2848](https://github.com/decred/dcrd/pull/2848))
- docs: Update for blockchain v4 module ([decred/dcrd#2831](https://github.com/decred/dcrd/pull/2831))
- docs: Update for rpcclient v7 module ([decred/dcrd#2851](https://github.com/decred/dcrd/pull/2851))
- primitives: Add skeleton README.md ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))

### Contrib changes:

- contrib: Update OpenBSD rc script for 6.9 features ([decred/dcrd#2646](https://github.com/decred/dcrd/pull/2646))
- contrib: Bump Dockerfile.alpine to alpine:3.14.0 ([decred/dcrd#2681](https://github.com/decred/dcrd/pull/2681))
- build: Use go 1.17 in Dockerfiles ([decred/dcrd#2722](https://github.com/decred/dcrd/pull/2722))
- build: Pin docker images with SHA instead of tag ([decred/dcrd#2735](https://github.com/decred/dcrd/pull/2735))
- build/contrib: Improve docker support ([decred/dcrd#2740](https://github.com/decred/dcrd/pull/2740))

### Developer-related package and module changes:

- dcrjson: Reject dup method type registrations ([decred/dcrd#2417](https://github.com/decred/dcrd/pull/2417))
- peer: various cleanups ([decred/dcrd#2396](https://github.com/decred/dcrd/pull/2396))
- blockchain: Create treasury buckets during upgrade ([decred/dcrd#2441](https://github.com/decred/dcrd/pull/2441))
- blockchain: Fix stxosToScriptSource ([decred/dcrd#2444](https://github.com/decred/dcrd/pull/2444))
- rpcserver: add NtfnManager interface ([decred/dcrd#2410](https://github.com/decred/dcrd/pull/2410))
- lru: Fix lookup race on small caches ([decred/dcrd#2464](https://github.com/decred/dcrd/pull/2464))
- gcs: update error types ([decred/dcrd#2262](https://github.com/decred/dcrd/pull/2262))
- main: Switch windows service dependency ([decred/dcrd#2479](https://github.com/decred/dcrd/pull/2479))
- blockchain: Simplify upgrade single run stage code ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Simplify upgrade batching logic ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Use new batching logic for filter init ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Use new batch logic for blkidx upgrade ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Use new batch logic for utxos upgrade ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Use new batch logic for spends upgrade ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- blockchain: Use new batch logic for clr failed ([decred/dcrd#2457](https://github.com/decred/dcrd/pull/2457))
- windows: Switch to os.Executable ([decred/dcrd#2485](https://github.com/decred/dcrd/pull/2485))
- blockchain: Revert fast add reversal ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Less order dependent full blocks tests ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move context free tx sanity checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move context free block sanity checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Rework contextual tx checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move {coin,trsy}base contextual checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move staketx-related contextual checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move sigop-related contextual checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Make CheckBlockSanity context free ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Context free CheckTransactionSanity ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- blockchain: Move contextual treasury spend checks ([decred/dcrd#2481](https://github.com/decred/dcrd/pull/2481))
- mempool: Comment and stylistic updates ([decred/dcrd#2480](https://github.com/decred/dcrd/pull/2480))
- mining: Rename TxMiningView Remove method ([decred/dcrd#2490](https://github.com/decred/dcrd/pull/2490))
- mining: Unexport TxMiningView methods ([decred/dcrd#2490](https://github.com/decred/dcrd/pull/2490))
- mining: Update mergeUtxoView comment ([decred/dcrd#2490](https://github.com/decred/dcrd/pull/2490))
- blockchain: Consolidate deployment errors ([decred/dcrd#2487](https://github.com/decred/dcrd/pull/2487))
- blockchain: Consolidate unknown block errors ([decred/dcrd#2487](https://github.com/decred/dcrd/pull/2487))
- blockchain: Consolidate no filter errors ([decred/dcrd#2487](https://github.com/decred/dcrd/pull/2487))
- blockchain: Consolidate no treasury bal errors ([decred/dcrd#2487](https://github.com/decred/dcrd/pull/2487))
- blockchain: Convert to LRU block cache ([decred/dcrd#2488](https://github.com/decred/dcrd/pull/2488))
- blockchain: Remove unused error returns ([decred/dcrd#2489](https://github.com/decred/dcrd/pull/2489))
- blockmanager: Remove unused stakediff infra ([decred/dcrd#2493](https://github.com/decred/dcrd/pull/2493))
- server: Use next stake diff from snapshot ([decred/dcrd#2493](https://github.com/decred/dcrd/pull/2493))
- blockchain: Explicit hash in next stake diff calcs ([decred/dcrd#2494](https://github.com/decred/dcrd/pull/2494))
- blockchain: Explicit hash in LN agenda active func ([decred/dcrd#2495](https://github.com/decred/dcrd/pull/2495))
- blockmanager: Remove unused config field ([decred/dcrd#2497](https://github.com/decred/dcrd/pull/2497))
- blockmanager: Decouple block database code ([decred/dcrd#2497](https://github.com/decred/dcrd/pull/2497))
- blockmanager: Decouple from global config var ([decred/dcrd#2497](https://github.com/decred/dcrd/pull/2497))
- blockchain: Explicit hash in max block size func ([decred/dcrd#2507](https://github.com/decred/dcrd/pull/2507))
- progresslog: Make block progress log internal ([decred/dcrd#2499](https://github.com/decred/dcrd/pull/2499))
- server: Do not use unexported block manager cfg ([decred/dcrd#2498](https://github.com/decred/dcrd/pull/2498))
- blockmanager: Rework chain current logic ([decred/dcrd#2498](https://github.com/decred/dcrd/pull/2498))
- multi: Handle chain ntfn callback in server ([decred/dcrd#2498](https://github.com/decred/dcrd/pull/2498))
- server: Rename blockManager field to syncManager ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- server: Add temp sync manager interface ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- netsync: Split blockmanager into separate package ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- netsync: Rename blockManager to SyncManager ([decred/dcrd#2500](https://github.com/decred/dcrd/pull/2500))
- internal/ticketdb: update error types ([decred/dcrd#2279](https://github.com/decred/dcrd/pull/2279))
- secp256k1/ecdsa: update error types ([decred/dcrd#2281](https://github.com/decred/dcrd/pull/2281))
- secp256k1/schnorr: update error types ([decred/dcrd#2282](https://github.com/decred/dcrd/pull/2282))
- dcrjson: update error types ([decred/dcrd#2271](https://github.com/decred/dcrd/pull/2271))
- dcrec/secp256k1: update error types ([decred/dcrd#2265](https://github.com/decred/dcrd/pull/2265))
- blockchain/stake: update error types ([decred/dcrd#2264](https://github.com/decred/dcrd/pull/2264))
- multi: update database error types ([decred/dcrd#2261](https://github.com/decred/dcrd/pull/2261))
- blockchain: Remove unused treasury active func ([decred/dcrd#2514](https://github.com/decred/dcrd/pull/2514))
- stake: update ticket lottery errors ([decred/dcrd#2433](https://github.com/decred/dcrd/pull/2433))
- netsync: Improve is current detection ([decred/dcrd#2513](https://github.com/decred/dcrd/pull/2513))
- internal/mining: update mining error types ([decred/dcrd#2515](https://github.com/decred/dcrd/pull/2515))
- multi: sprinkle on more errors.As/Is ([decred/dcrd#2522](https://github.com/decred/dcrd/pull/2522))
- mining: Correct fee calculations during reorgs ([decred/dcrd#2530](https://github.com/decred/dcrd/pull/2530))
- fees: Remove deprecated DisableLog ([decred/dcrd#2529](https://github.com/decred/dcrd/pull/2529))
- rpcclient: Remove deprecated DisableLog ([decred/dcrd#2527](https://github.com/decred/dcrd/pull/2527))
- rpcclient: Remove notifystakedifficulty ([decred/dcrd#2519](https://github.com/decred/dcrd/pull/2519))
- rpc/jsonrpc/types: Remove notifystakedifficulty ([decred/dcrd#2519](https://github.com/decred/dcrd/pull/2519))
- netsync: Remove unneeded ForceReorganization ([decred/dcrd#2520](https://github.com/decred/dcrd/pull/2520))
- mining: Remove duplicate method ([decred/dcrd#2520](https://github.com/decred/dcrd/pull/2520))
- multi: use EstimateSmartFeeResult ([decred/dcrd#2283](https://github.com/decred/dcrd/pull/2283))
- rpcclient: Remove v1 getcfilter{,header} ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- rpc/jsonrpc/types: Remove v1 getcfilter{,header} ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- gcs: Remove unused v1 blockcf package ([decred/dcrd#2525](https://github.com/decred/dcrd/pull/2525))
- blockchain: Remove legacy sequence lock view ([decred/dcrd#2534](https://github.com/decred/dcrd/pull/2534))
- blockchain: Remove IsFixSeqLocksAgendaActive ([decred/dcrd#2534](https://github.com/decred/dcrd/pull/2534))
- blockchain: Explicit hash in estimate stake diff ([decred/dcrd#2524](https://github.com/decred/dcrd/pull/2524))
- netsync: Remove unneeded TipGeneration ([decred/dcrd#2537](https://github.com/decred/dcrd/pull/2537))
- netsync: Remove unused TicketPoolValue ([decred/dcrd#2544](https://github.com/decred/dcrd/pull/2544))
- netsync: Embed peers vs separate peer states ([decred/dcrd#2541](https://github.com/decred/dcrd/pull/2541))
- netsync/server: Update peer heights directly ([decred/dcrd#2542](https://github.com/decred/dcrd/pull/2542))
- netsync: Move proactive sigcache evict to server ([decred/dcrd#2543](https://github.com/decred/dcrd/pull/2543))
- blockchain: Add invalidate/reconsider infrastruct ([decred/dcrd#2536](https://github.com/decred/dcrd/pull/2536))
- rpc/jsonrpc/types: Add invalidate/reconsiderblock ([decred/dcrd#2536](https://github.com/decred/dcrd/pull/2536))
- netsync: Convert lifecycle to context ([decred/dcrd#2545](https://github.com/decred/dcrd/pull/2545))
- multi: Rework utxoset/view to use outpoints ([decred/dcrd#2540](https://github.com/decred/dcrd/pull/2540))
- blockchain: Remove compression version param ([decred/dcrd#2547](https://github.com/decred/dcrd/pull/2547))
- blockchain: Remove error from LatestBlockLocator ([decred/dcrd#2548](https://github.com/decred/dcrd/pull/2548))
- blockchain: Fix incorrect decompressScript calls ([decred/dcrd#2552](https://github.com/decred/dcrd/pull/2552))
- blockchain: Fix V3 spend journal migration ([decred/dcrd#2552](https://github.com/decred/dcrd/pull/2552))
- multi: Remove blockChain field from UtxoViewpoint ([decred/dcrd#2553](https://github.com/decred/dcrd/pull/2553))
- blockchain: Move UtxoEntry to a separate file ([decred/dcrd#2553](https://github.com/decred/dcrd/pull/2553))
- blockchain: Update UtxoEntry Clone method comment ([decred/dcrd#2553](https://github.com/decred/dcrd/pull/2553))
- progresslog: Make logger more generic ([decred/dcrd#2555](https://github.com/decred/dcrd/pull/2555))
- server: Remove several unused funcs ([decred/dcrd#2561](https://github.com/decred/dcrd/pull/2561))
- mempool: Store staged transactions as TxDesc ([decred/dcrd#2319](https://github.com/decred/dcrd/pull/2319))
- connmgr: Add func to iterate conn reqs ([decred/dcrd#2562](https://github.com/decred/dcrd/pull/2562))
- netsync: Correct check for needTx ([decred/dcrd#2568](https://github.com/decred/dcrd/pull/2568))
- rpcclient: Update EstimateSmartFee return type ([decred/dcrd#2255](https://github.com/decred/dcrd/pull/2255))
- server: Notify sync mgr later and track ntfn ([decred/dcrd#2582](https://github.com/decred/dcrd/pull/2582))
- apbf: Introduce Age-Partitioned Bloom Filters ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- apbf: Add basic usage example ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- apbf: Add support to go generate a KL table ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- apbf: Switch to fast reduce method ([decred/dcrd#2584](https://github.com/decred/dcrd/pull/2584))
- server: Remove unneeded child context ([decred/dcrd#2593](https://github.com/decred/dcrd/pull/2593))
- blockchain: Separate utxo state from tx flags ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Add utxoStateFresh to UtxoEntry ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Add size method to UtxoEntry ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Deep copy view entry script from tx ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Add utxoSetState to the database ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- multi: Add UtxoCache ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Make InitUtxoCache a UtxoCache method ([decred/dcrd#2599](https://github.com/decred/dcrd/pull/2599))
- blockchain: Add UtxoCacher interface ([decred/dcrd#2599](https://github.com/decred/dcrd/pull/2599))
- dcrutil: Correct ed25519 address constructor ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Introduce package infra for std addrs ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add infrastructure for v0 decoding ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pk-ecdsa-secp256k1 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pk-ed25519 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pk-schnorr-secp256k1 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pkh-ecdsa-secp256k1 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pkh-ed25519 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2pkh-schnorr-secp256k1 support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add v0 p2sh support ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- stdaddr: Add decode address example ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- txscript: Rename script bldr add data to unchecked ([decred/dcrd#2611](https://github.com/decred/dcrd/pull/2611))
- txscript: Add script bldr unchecked op add ([decred/dcrd#2611](https://github.com/decred/dcrd/pull/2611))
- rpcserver: Remove uncompressed pubkeys fast path ([decred/dcrd#2617](https://github.com/decred/dcrd/pull/2617))
- blockchain: Allow alternate tips for current check ([decred/dcrd#2612](https://github.com/decred/dcrd/pull/2612))
- txscript: Accept raw public keys in MultiSigScript ([decred/dcrd#2615](https://github.com/decred/dcrd/pull/2615))
- cpuminer: Remove unused MiningAddrs from Config ([decred/dcrd#2616](https://github.com/decred/dcrd/pull/2616))
- stdaddr: Add ability to obtain raw public key ([decred/dcrd#2619](https://github.com/decred/dcrd/pull/2619))
- stdaddr: Move from internal/staging to txscript ([decred/dcrd#2620](https://github.com/decred/dcrd/pull/2620))
- stdaddr: Accept vote and revoke limits separately ([decred/dcrd#2624](https://github.com/decred/dcrd/pull/2624))
- stake: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- indexers: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- blockchain: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- rpcclient: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- hdkeychain: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSStx ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSStxChange ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSGen ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSGenSHDirect ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSGenPKHDirect ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSRtx ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSRtxPKHDirect ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToSSRtxSHDirect ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToAddrScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused PayToScriptHashScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToSchnorrPubKeyScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToEdwardsPubKeyScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToPubKeyScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToScriptHashScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToPubKeyHashSchnorrScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToPubKeyHashEdwardsScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused payToPubKeyHashScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused GenerateSStxAddrPush ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Remove unused ErrUnsupportedAddress ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- txscript: Break dcrutil dependency ([decred/dcrd#2626](https://github.com/decred/dcrd/pull/2626))
- stdaddr: Replace Address method with String ([decred/dcrd#2633](https://github.com/decred/dcrd/pull/2633))
- dcrutil: Convert to use new stdaddr package ([decred/dcrd#2628](https://github.com/decred/dcrd/pull/2628))
- dcrutil: Remove all code related to Address ([decred/dcrd#2628](https://github.com/decred/dcrd/pull/2628))
- blockchain: Trsy always inactive for genesis blk ([decred/dcrd#2636](https://github.com/decred/dcrd/pull/2636))
- blockchain: Use agenda flags for tx check context ([decred/dcrd#2639](https://github.com/decred/dcrd/pull/2639))
- blockchain: Move UTXO DB methods to separate file ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- blockchain: Move UTXO DB tests to separate file ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- ipc: Fix lifetimeEvent comments ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- blockchain: Add utxoDatabaseInfo ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- multi: Introduce UTXO database ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- blockchain: Decouple stxo and utxo migrations ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- multi: Migrate to UTXO database ([decred/dcrd#2632](https://github.com/decred/dcrd/pull/2632))
- main: Handle SIGHUP with clean shutdown ([decred/dcrd#2645](https://github.com/decred/dcrd/pull/2645))
- txscript: Split signing code to sign subpackage ([decred/dcrd#2642](https://github.com/decred/dcrd/pull/2642))
- database: Add Flush to DB interface ([decred/dcrd#2649](https://github.com/decred/dcrd/pull/2649))
- multi: Flush block DB before UTXO DB ([decred/dcrd#2649](https://github.com/decred/dcrd/pull/2649))
- blockchain: Flush UTXO DB after init utxoSetState ([decred/dcrd#2649](https://github.com/decred/dcrd/pull/2649))
- blockchain: Force flush in separateUtxoDatabase ([decred/dcrd#2649](https://github.com/decred/dcrd/pull/2649))
- version: Rework to support single version override ([decred/dcrd#2651](https://github.com/decred/dcrd/pull/2651))
- blockchain: Remove UtxoCacher DB Tx dependency ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add UtxoBackend interface ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Export UtxoSetState ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add FetchEntry to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add PutUtxos to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add FetchState to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add FetchStats to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add FetchInfo to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Move LoadUtxoDB to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Add Upgrade to UtxoBackend ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- multi: Remove UTXO db in BlockChain and UtxoCache ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- blockchain: Export ViewFilteredSet ([decred/dcrd#2652](https://github.com/decred/dcrd/pull/2652))
- stake: Return StakeAddress from cmtmt conversion ([decred/dcrd#2655](https://github.com/decred/dcrd/pull/2655))
- stdscript: Introduce pkg infra for std scripts ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pk-ecdsa-secp256k1 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pk-ed25519 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pk-schnorr-secp256k1 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-ecdsa-secp256k1 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-ed25519 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-schnorr-secp256k1 support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 ecdsa multisig support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 ecdsa multisig redeem support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 nulldata support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake sub p2pkh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake sub p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake gen p2pkh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake gen p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake revoke p2pkh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake revoke p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake change p2pkh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake change p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury add support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury gen p2pkh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury gen p2sh support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add ecdsa multisig creation script ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 atomic swap redeem support ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add example for determining script type ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add example for p2pkh extract ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add example of script hash extract ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- blockchain: Use scripts in tickets address query ([decred/dcrd#2657](https://github.com/decred/dcrd/pull/2657))
- stake: Do not use standardness code in consensus ([decred/dcrd#2658](https://github.com/decred/dcrd/pull/2658))
- blockchain: Remove unneeded OP_TADD maturity check ([decred/dcrd#2659](https://github.com/decred/dcrd/pull/2659))
- stake: Add is treasury gen script ([decred/dcrd#2660](https://github.com/decred/dcrd/pull/2660))
- blockchain: No standardness code in consensus ([decred/dcrd#2661](https://github.com/decred/dcrd/pull/2661))
- gcs: No standardness code in consensus ([decred/dcrd#2662](https://github.com/decred/dcrd/pull/2662))
- stake: Remove stale TODOs from CheckSSGenVotes ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- stake: Remove stale TODOs from CheckSSRtx ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- txscript: Move contains stake opcode to consensus ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Move stake blockref script to consensus ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Move stake votebits script to consensus ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Remove unused IsPubKeyHashScript ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Remove unused IsStakeChangeScript ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Remove unused PushedData ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- blockchain: Flush UtxoCache when latch to current ([decred/dcrd#2671](https://github.com/decred/dcrd/pull/2671))
- dcrjson: Minor jsonerr.go update ([decred/dcrd#2672](https://github.com/decred/dcrd/pull/2672))
- rpcclient: Cancel client context on shutdown ([decred/dcrd#2678](https://github.com/decred/dcrd/pull/2678))
- blockchain: Remove serializeUtxoEntry error ([decred/dcrd#2683](https://github.com/decred/dcrd/pull/2683))
- blockchain: Add IsTreasuryEnabled to AgendaFlags ([decred/dcrd#2686](https://github.com/decred/dcrd/pull/2686))
- multi: Update block ntfns to contain AgendaFlags ([decred/dcrd#2686](https://github.com/decred/dcrd/pull/2686))
- multi: Update ProcessOrphans to use AgendaFlags ([decred/dcrd#2686](https://github.com/decred/dcrd/pull/2686))
- mempool: Add maybeAcceptTransaction AgendaFlags ([decred/dcrd#2686](https://github.com/decred/dcrd/pull/2686))
- secp256k1: Allow code generation to compile again ([decred/dcrd#2687](https://github.com/decred/dcrd/pull/2687))
- jsonrpc/types: Add missing Method type to vars ([decred/dcrd#2688](https://github.com/decred/dcrd/pull/2688))
- blockchain: Add UTXO backend error kinds ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Add helper to convert leveldb errors ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Add UtxoBackendIterator interface ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Add UtxoBackendTx interface ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Add levelDbUtxoBackendTx type ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- multi: Update UtxoBackend to use leveldb directly ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- multi: Move UTXO database ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Unexport levelDbUtxoBackend ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Always use node lookup methods ([decred/dcrd#2685](https://github.com/decred/dcrd/pull/2685))
- blockchain: Use short keys for block index ([decred/dcrd#2685](https://github.com/decred/dcrd/pull/2685))
- rpcclient: Shutdown breaks reconnect sleep ([decred/dcrd#2696](https://github.com/decred/dcrd/pull/2696))
- secp256k1: No deps on adaptor code for precomps ([decred/dcrd#2690](https://github.com/decred/dcrd/pull/2690))
- secp256k1: Always initialize adaptor instance ([decred/dcrd#2690](https://github.com/decred/dcrd/pull/2690))
- secp256k1: Optimize precomp values to use affine ([decred/dcrd#2690](https://github.com/decred/dcrd/pull/2690))
- rpcserver: Handle getwork nil err during reorg ([decred/dcrd#2700](https://github.com/decred/dcrd/pull/2700))
- secp256k1: Use blake256 directly in examples ([decred/dcrd#2697](https://github.com/decred/dcrd/pull/2697))
- secp256k1: Improve scalar mult readability ([decred/dcrd#2695](https://github.com/decred/dcrd/pull/2695))
- secp256k1: Optimize NAF conversion ([decred/dcrd#2695](https://github.com/decred/dcrd/pull/2695))
- blockchain: Verify state of DCP0007 voting ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- blockchain: Rename max expenditure funcs ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- stake: Add ExpiringNextBlock method to Node ([decred/dcrd#2701](https://github.com/decred/dcrd/pull/2701))
- rpcclient: Add GetNetworkInfo call ([decred/dcrd#2703](https://github.com/decred/dcrd/pull/2703))
- stake: Pre-allocate lottery ticket index slice ([decred/dcrd#2710](https://github.com/decred/dcrd/pull/2710))
- blockchain: Switch to treasuryValueType.IsDebit ([decred/dcrd#2680](https://github.com/decred/dcrd/pull/2680))
- blockchain: Sum amounts added to treasury ([decred/dcrd#2680](https://github.com/decred/dcrd/pull/2680))
- blockchain: Add maxTreasuryExpenditureDCP0007 ([decred/dcrd#2680](https://github.com/decred/dcrd/pull/2680))
- blockchain: Use new expenditure policy if activated ([decred/dcrd#2680](https://github.com/decred/dcrd/pull/2680))
- blockchain: Add checkTicketRedeemers ([decred/dcrd#2702](https://github.com/decred/dcrd/pull/2702))
- blockchain: Add NextExpiringTickets to BestState ([decred/dcrd#2708](https://github.com/decred/dcrd/pull/2708))
- multi: Add FetchUtxoEntry to mining Config ([decred/dcrd#2709](https://github.com/decred/dcrd/pull/2709))
- stake: Add func to create revocation from ticket ([decred/dcrd#2707](https://github.com/decred/dcrd/pull/2707))
- rpcserver: Use CreateRevocationFromTicket ([decred/dcrd#2707](https://github.com/decred/dcrd/pull/2707))
- multi: Don't use deprecated ioutil package ([decred/dcrd#2722](https://github.com/decred/dcrd/pull/2722))
- blockchain: Consolidate tx check flag construction ([decred/dcrd#2716](https://github.com/decred/dcrd/pull/2716))
- stake: Export Hash256PRNG UniformRandom ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- blockchain: Check auto revocations agenda state ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- multi: Add mempool IsAutoRevocationsAgendaActive ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- multi: Add auto revocations to agenda flags ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- multi: Check tx inputs auto revocations flag ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- blockchain: Add auto revocation error kinds ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- stake: Add auto revocation error kinds ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- blockchain: Move revocation checks block context ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- multi: Add isAutoRevocationsEnabled to CheckSSRtx ([decred/dcrd#2719](https://github.com/decred/dcrd/pull/2719))
- addrmgr: Remove deprecated code ([decred/dcrd#2729](https://github.com/decred/dcrd/pull/2729))
- peer: Remove deprecated DisableLog ([decred/dcrd#2730](https://github.com/decred/dcrd/pull/2730))
- database: Remove deprecated DisableLog ([decred/dcrd#2731](https://github.com/decred/dcrd/pull/2731))
- addrmgr: Decouple IP network checks from wire ([decred/dcrd#2596](https://github.com/decred/dcrd/pull/2596))
- addrmgr: Rename network address type ([decred/dcrd#2596](https://github.com/decred/dcrd/pull/2596))
- addrmgr: Decouple addrmgr from wire NetAddress ([decred/dcrd#2596](https://github.com/decred/dcrd/pull/2596))
- multi: add spend pruner ([decred/dcrd#2641](https://github.com/decred/dcrd/pull/2641))
- multi: synchronize spend prunes and notifications ([decred/dcrd#2641](https://github.com/decred/dcrd/pull/2641))
- blockchain: workSorterLess -> betterCandidate ([decred/dcrd#2747](https://github.com/decred/dcrd/pull/2747))
- mempool: Add HeaderByHash to Config ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- rpctest: Remove unused BlockVersion const ([decred/dcrd#2754](https://github.com/decred/dcrd/pull/2754))
- blockchain: Handle genesis auto revocation agenda ([decred/dcrd#2755](https://github.com/decred/dcrd/pull/2755))
- indexers: remove index manager ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- indexers: add index subscriber ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- indexers: refactor interfaces ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- indexers: async transaction index ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- indexers: update address index ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- indexers: async exists address index ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- multi: integrate index subscriber ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- multi: avoid using subscriber lifecycle in catchup ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- multi: remove spend deps on index disc. notifs ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- multi: copy snapshot pkScript ([decred/dcrd#2219](https://github.com/decred/dcrd/pull/2219))
- blockchain: Conditionally log difficulty retarget ([decred/dcrd#2761](https://github.com/decred/dcrd/pull/2761))
- multi: Use single latest checkpoint ([decred/dcrd#2763](https://github.com/decred/dcrd/pull/2763))
- blockchain: Move diff retarget log to connect ([decred/dcrd#2765](https://github.com/decred/dcrd/pull/2765))
- multi: source index notif. from block notif ([decred/dcrd#2256](https://github.com/decred/dcrd/pull/2256))
- server: fix wireToAddrmgrNetAddress data race ([decred/dcrd#2758](https://github.com/decred/dcrd/pull/2758))
- multi: Flush cache before fetching UTXO stats ([decred/dcrd#2767](https://github.com/decred/dcrd/pull/2767))
- blockchain: Don't use deprecated ioutil package ([decred/dcrd#2769](https://github.com/decred/dcrd/pull/2769))
- blockchain: Fix ticket db disconnect revocations ([decred/dcrd#2768](https://github.com/decred/dcrd/pull/2768))
- blockchain: Add convenience ancestor of func ([decred/dcrd#2771](https://github.com/decred/dcrd/pull/2771))
- blockchain: Use new ancestor of convenience func ([decred/dcrd#2771](https://github.com/decred/dcrd/pull/2771))
- blockchain: Remove unused latest blk locator func ([decred/dcrd#2772](https://github.com/decred/dcrd/pull/2772))
- blockchain: Remove unused next lottery data func ([decred/dcrd#2773](https://github.com/decred/dcrd/pull/2773))
- secp256k1: Correct 96-bit accum double overflow ([decred/dcrd#2778](https://github.com/decred/dcrd/pull/2778))
- blockchain: Further decouple upgrade code ([decred/dcrd#2776](https://github.com/decred/dcrd/pull/2776))
- blockchain: Add bulk import mode ([decred/dcrd#2782](https://github.com/decred/dcrd/pull/2782))
- multi: Remove flags from SyncManager ProcessBlock ([decred/dcrd#2783](https://github.com/decred/dcrd/pull/2783))
- netsync: Remove flags from processBlockMsg ([decred/dcrd#2783](https://github.com/decred/dcrd/pull/2783))
- multi: Remove flags from blockchain ProcessBlock ([decred/dcrd#2783](https://github.com/decred/dcrd/pull/2783))
- multi: Remove flags from ProcessBlockHeader ([decred/dcrd#2785](https://github.com/decred/dcrd/pull/2785))
- blockchain: Remove flags maybeAcceptBlockHeader ([decred/dcrd#2785](https://github.com/decred/dcrd/pull/2785))
- version: Use uint32 for major/minor/patch ([decred/dcrd#2789](https://github.com/decred/dcrd/pull/2789))
- wire: Write message header directly ([decred/dcrd#2790](https://github.com/decred/dcrd/pull/2790))
- stake: Correct treasury enabled vote discovery ([decred/dcrd#2780](https://github.com/decred/dcrd/pull/2780))
- blockchain: Correct treasury spend vote data ([decred/dcrd#2780](https://github.com/decred/dcrd/pull/2780))
- blockchain: UTXO database migration fix ([decred/dcrd#2798](https://github.com/decred/dcrd/pull/2798))
- blockchain: Handle zero-length UTXO backend state ([decred/dcrd#2798](https://github.com/decred/dcrd/pull/2798))
- mining: Remove unnecessary tx copy ([decred/dcrd#2792](https://github.com/decred/dcrd/pull/2792))
- multi: Use dcrutil Tx in NewTxDeepTxIns ([decred/dcrd#2802](https://github.com/decred/dcrd/pull/2802))
- indexers: synchronize index subscriber ntfn sends/receives ([decred/dcrd#2806](https://github.com/decred/dcrd/pull/2806))
- stdscript: Add exported MaxDataCarrierSizeV0 ([decred/dcrd#2803](https://github.com/decred/dcrd/pull/2803))
- stdscript: Add ProvablyPruneableScriptV0 ([decred/dcrd#2803](https://github.com/decred/dcrd/pull/2803))
- stdscript: Add num required sigs support ([decred/dcrd#2805](https://github.com/decred/dcrd/pull/2805))
- netsync: Remove unused RpcServer ([decred/dcrd#2811](https://github.com/decred/dcrd/pull/2811))
- netsync: Consolidate initial sync handling ([decred/dcrd#2812](https://github.com/decred/dcrd/pull/2812))
- stdscript: Add v0 p2pk-ed25519 extract ([decred/dcrd#2807](https://github.com/decred/dcrd/pull/2807))
- stdscript: Add v0 p2pk-schnorr-secp256k1 extract ([decred/dcrd#2807](https://github.com/decred/dcrd/pull/2807))
- stdscript: Add v0 p2pkh-ed25519 extract ([decred/dcrd#2807](https://github.com/decred/dcrd/pull/2807))
- stdscript: Add v0 p2pkh-schnorr-secp256k1 extract ([decred/dcrd#2807](https://github.com/decred/dcrd/pull/2807))
- stdscript: Add script to address conversion ([decred/dcrd#2807](https://github.com/decred/dcrd/pull/2807))
- stdscript: Move from internal/staging to txscript ([decred/dcrd#2810](https://github.com/decred/dcrd/pull/2810))
- mining: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- mempool: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- chaingen: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- blockchain: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- indexers: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- indexers: Remove unused trsy enabled params ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript/sign: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript/sign: Remove unused trsy enabled params ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- rpcserver: Convert to use stdscript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove deprecated ExtractAtomicSwapDataPushes ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused GenerateProvablyPruneableOut ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused MultiSigScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused MultisigRedeemScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused CalcMultiSigStats ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused IsMultisigScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused IsMultisigSigScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ExtractPkScriptAltSigType ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused GetScriptClass ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused GetStakeOutSubclass ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused typeOfScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isTreasurySpendScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isMultisigScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ExtractPkScriptAddrs ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused scriptHashToAddrs ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused pubKeyHashToAddrs ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isTreasuryAddScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractMultisigScriptDetails ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isStakeChangeScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isPubKeyHashScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isStakeRevocationScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isStakeGenScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isStakeSubmissionScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractStakeScriptHash ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractStakePubKeyHash ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isNullDataScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractPubKeyHash ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isPubKeyAltScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractPubKeyAltDetails ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isPubKeyScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractPubKey ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractUncompressedPubKey ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractCompressedPubKey ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isPubKeyHashAltScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused extractPubKeyHashAltDetails ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused isStandardAltSignatureType ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused MaxDataCarrierSize ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ScriptClass ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ErrNotMultisigScript ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ErrTooManyRequiredSigs ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- txscript: Remove unused ErrTooMuchNullData ([decred/dcrd#2808](https://github.com/decred/dcrd/pull/2808))
- stdaddr: Use txscript for opcode definitions ([decred/dcrd#2809](https://github.com/decred/dcrd/pull/2809))
- stdscript: Add v0 stake-tagged p2pkh extract ([decred/dcrd#2816](https://github.com/decred/dcrd/pull/2816))
- stdscript: Add v0 stake-tagged p2sh extract ([decred/dcrd#2816](https://github.com/decred/dcrd/pull/2816))
- server: sync rebroadcast inv sends/receives ([decred/dcrd#2814](https://github.com/decred/dcrd/pull/2814))
- multi: Move last ann block from peer to netsync ([decred/dcrd#2821](https://github.com/decred/dcrd/pull/2821))
- uint256: Introduce package infrastructure ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add set from big endian bytes ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add set from little endian bytes ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add get big endian bytes ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add get little endian bytes ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add zero support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add uint32 casting support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add uint64 casting support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add equality comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add less than comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add less or equals comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add greater than comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add greater or equals comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add general comparison support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add addition support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add subtraction support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add multiplication support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add squaring support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add division support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add negation support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add is odd support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise left shift support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise right shift support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise not support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise or support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise and support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise xor support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bit length support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add text formatting support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add conversion to stdlib big int support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add conversion from stdlib big int support ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add basic usage example ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- stake: Rename func to identify stake cmtmnt output ([decred/dcrd#2824](https://github.com/decred/dcrd/pull/2824))
- progresslog: Make header logging concurrent safe ([decred/dcrd#2833](https://github.com/decred/dcrd/pull/2833))
- netsync: Contiguous hashes for initial state reqs ([decred/dcrd#2825](https://github.com/decred/dcrd/pull/2825))
- multi: Allow discrete mining with invalidated tip ([decred/dcrd#2838](https://github.com/decred/dcrd/pull/2838))
- primitives: Add difficulty bits <-> uint256 ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add work calc from diff bits ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add hash to uint256 conversion ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add check proof of work ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add core merkle tree root calcs ([decred/dcrd#2826](https://github.com/decred/dcrd/pull/2826))
- primitives: Add inclusion proof funcs ([decred/dcrd#2827](https://github.com/decred/dcrd/pull/2827))
- indexers: update indexer error types ([decred/dcrd#2770](https://github.com/decred/dcrd/pull/2770))
- rpcserver: Submit transactions directly ([decred/dcrd#2835](https://github.com/decred/dcrd/pull/2835))
- netsync: Remove unused tx submission processing ([decred/dcrd#2835](https://github.com/decred/dcrd/pull/2835))
- internal/staging: add ban manager ([decred/dcrd#2554](https://github.com/decred/dcrd/pull/2554))
- uint256: Correct base 10 output formatting ([decred/dcrd#2844](https://github.com/decred/dcrd/pull/2844))
- multi: Add assumeValid to BlockChain ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- blockchain: Track assumed valid node ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- blockchain: Set BFFastAdd based on assume valid ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- blockchain: Assume valid skip script validation ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- blockchain: Bulk import skip script validation ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- hdkeychain: Add a strict BIP32 child derivation method ([decred/dcrd#2845](https://github.com/decred/dcrd/pull/2845))
- mempool: Consolidate tx check flag construction ([decred/dcrd#2846](https://github.com/decred/dcrd/pull/2846))
- standalone: Add modified subsidy split calcs ([decred/dcrd#2848](https://github.com/decred/dcrd/pull/2848))

### Developer-related module management:

- rpcclient: Prepare v6.0.1 ([decred/dcrd#2455](https://github.com/decred/dcrd/pull/2455))
- multi: Start blockchain v4 module dev cycle ([decred/dcrd#2463](https://github.com/decred/dcrd/pull/2463))
- multi: Start rpcclient v7 module dev cycle ([decred/dcrd#2463](https://github.com/decred/dcrd/pull/2463))
- multi: Start gcs v3 module dev cycle ([decred/dcrd#2463](https://github.com/decred/dcrd/pull/2463))
- multi: Start blockchain/stake v4 module dev cycle ([decred/dcrd#2511](https://github.com/decred/dcrd/pull/2511))
- multi: Start txscript v4 module dev cycle ([decred/dcrd#2511](https://github.com/decred/dcrd/pull/2511))
- multi: Start dcrutil v4 module dev cycle ([decred/dcrd#2511](https://github.com/decred/dcrd/pull/2511))
- multi: Start dcrec/secp256k1 v4 module dev cycle ([decred/dcrd#2511](https://github.com/decred/dcrd/pull/2511))
- rpc/jsonrpc/types: Start v3 module dev cycle ([decred/dcrd#2517](https://github.com/decred/dcrd/pull/2517))
- multi: Round 1 prerel module release ver updates ([decred/dcrd#2569](https://github.com/decred/dcrd/pull/2569))
- multi: Round 2 prerel module release ver updates ([decred/dcrd#2570](https://github.com/decred/dcrd/pull/2570))
- multi: Round 3 prerel module release ver updates ([decred/dcrd#2572](https://github.com/decred/dcrd/pull/2572))
- multi: Round 4 prerel module release ver updates ([decred/dcrd#2573](https://github.com/decred/dcrd/pull/2573))
- multi: Round 5 prerel module release ver updates ([decred/dcrd#2574](https://github.com/decred/dcrd/pull/2574))
- multi: Round 6 prerel module release ver updates ([decred/dcrd#2575](https://github.com/decred/dcrd/pull/2575))
- multi: Update to siphash v1.2.2 ([decred/dcrd#2577](https://github.com/decred/dcrd/pull/2577))
- peer: Start v3 module dev cycle ([decred/dcrd#2585](https://github.com/decred/dcrd/pull/2585))
- addrmgr: Start v2 module dev cycle ([decred/dcrd#2592](https://github.com/decred/dcrd/pull/2592))
- blockchain: Prerel module release ver updates ([decred/dcrd#2634](https://github.com/decred/dcrd/pull/2634))
- blockchain: Bump database module minor version ([decred/dcrd#2654](https://github.com/decred/dcrd/pull/2654))
- multi: Require last database/v2.0.3-x version ([decred/dcrd#2689](https://github.com/decred/dcrd/pull/2689))
- multi: Introduce database/v3 module ([decred/dcrd#2689](https://github.com/decred/dcrd/pull/2689))
- multi: Use database/v3 module ([decred/dcrd#2693](https://github.com/decred/dcrd/pull/2693))
- main: Use pseudo-versions in bumped mods ([decred/dcrd#2698](https://github.com/decred/dcrd/pull/2698))
- blockchain: Add replace to chaincfg dependency ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- dcrjson: Introduce v4 module ([decred/dcrd#2733](https://github.com/decred/dcrd/pull/2733))
- secp256k1: Prepare v4.0.0 ([decred/dcrd#2732](https://github.com/decred/dcrd/pull/2732))
- docs: Update for dcrjson v4 module ([decred/dcrd#2734](https://github.com/decred/dcrd/pull/2734))
- dcrjson: Prepare v4.0.0 ([decred/dcrd#2734](https://github.com/decred/dcrd/pull/2734))
- blockchain: Prerel module release ver updates ([decred/dcrd#2748](https://github.com/decred/dcrd/pull/2748))
- gcs: Prerel module release ver updates ([decred/dcrd#2749](https://github.com/decred/dcrd/pull/2749))
- multi: Update gcs prerel version ([decred/dcrd#2750](https://github.com/decred/dcrd/pull/2750))
- multi: update build tags to pref. go1.17 syntax ([decred/dcrd#2764](https://github.com/decred/dcrd/pull/2764))
- chaincfg: Prepare v3.1.0 ([decred/dcrd#2799](https://github.com/decred/dcrd/pull/2799))
- addrmgr: Prepare v2.0.0 ([decred/dcrd#2797](https://github.com/decred/dcrd/pull/2797))
- rpc/jsonrpc/types: Prepare v3.0.0 ([decred/dcrd#2801](https://github.com/decred/dcrd/pull/2801))
- txscript: Prepare v4.0.0 ([decred/dcrd#2815](https://github.com/decred/dcrd/pull/2815))
- hdkeychain: Prepare v3.0.1 ([decred/dcrd#2817](https://github.com/decred/dcrd/pull/2817))
- dcrutil: Prepare v4.0.0 ([decred/dcrd#2818](https://github.com/decred/dcrd/pull/2818))
- connmgr: Prepare v3.1.0 ([decred/dcrd#2819](https://github.com/decred/dcrd/pull/2819))
- peer: Prepare v3.0.0 ([decred/dcrd#2820](https://github.com/decred/dcrd/pull/2820))
- database: Prepare v3.0.0 ([decred/dcrd#2822](https://github.com/decred/dcrd/pull/2822))
- blockchain/stake: Prepare v4.0.0 ([decred/dcrd#2824](https://github.com/decred/dcrd/pull/2824))
- gcs: Prepare v3.0.0 ([decred/dcrd#2830](https://github.com/decred/dcrd/pull/2830))
- math/uint256: Prepare v1.0.0 ([decred/dcrd#2842](https://github.com/decred/dcrd/pull/2842))
- blockchain: Prepare v4.0.0 ([decred/dcrd#2831](https://github.com/decred/dcrd/pull/2831))
- rpcclient: Prepare v7.0.0 ([decred/dcrd#2851](https://github.com/decred/dcrd/pull/2851))
- version: Include VCS build info in version string ([decred/dcrd#2841](https://github.com/decred/dcrd/pull/2841))
- main: Update to use all new module versions ([decred/dcrd#2853](https://github.com/decred/dcrd/pull/2853))
- main: Remove module replacements ([decred/dcrd#2855](https://github.com/decred/dcrd/pull/2855))

### Testing and Quality Assurance:

- rpcserver: Add handleGetTreasuryBalance tests ([decred/dcrd#2390](https://github.com/decred/dcrd/pull/2390))
- rpcserver: Add handleGet{Generate,HashesPerSec} tests ([decred/dcrd#2365](https://github.com/decred/dcrd/pull/2365))
- mining: Cleanup txPriorityQueue tests ([decred/dcrd#2431](https://github.com/decred/dcrd/pull/2431))
- blockchain: fix errorlint warnings ([decred/dcrd#2411](https://github.com/decred/dcrd/pull/2411))
- rpcserver: Add handleGetHeaders test ([decred/dcrd#2366](https://github.com/decred/dcrd/pull/2366))
- rpcserver: add ticketsforaddress tests ([decred/dcrd#2405](https://github.com/decred/dcrd/pull/2405))
- rpcserver: add ticketvwap tests ([decred/dcrd#2406](https://github.com/decred/dcrd/pull/2406))
- rpcserver: add handleTxFeeInfo tests ([decred/dcrd#2407](https://github.com/decred/dcrd/pull/2407))
- rpcserver: add handleTicketFeeInfo tests ([decred/dcrd#2408](https://github.com/decred/dcrd/pull/2408))
- rpcserver: add handleVerifyMessage tests ([decred/dcrd#2413](https://github.com/decred/dcrd/pull/2413))
- rpcserver: add handleSendRawTransaction tests ([decred/dcrd#2410](https://github.com/decred/dcrd/pull/2410))
- rpcserver: add handleGetVoteInfo tests ([decred/dcrd#2432](https://github.com/decred/dcrd/pull/2432))
- database: Fix errorlint warnings ([decred/dcrd#2484](https://github.com/decred/dcrd/pull/2484))
- mining: Add mining test harness ([decred/dcrd#2480](https://github.com/decred/dcrd/pull/2480))
- mining: Add NewBlockTemplate tests ([decred/dcrd#2480](https://github.com/decred/dcrd/pull/2480))
- mining: Move TxMiningView tests to mining ([decred/dcrd#2480](https://github.com/decred/dcrd/pull/2480))
- rpcserver: add handleGetRawTransaction tests ([decred/dcrd#2483](https://github.com/decred/dcrd/pull/2483))
- blockchain: Improve synthetic treasury vote tests ([decred/dcrd#2488](https://github.com/decred/dcrd/pull/2488))
- rpcserver: Add handleGetMempoolInfo test ([decred/dcrd#2492](https://github.com/decred/dcrd/pull/2492))
- connmgr: Increase test timeouts ([decred/dcrd#2505](https://github.com/decred/dcrd/pull/2505))
- run_tests.sh: Avoid command substitution ([decred/dcrd#2506](https://github.com/decred/dcrd/pull/2506))
- mempool: Make sequence lock tests more consistent ([decred/dcrd#2496](https://github.com/decred/dcrd/pull/2496))
- mempool: Rework sequence lock acceptance tests ([decred/dcrd#2496](https://github.com/decred/dcrd/pull/2496))
- rpcserver: Add handleGetTxOut tests ([decred/dcrd#2516](https://github.com/decred/dcrd/pull/2516))
- rpcserver: Add handleGetNetworkHashPS test ([decred/dcrd#2512](https://github.com/decred/dcrd/pull/2512))
- rpcserver: Add handleGetMiningInfo test ([decred/dcrd#2512](https://github.com/decred/dcrd/pull/2512))
- blockchain: Simplify TestFixedSequenceLocks ([decred/dcrd#2534](https://github.com/decred/dcrd/pull/2534))
- chaingen: Support querying block test name by hash ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- blockchain: Improve test harness logging ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- blockchain: Support separate test block generation ([decred/dcrd#2518](https://github.com/decred/dcrd/pull/2518))
- rpcserver: add handleVersion, handleHelp rpc tests ([decred/dcrd#2549](https://github.com/decred/dcrd/pull/2549))
- blockchain: Use ReplaceVoteBits in utxoview tests ([decred/dcrd#2553](https://github.com/decred/dcrd/pull/2553))
- blockchain: Add unit test coverage for UtxoEntry ([decred/dcrd#2553](https://github.com/decred/dcrd/pull/2553))
- rpctest: Don't use installed node ([decred/dcrd#2523](https://github.com/decred/dcrd/pull/2523))
- apbf: Add comprehensive tests ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- apbf: Add benchmarks ([decred/dcrd#2579](https://github.com/decred/dcrd/pull/2579))
- rpcserver: Add handleGetRawMempool test ([decred/dcrd#2589](https://github.com/decred/dcrd/pull/2589))
- build: Test against go 1.16 ([decred/dcrd#2598](https://github.com/decred/dcrd/pull/2598))
- blockchain: Add test name to TestUtxoEntry errors ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Add UtxoCache test coverage ([decred/dcrd#2591](https://github.com/decred/dcrd/pull/2591))
- blockchain: Use new style for chainio test errors ([decred/dcrd#2595](https://github.com/decred/dcrd/pull/2595))
- rpcserver: Add handleInvalidateBlock test ([decred/dcrd#2604](https://github.com/decred/dcrd/pull/2604))
- blockchain: Mock time.Now for utxo cache tests ([decred/dcrd#2605](https://github.com/decred/dcrd/pull/2605))
- blockchain: Add UtxoCache Initialize tests ([decred/dcrd#2599](https://github.com/decred/dcrd/pull/2599))
- blockchain: Add TestShutdownUtxoCache tests ([decred/dcrd#2599](https://github.com/decred/dcrd/pull/2599))
- rpcserver: Add handleReconsiderBlock test ([decred/dcrd#2613](https://github.com/decred/dcrd/pull/2613))
- stdaddr: Add benchmarks ([decred/dcrd#2610](https://github.com/decred/dcrd/pull/2610))
- rpctest: Make tests work properly with latest code ([decred/dcrd#2614](https://github.com/decred/dcrd/pull/2614))
- mempool: Remove unused field from test struct ([decred/dcrd#2618](https://github.com/decred/dcrd/pull/2618))
- mempool: Remove unused func from tests ([decred/dcrd#2621](https://github.com/decred/dcrd/pull/2621))
- rpctest: Don't use Fatalf in goroutines ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- chaingen: Remove unused PurchaseCommitmentScript ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- rpctest: Convert to use new stdaddr package ([decred/dcrd#2625](https://github.com/decred/dcrd/pull/2625))
- dcrutil: Move address params iface and mock impls ([decred/dcrd#2628](https://github.com/decred/dcrd/pull/2628))
- stdscript: Add v0 p2pk-ecdsa-secp256k1 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pk-ed25519 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pk-schnorr-secp256k1 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-ecdsa-secp256k1 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-ed25519 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2pkh-schnorr-secp256k1 benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 ecdsa multisig benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 ecdsa multisig redeem benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add extract v0 multisig redeem benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 nulldata benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake sub p2pkh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake sub p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake gen p2pkh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake gen p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake revoke p2pkh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake revoke p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake change p2pkh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 stake change p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury add benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury gen p2pkh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 treasury gen p2sh benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add determine script type benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- stdscript: Add v0 atomic swap redeem benchmark ([decred/dcrd#2656](https://github.com/decred/dcrd/pull/2656))
- txscript: Separate short form script parsing ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Explicit consensus p2sh tests ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- txscript: Explicit consensus any kind p2sh tests ([decred/dcrd#2666](https://github.com/decred/dcrd/pull/2666))
- stake: No standardness code in tests ([decred/dcrd#2667](https://github.com/decred/dcrd/pull/2667))
- blockchain: Add outpointKey tests ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- blockchain: Add block index key collision tests ([decred/dcrd#2685](https://github.com/decred/dcrd/pull/2685))
- secp256k1: Rework NAF tests ([decred/dcrd#2695](https://github.com/decred/dcrd/pull/2695))
- secp256k1: Cleanup NAF benchmark ([decred/dcrd#2695](https://github.com/decred/dcrd/pull/2695))
- rpctest: Add P2PAddress() function ([decred/dcrd#2704](https://github.com/decred/dcrd/pull/2704))
- tests: Remove hardcoded CC=gcc from run_tests.sh ([decred/dcrd#2706](https://github.com/decred/dcrd/pull/2706))
- build: Test against Go 1.17 ([decred/dcrd#2712](https://github.com/decred/dcrd/pull/2712))
- blockchain: Support voting multiple agendas in test ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- blockchain: Single out treasury policy test ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- blockchain: Correct test harness err msg ([decred/dcrd#2714](https://github.com/decred/dcrd/pull/2714))
- blockchain: Test new max expenditure policy ([decred/dcrd#2680](https://github.com/decred/dcrd/pull/2680))
- chaingen: Add spendable coinbase out snapshots ([decred/dcrd#2715](https://github.com/decred/dcrd/pull/2715))
- mempool: Accept test mungers for create tickets ([decred/dcrd#2721](https://github.com/decred/dcrd/pull/2721))
- build: Don't set GO111MODULE unnecessarily ([decred/dcrd#2722](https://github.com/decred/dcrd/pull/2722))
- build: Don't manually test changing go.{mod,sum} ([decred/dcrd#2722](https://github.com/decred/dcrd/pull/2722))
- stake: Add CalculateRewards tests ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- stake: Add CheckSSRtx tests ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- blockchain: Test auto revocations deployment ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- chaingen: Add revocation mungers ([decred/dcrd#2718](https://github.com/decred/dcrd/pull/2718))
- addrmgr: Improve test coverage ([decred/dcrd#2596](https://github.com/decred/dcrd/pull/2596))
- addrmgr: Remove unnecessary test cases ([decred/dcrd#2596](https://github.com/decred/dcrd/pull/2596))
- rpcserver: Tune large tspend test amount ([decred/dcrd#2679](https://github.com/decred/dcrd/pull/2679))
- build: Pin GitHub Actions to SHA ([decred/dcrd#2736](https://github.com/decred/dcrd/pull/2736))
- blockchain: Add calcTicketReturnAmounts tests ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- blockchain: Add checkTicketRedeemers tests ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- blockchain: Add auto revocation validation tests ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- mining: Add auto revocation block template tests ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- mempool: Add tests with auto revocations enabled ([decred/dcrd#2720](https://github.com/decred/dcrd/pull/2720))
- txscript: Add versioned short form parsing ([decred/dcrd#2756](https://github.com/decred/dcrd/pull/2756))
- txscript: Test consistency and cleanup ([decred/dcrd#2757](https://github.com/decred/dcrd/pull/2757))
- mempool: Add blockHeight to AddFakeUTXO for tests ([decred/dcrd#2804](https://github.com/decred/dcrd/pull/2804))
- mempool: Test fraud proof handling ([decred/dcrd#2804](https://github.com/decred/dcrd/pull/2804))
- stdscript: Add extract v0 stake-tagged p2pkh bench ([decred/dcrd#2816](https://github.com/decred/dcrd/pull/2816))
- stdscript: Add extract v0 stake-tagged p2sh bench ([decred/dcrd#2816](https://github.com/decred/dcrd/pull/2816))
- mempool: Update test to check hash value ([decred/dcrd#2804](https://github.com/decred/dcrd/pull/2804))
- stdscript: Add num required sigs benchmark ([decred/dcrd#2805](https://github.com/decred/dcrd/pull/2805))
- uint256: Add big endian set benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add little endian set benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add big endian get benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add little endian get benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add zero benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add equality comparison benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add less than comparison benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add greater than comparison benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add general comparison benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add addition benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add subtraction benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add multiplication benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add squaring benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add division benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add negation benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add is odd benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise left shift benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise right shift benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise not benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise or benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise and benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bitwise xor benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add bit length benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add text formatting benchmarks ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add conversion to stdlib big int benchmark ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- uint256: Add conversion from stdlib big int benchmark ([decred/dcrd#2787](https://github.com/decred/dcrd/pull/2787))
- primitives: Add diff bits conversion benchmarks ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add work calc benchmark ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add hash to uint256 benchmark ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add check proof of work benchmark ([decred/dcrd#2788](https://github.com/decred/dcrd/pull/2788))
- primitives: Add merkle root benchmarks ([decred/dcrd#2826](https://github.com/decred/dcrd/pull/2826))
- primitives: Add inclusion proof benchmarks ([decred/dcrd#2827](https://github.com/decred/dcrd/pull/2827))
- blockchain: Add AssumeValid tests ([decred/dcrd#2839](https://github.com/decred/dcrd/pull/2839))
- chaingen: Add vote subsidy munger ([decred/dcrd#2848](https://github.com/decred/dcrd/pull/2848))

### Misc:

- release: Bump for 1.7 release cycle ([decred/dcrd#2429](https://github.com/decred/dcrd/pull/2429))
- secp256k1: Correct const name for doc comment ([decred/dcrd#2445](https://github.com/decred/dcrd/pull/2445))
- multi: Fix various typos ([decred/dcrd#2607](https://github.com/decred/dcrd/pull/2607))
- rpcserver: Fix createrawssrtx comments ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- blockchain: Fix comment formatting in generator ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- stake: Fix MaxOutputsPerSSRtx comment ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- stake: Fix CheckSSGenVotes function comment ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- stake: Fix CheckSSRtx function comment ([decred/dcrd#2665](https://github.com/decred/dcrd/pull/2665))
- database: Add comment on os.MkdirAll behavior ([decred/dcrd#2670](https://github.com/decred/dcrd/pull/2670))
- multi: Address some linter complaints ([decred/dcrd#2684](https://github.com/decred/dcrd/pull/2684))
- txscript: Fix a couple of a comment typos ([decred/dcrd#2692](https://github.com/decred/dcrd/pull/2692))
- blockchain: Remove inapplicable comment ([decred/dcrd#2742](https://github.com/decred/dcrd/pull/2742))
- mining: Fix error in comment ([decred/dcrd#2743](https://github.com/decred/dcrd/pull/2743))
- blockchain: Fix several typos ([decred/dcrd#2745](https://github.com/decred/dcrd/pull/2745))
- blockchain: Update a few BFFastAdd comments ([decred/dcrd#2781](https://github.com/decred/dcrd/pull/2781))
- multi: Address some linter complaints ([decred/dcrd#2791](https://github.com/decred/dcrd/pull/2791))
- netsync: Correct typo ([decred/dcrd#2813](https://github.com/decred/dcrd/pull/2813))
- netsync: Fix misc typos ([decred/dcrd#2834](https://github.com/decred/dcrd/pull/2834))
- mining: Fix typo ([decred/dcrd#2834](https://github.com/decred/dcrd/pull/2834))
- blockchain: Correct comment typos for find fork ([decred/dcrd#2828](https://github.com/decred/dcrd/pull/2828))
- rpcserver: Rename var to make linter happy ([decred/dcrd#2835](https://github.com/decred/dcrd/pull/2835))
- blockchain: Wrap at max line length ([decred/dcrd#2843](https://github.com/decred/dcrd/pull/2843))
- release: Bump for 1.7.0 ([decred/dcrd#2856](https://github.com/decred/dcrd/pull/2856))

### Code Contributors (alphabetical order):

- briancolecoinmetrics
- Dave Collins
- David Hill
- degeri
- Donald Adu-Poku
- J Fixby
- Jamie Holdstock
- Joe Gruffins
- Jonathan Chappelow
- Josh Rickmar
- lolandhold
- Matheus Degiovani
- Naveen
- Ryan Staudt
- Youssef Boukenken
- Wisdom Arerosuoghene
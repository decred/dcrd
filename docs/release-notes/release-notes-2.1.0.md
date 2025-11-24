# dcrd v2.1.0 Release Notes

This is a new major release of dcrd.  Some of the key highlights are:

* A new consensus vote agenda which allows the stakeholders to decide whether or
  not to activate a new decentralized treasury maximum expenditure policy
* Exclusion timeouts for disruptive StakeShuffle mixing participants
* Less bandwidth usage for StakeShuffle mixing messages
* Configurable maximum overall memory usage limit
* Higher network throughput
* Faster address decoding
* Hardware optimizations for hashes
* Various updates to the RPC server
  * IPv6 zone support
  * New JSON-RPC API additions for dynamic profiling server control
  * Discrete mining cancellation support
* Infrastructure improvements
* Miscellaneous network and protocol optimizations
* Quality assurance changes

For those unfamiliar with the
[voting process](https://docs.decred.org/governance/consensus-rule-voting/overview/)
in Decred, all code needed in order to support each of the aforementioned
consensus changes is already included in this release, however it will remain
dormant until the stakeholders vote to activate it.

For reference, the consensus change work was originally proposed and approved
for initial implementation via the following Politeia proposal:
- [Treasury Expenditure Policy Consensus Change](https://proposals.decred.org/record/16a93c7751722954)

The following Decred Change Proposal (DCP) describes the proposed changes in
detail and provides full technical specifications:
* [DCP0013: New Max Treasury Expenditure Policy](https://github.com/decred/dcps/blob/master/dcp-0013/dcp-0013.mediawiki)

## Upgrade Required

**It is extremely important for everyone to upgrade their software to this
latest release even if you don't intend to vote in favor of the agenda.  This
particularly applies to PoW miners as failure to upgrade will result in lost
rewards after block height 1035288.  That is estimated to be around December
19th, 2025.**

## Notable Changes

### New Treasury Maximum Expenditure Policy Vote

A consensus change vote with the id `maxtreasuryspend` is now available as of
this release.  After upgrading, stakeholders may set their preferences through
their wallet.

The primary goals of this change are to:

* Provide a maximum expenditure policy that is stable for the long term while
  retaining the primary motivations for having a maximum expenditure policy
* Serve as a preemptive measure to avoid the current overly-restrictive spending
  limits from potentially resulting in issues with funding ongoing operations

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/16a93c7751722954)
* [DCP0013](https://github.com/decred/dcps/blob/master/dcp-0013/dcp-0013.mediawiki)

### Exclusion Timeouts for Disruptive StakeShuffle Mixing Participants

Coins participating in decentralized StakeShuffle mixing that are excluded from
ongoing mixes due to failing to complete the entire protocol too many times are
now prohibited from participating in any new mixes for a certain period of time.

The exclusion timeout is a matter of policy that may be refined in the future.
It is currently set to 24 hours.

This change improves the overall robustness of the system and helps ensure
smooth operation for all honest participants in the event of misbehavior,
intentional or otherwise.

### Less Bandwidth Usage for StakeShuffle Mixing Messages

This release includes various changes to reduce the amount of bandwidth required
for decentralized StakeShuffle mixing by approximately 15%.

Aside from generally improving efficiency, it provides the key benefit of
lowering the overall resource requirements for full nodes running in
bandwidth-constrained environments.

### Configurable Maximum Overall Memory Usage Limit

The server automatically calculates a default target maximum memory limit that
is sufficient for almost all users and use cases.

System administrators that are comfortable trading off more memory for
performance may now optionally set the `GOMEMLIMIT` environment variable prior
to starting drcd in order to increase the target maximum memory limit.

The target limit is logged when the server starts.  For example:

> Soft memory limit: 1.80 GiB

Note that the value may not be configured to be less than the default calculated
value and is potentially influenced by the cache-specific CLI options
`--sigcachemaxsize` and `--utxocachemaxsize`.

### Higher Network Throughput

The primary network syncing infrastructure now uses a model that makes better
use of multiple processor cores provided by all modern hardware.  The net result
is better overall throughput that scales equally well across the entire spectrum
of low-end to high-end computing hardware.

### Faster Address Decoding

Decoding of Decred payment addresses is now about two times faster on average.

Larger wallets connecting to dcrd via RPC will notice significantly faster load
times and mobile wallets using the new code will benefit from less battery
usage.

### Hardware Optimizations for Hashes

`BLAKE-256` hashing is now approximately 70% faster on any modern amd64
hardware.

The primary benefits are:

* Notably reduced overall CPU usage for the same amount of work
* Performance enhancements to nearly all areas such as:
  * The initial chain sync process
  * Transaction and block validation
  * Mixing message validation
  * Signature verification
  * Key generation
  * Lightweight client sync time
  * Stakeholder voting lottery selection
  * Internal caches
  * Payment addresses

### RPC Server Changes

The RPC server version as of this release is 8.3.0

#### IPv6 Zone Support

The RPC server may now be configured to listen on IPv6 addresses with a zone.

This is particularly useful for specifying which scope or interface to bind the
listener to in systems with multiple interfaces.  For example, link local IPv6
addresses typically require specifying the zone to avoid routing issues since
they are otherwise ambiguous on systems with multiple interfaces.

#### New Dynamic Profiling Server Control RPCs (`startprofiler` and `stopprofiler`)

Two new RPCs named `startprofiler` and `stopprofiler` are now available.  These
RPCs can be used to dynamically start and stop the built-in profiling server.

The profiling server is primarily useful for developers working on the software.

See the following for API details:

* [startprofiler JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#startprofiler)
* [stopprofilers JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#stopprofiler)

#### Discrete Mining Cancellation Support (`generate`)

The `generate` RPC used in test environments such as the test and simulation
networks to mine a discrete number of blocks now accepts a parameter of 0 to
cancel a running active discrete mining instance.

See the following for API details:

* [generate JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#generate)

## Changelog

This release consists of 355 commits from 8 contributors which total to 294
files changed, 25,567 additional lines of code, and 10,534 deleted lines of
code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.0.6...release-v2.1.0).

### Protocol and network:

- netsync: Request new headers on rejected tip ([decred/dcrd#3302](https://github.com/decred/dcrd/pull/3302))
- server: Support listening on zoned IPv6 link local ([decred/dcrd#3322](https://github.com/decred/dcrd/pull/3322))
- server: Prefer min one mix capable outbound peer ([decred/dcrd#3336](https://github.com/decred/dcrd/pull/3336))
- peer: Randomize inv batching delays ([decred/dcrd#3363](https://github.com/decred/dcrd/pull/3363))
- server: Cache advertised txns ([decred/dcrd#3362](https://github.com/decred/dcrd/pull/3362))
- server: Prefer 3 min mix capable outbound peers ([decred/dcrd#3389](https://github.com/decred/dcrd/pull/3389))
- peer: Add factored poly to stall detector getdata responses ([decred/dcrd#3446](https://github.com/decred/dcrd/pull/3446))
- netsync: Always request initial non-mining state ([decred/dcrd#3476](https://github.com/decred/dcrd/pull/3476))
- netsync: Don't request recently removed mix msgs ([decred/dcrd#3494](https://github.com/decred/dcrd/pull/3494))
- chaincfg: Remove dcrdata.org seeders ([decred/dcrd#3495](https://github.com/decred/dcrd/pull/3495))
- peer: Do not send inventory before version ([decred/dcrd#3479](https://github.com/decred/dcrd/pull/3479))
- chaincfg: Introduce max treasury spend agenda ([decred/dcrd#3548](https://github.com/decred/dcrd/pull/3548))
- multi: Implement DCP0013 max treasury spend vote ([decred/dcrd#3549](https://github.com/decred/dcrd/pull/3549))
- chaincfg: Update assume valid for release ([decred/dcrd#3557](https://github.com/decred/dcrd/pull/3557))
- chaincfg: Update min known chain work for release ([decred/dcrd#3558](https://github.com/decred/dcrd/pull/3558))
- server: Force PoW upgrade to v11 ([decred/dcrd#3570](https://github.com/decred/dcrd/pull/3570))

### Mixing message relay (mix pool):

- netsync: Handle notfound mix messages ([decred/dcrd#3325](https://github.com/decred/dcrd/pull/3325))
- netsync: Remove spent PRs from prev block txs ([decred/dcrd#3334](https://github.com/decred/dcrd/pull/3334))
- mixpool: Only ban full nodes for bad UTXO sigs ([decred/dcrd#3345](https://github.com/decred/dcrd/pull/3345))
- mixing: Create new sessions over incrementing runs ([decred/dcrd#3343](https://github.com/decred/dcrd/pull/3343))
- mixpool: Cache recently removed msgs ([decred/dcrd#3366](https://github.com/decred/dcrd/pull/3366))
- netsync: Remove spent PRs from tip block txns ([decred/dcrd#3367](https://github.com/decred/dcrd/pull/3367))
- mixclient: Introduce random message jitter ([decred/dcrd#3388](https://github.com/decred/dcrd/pull/3388))
- mixpool: Reject KEs submitted too early ([decred/dcrd#3403](https://github.com/decred/dcrd/pull/3403))
- mixclient: Use newest (fewest-PR) KEs to form alt sessions ([decred/dcrd#3404](https://github.com/decred/dcrd/pull/3404))
- mixpool: Do not return early for revealed secrets ([decred/dcrd#3454](https://github.com/decred/dcrd/pull/3454))
- mixpool: Add malicious peer greylisting ([decred/dcrd#3554](https://github.com/decred/dcrd/pull/3554))

### Transaction relay (memory pool):

- dcrd: Make DisableRelayTx also include mixing messages ([decred/dcrd#3304](https://github.com/decred/dcrd/pull/3304))

### Mining:

- cpuminer: Support discrete miner generate cancel ([decred/dcrd#3508](https://github.com/decred/dcrd/pull/3508))
- cpuminer: Prioritize num blocks in discrete mining ([decred/dcrd#3509](https://github.com/decred/dcrd/pull/3509))
- cpuminer: Terminate discrete mining via blk ntfns ([decred/dcrd#3510](https://github.com/decred/dcrd/pull/3510))
- cpuminer: Terminate discrete mining via blk ntfns ([decred/dcrd#3510](https://github.com/decred/dcrd/pull/3510))

### RPC:

- dcrjson,rpcserver,types: Add getmixmessage ([decred/dcrd#3307](https://github.com/decred/dcrd/pull/3307))
- rpcserver: Allow getmixmessage from limited users ([decred/dcrd#3310](https://github.com/decred/dcrd/pull/3310))
- server: Support listening on zoned IPv6 link local ([decred/dcrd#3322](https://github.com/decred/dcrd/pull/3322))
- server: Add dynamic profile server infrastructure ([decred/dcrd#3323](https://github.com/decred/dcrd/pull/3323))
- rpcserver: Add {start,stop}profiler support ([decred/dcrd#3323](https://github.com/decred/dcrd/pull/3323))
- rpcserver: Remove wallet RPC stakepooluserinfo ([decred/dcrd#3439](https://github.com/decred/dcrd/pull/3439))
- rpcserver: Remove wallet RPC ticketsforaddress ([decred/dcrd#3439](https://github.com/decred/dcrd/pull/3439))
- main,gencerts: Punycode non-ASCII hostnames ([decred/dcrd#3427](https://github.com/decred/dcrd/pull/3427))
- rpcserver: Discrete mining cancel via generate 0 ([decred/dcrd#3508](https://github.com/decred/dcrd/pull/3508))

### gencerts utility changes:

- main,gencerts: Punycode non-ASCII hostnames ([decred/dcrd#3427](https://github.com/decred/dcrd/pull/3427))
- gencerts: Explicit KeyUsage for leaf signing ([decred/dcrd#3551](https://github.com/decred/dcrd/pull/3551))
- gencerts: Uncapitalize -S description ([decred/dcrd#3555](https://github.com/decred/dcrd/pull/3555))

### dcrd command-line flags and configuration:

- config: Explicitly handle help requests ([decred/dcrd#3292](https://github.com/decred/dcrd/pull/3292))

### dcrd server runtime changes:

- main: Increase runtime GC memlimit params ([decred/dcrd#3485](https://github.com/decred/dcrd/pull/3485))
- main: Respect GOMEMLIMIT environment variable ([decred/dcrd#3486](https://github.com/decred/dcrd/pull/3486))

### Decentralized Treasury:

- blockchain: Allow trsy spends of maturing funds ([decred/dcrd#3520](https://github.com/decred/dcrd/pull/3520))
- multi: Implement DCP0013 max treasury spend vote ([decred/dcrd#3549](https://github.com/decred/dcrd/pull/3549))

### Documentation:

- docs: Fix release notes template typos ([decred/dcrd#3294](https://github.com/decred/dcrd/pull/3294))
- docs: Add release notes for v2.0.0 ([decred/dcrd#3293](https://github.com/decred/dcrd/pull/3293))
- docs: Add getmixmessage ([decred/dcrd#3311](https://github.com/decred/dcrd/pull/3311))
- docs: Add release notes for v2.0.1 ([decred/dcrd#3320](https://github.com/decred/dcrd/pull/3320))
- docs: Add start/stopprofiler JSON-RPC API ([decred/dcrd#3323](https://github.com/decred/dcrd/pull/3323))
- docs: Add release notes for v2.0.2 ([decred/dcrd#3354](https://github.com/decred/dcrd/pull/3354))
- docs: Update for container/lru module ([decred/dcrd#3359](https://github.com/decred/dcrd/pull/3359))
- docs: Update for crypto/rand module ([decred/dcrd#3372](https://github.com/decred/dcrd/pull/3372))
- rand: Add README.md ([decred/dcrd#3379](https://github.com/decred/dcrd/pull/3379))
- docs: Add release notes for v2.0.3 ([decred/dcrd#3394](https://github.com/decred/dcrd/pull/3394))
- blake256: Add main README.md ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add internal/compress README.md ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add basic usage example ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add rolling hasher usage example ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add same process save/restore example ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add _asm note about AVX2 attempts ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add internal _asm README.md ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- docs: Update for mixing module ([decred/dcrd#3422](https://github.com/decred/dcrd/pull/3422))
- docs: Update README.md to required Go 1.22/1.23 ([decred/dcrd#3425](https://github.com/decred/dcrd/pull/3425))
- docs: Add release notes for v2.0.4 ([decred/dcrd#3434](https://github.com/decred/dcrd/pull/3434))
- docs: Correct getwork JSON-RPC API description ([decred/dcrd#3450](https://github.com/decred/dcrd/pull/3450))
- docs: Add release notes for v2.0.5 ([decred/dcrd#3460](https://github.com/decred/dcrd/pull/3460))
- docs: Update README.md to required Go 1.23/1.24 ([decred/dcrd#3470](https://github.com/decred/dcrd/pull/3470))
- docs: Add release notes for v2.0.6 ([decred/dcrd#3475](https://github.com/decred/dcrd/pull/3475))
- netsync: Update docs for initial chain sync ([decred/dcrd#3490](https://github.com/decred/dcrd/pull/3490))
- rpcserver: Update getblockchaininfo sync help ([decred/dcrd#3490](https://github.com/decred/dcrd/pull/3490))
- docs: Update getblockchaininfo JSON-RPC API desc ([decred/dcrd#3490](https://github.com/decred/dcrd/pull/3490))
- docs: Update README.md to required Go 1.24/1.25 ([decred/dcrd#3502](https://github.com/decred/dcrd/pull/3502))
- docs: Update generate JSON-RPC API ([decred/dcrd#3508](https://github.com/decred/dcrd/pull/3508))
- docs: Update generate JSON-RPC API ([decred/dcrd#3509](https://github.com/decred/dcrd/pull/3509))
- docs: Update for addrmgr v3 module ([decred/dcrd#3537](https://github.com/decred/dcrd/pull/3537))
- sampleconfig: Update dcrctl sample config ([decred/dcrd#3546](https://github.com/decred/dcrd/pull/3546))

### Contrib changes:

- docker: Update image to golang:1.22.3-alpine3.20 ([decred/dcrd#3305](https://github.com/decred/dcrd/pull/3305))
- contrib: Add container/lru to workspace script ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- contrib: Add crypto/rand to workspace script ([decred/dcrd#3380](https://github.com/decred/dcrd/pull/3380))
- docker: Update image to golang:1.22.4-alpine3.20 ([decred/dcrd#3370](https://github.com/decred/dcrd/pull/3370))
- docker: Update image to golang:1.22.5-alpine3.20 ([decred/dcrd#3400](https://github.com/decred/dcrd/pull/3400))
- docker: Update image to golang:1.22.6-alpine3.20 ([decred/dcrd#3412](https://github.com/decred/dcrd/pull/3412))
- contrib: Move scripts to devtools subdir ([decred/dcrd#3416](https://github.com/decred/dcrd/pull/3416))
- contrib: Add script to bump docker image version ([decred/dcrd#3417](https://github.com/decred/dcrd/pull/3417))
- contrib: Add script to bump assumevalid block ([decred/dcrd#3418](https://github.com/decred/dcrd/pull/3418))
- contrib: Add script to bump min known chain work ([decred/dcrd#3419](https://github.com/decred/dcrd/pull/3419))
- docker: Update image to golang:1.23.0-alpine3.20 ([decred/dcrd#3423](https://github.com/decred/dcrd/pull/3423))
- docker: Update image to golang:1.23.1-alpine3.20 ([decred/dcrd#3440](https://github.com/decred/dcrd/pull/3440))
- docker: Update image to golang:1.23.2-alpine3.20 ([decred/dcrd#3447](https://github.com/decred/dcrd/pull/3447))
- docker: Update image to golang:1.23.3-alpine3.20 ([decred/dcrd#3452](https://github.com/decred/dcrd/pull/3452))
- docker: Update image to golang:1.23.4-alpine3.21 ([decred/dcrd#3462](https://github.com/decred/dcrd/pull/3462))
- docker: Update image to golang:1.24.1-alpine3.21 ([decred/dcrd#3481](https://github.com/decred/dcrd/pull/3481))
- docker: Update image to golang:1.24.2-alpine3.21 ([decred/dcrd#3487](https://github.com/decred/dcrd/pull/3487))
- docker: Update image to golang:1.24.5-alpine3.22 ([decred/dcrd#3497](https://github.com/decred/dcrd/pull/3497))
- docker: Update image to golang:1.25.0-alpine3.22 ([decred/dcrd#3503](https://github.com/decred/dcrd/pull/3503))
- docker: Update image to golang:1.25.3-alpine3.22 ([decred/dcrd#3515](https://github.com/decred/dcrd/pull/3515))
- docker: Update image to golang:1.25.4-alpine3.22 ([decred/dcrd#3530](https://github.com/decred/dcrd/pull/3530))

### Developer-related package and module changes:

- mixpool: Require 1 block confirmation ([decred/dcrd#3295](https://github.com/decred/dcrd/pull/3295))
- mixpool: Add missing mutex acquire ([decred/dcrd#3296](https://github.com/decred/dcrd/pull/3296))
- mixing: Reduce a couple of allocation cases ([decred/dcrd#3298](https://github.com/decred/dcrd/pull/3298))
- mixpool: Wrap non-bannable errors with RuleError ([decred/dcrd#3301](https://github.com/decred/dcrd/pull/3301))
- server: Add logs on why a peer is banned ([decred/dcrd#3306](https://github.com/decred/dcrd/pull/3306))
- server: Make peer banning synchronous ([decred/dcrd#3299](https://github.com/decred/dcrd/pull/3299))
- server: Consolidate ban reason logging ([decred/dcrd#3309](https://github.com/decred/dcrd/pull/3309))
- mixclient: Do not remove PRs from failed runs ([decred/dcrd#3308](https://github.com/decred/dcrd/pull/3308))
- addrmgr: Give unattempted addresses a chance ([decred/dcrd#3300](https://github.com/decred/dcrd/pull/3300))
- multi: Use index for IPv6 zone ([decred/dcrd#3303](https://github.com/decred/dcrd/pull/3303))
- mixclient: Improve error returning ([decred/dcrd#3312](https://github.com/decred/dcrd/pull/3312))
- server: Add missing ban reason ([decred/dcrd#3321](https://github.com/decred/dcrd/pull/3321))
- mixpool: Debug log all removed messages ([decred/dcrd#3326](https://github.com/decred/dcrd/pull/3326))
- peer,server: Hash mix messages ASAP ([decred/dcrd#3328](https://github.com/decred/dcrd/pull/3328))
- mixclient: Unexport ErrExpired ([decred/dcrd#3330](https://github.com/decred/dcrd/pull/3330))
- peer: Add mix message summary strings ([decred/dcrd#3327](https://github.com/decred/dcrd/pull/3327))
- mixclient: Log identities of blamed peers ([decred/dcrd#3331](https://github.com/decred/dcrd/pull/3331))
- mixclient: Avoid overwriting prev PRs slice ([decred/dcrd#3332](https://github.com/decred/dcrd/pull/3332))
- mixclient: Log reasons for blaming during run ([decred/dcrd#3333](https://github.com/decred/dcrd/pull/3333))
- dcrd: Switch getdata log from trace to debug ([decred/dcrd#3329](https://github.com/decred/dcrd/pull/3329))
- server: Remove dup min protocol version check ([decred/dcrd#3335](https://github.com/decred/dcrd/pull/3335))
- mempool: Return missing prev outpoints ([decred/dcrd#3337](https://github.com/decred/dcrd/pull/3337))
- netsync: Split header sync from block sync ([decred/dcrd#3324](https://github.com/decred/dcrd/pull/3324))
- mixpool: Debug log accepted messages ([decred/dcrd#3339](https://github.com/decred/dcrd/pull/3339))
- mixpool: Log correct accepted orphans ([decred/dcrd#3340](https://github.com/decred/dcrd/pull/3340))
- mixpool: Remove unused entry run field ([decred/dcrd#3342](https://github.com/decred/dcrd/pull/3342))
- rpc/jsonrpc/types: Add {start,stop}profiler ([decred/dcrd#3323](https://github.com/decred/dcrd/pull/3323))
- mixpool: Remove unused exported methods ([decred/dcrd#3344](https://github.com/decred/dcrd/pull/3344))
- mixclient: Respect standard tx size limits ([decred/dcrd#3338](https://github.com/decred/dcrd/pull/3338))
- mixclient: Fix build ([decred/dcrd#3347](https://github.com/decred/dcrd/pull/3347))
- mixpool: Don't alloc error on unknown msg ([decred/dcrd#3348](https://github.com/decred/dcrd/pull/3348))
- main: Add .gitattributes ([decred/dcrd#3358](https://github.com/decred/dcrd/pull/3358))
- mixing: Prealloc buffers ([decred/dcrd#3361](https://github.com/decred/dcrd/pull/3361))
- mixpool: Remove Receive expectedMessages argument ([decred/dcrd#3364](https://github.com/decred/dcrd/pull/3364))
- container/lru: Implement type safe generic LRUs ([decred/dcrd#3359](https://github.com/decred/dcrd/pull/3359))
- peer/uprng: Remove unneeded carry add ([decred/dcrd#3369](https://github.com/decred/dcrd/pull/3369))
- crypto/rand: Implement module ([decred/dcrd#3371](https://github.com/decred/dcrd/pull/3371))
- peer: Use container/lru module ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- blockchain: Use container/lru module ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- mining: Use container/lru module ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- rand: Add BigInt ([decred/dcrd#3374](https://github.com/decred/dcrd/pull/3374))
- rand: Uppercase N in half-open-range funcs ([decred/dcrd#3375](https://github.com/decred/dcrd/pull/3375))
- multi: Use new crypto/rand module ([decred/dcrd#3373](https://github.com/decred/dcrd/pull/3373))
- rand: Add rand.N generic func ([decred/dcrd#3376](https://github.com/decred/dcrd/pull/3376))
- rand: Add ShuffleSlice ([decred/dcrd#3377](https://github.com/decred/dcrd/pull/3377))
- rand: Add benchmarks ([decred/dcrd#3378](https://github.com/decred/dcrd/pull/3378))
- multi: Use rand.ShuffleSlice ([decred/dcrd#3382](https://github.com/decred/dcrd/pull/3382))
- mixpool: Remove run from conflicting msg err ([decred/dcrd#3385](https://github.com/decred/dcrd/pull/3385))
- mixpool: Remove more references to runs ([decred/dcrd#3386](https://github.com/decred/dcrd/pull/3386))
- mixing: Reduce slot reservation mix pads allocs ([decred/dcrd#3387](https://github.com/decred/dcrd/pull/3387))
- mixclient: Remove completely unused var ([decred/dcrd#3395](https://github.com/decred/dcrd/pull/3395))
- mixpool: Remove error which is always returned nil ([decred/dcrd#3396](https://github.com/decred/dcrd/pull/3396))
- mixclient: Dont append to slice with non-zero length ([decred/dcrd#3397](https://github.com/decred/dcrd/pull/3397))
- mixing: Add missing copyright headers ([decred/dcrd#3399](https://github.com/decred/dcrd/pull/3399))
- mixclient: Add missing copyright headers ([decred/dcrd#3399](https://github.com/decred/dcrd/pull/3399))
- mixclient: Remove submit queue channel ([decred/dcrd#3401](https://github.com/decred/dcrd/pull/3401))
- mixclient: Do not submit PRs holding client mutex ([decred/dcrd#3401](https://github.com/decred/dcrd/pull/3401))
- txscript: Fix minor typo ([decred/dcrd#3408](https://github.com/decred/dcrd/pull/3408))
- addrmgr: Track network address types ([decred/dcrd#3406](https://github.com/decred/dcrd/pull/3406))
- addrmgr: Remove TorV2 support ([decred/dcrd#3406](https://github.com/decred/dcrd/pull/3406))
- addrmgr: Add convenient new functions ([decred/dcrd#3406](https://github.com/decred/dcrd/pull/3406))
- blake256: Full redesign ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add cpu feature detection ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add specialized SSE2 implementation ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add specialized SSE4.1 implementation ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add specialized AVX implementation ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- server: Refactor serverPeer OnInv handler ([decred/dcrd#3410](https://github.com/decred/dcrd/pull/3410))
- server: Make per-peer inv relay handler a server method ([decred/dcrd#3411](https://github.com/decred/dcrd/pull/3411))
- addrmgr: Fix bug in AddressCache ([decred/dcrd#3409](https://github.com/decred/dcrd/pull/3409))
- addrmgr: Add network address type filter ([decred/dcrd#3409](https://github.com/decred/dcrd/pull/3409))
- addrmgr: Remove DNS lookups from address manager ([decred/dcrd#3409](https://github.com/decred/dcrd/pull/3409))
- netsync: Name received msg handlers with On, not Queue ([decred/dcrd#3414](https://github.com/decred/dcrd/pull/3414))
- secp256k1: Optimize field inverse calc ([decred/dcrd#3421](https://github.com/decred/dcrd/pull/3421))
- blockchain: Consolidate active agenda detection ([decred/dcrd#3435](https://github.com/decred/dcrd/pull/3435))
- secp256k1/schnorr: Expose signature r and s ([decred/dcrd#3437](https://github.com/decred/dcrd/pull/3437))
- secp256k1: Separate affine equality func in tests ([decred/dcrd#3436](https://github.com/decred/dcrd/pull/3436))
- secp256k1: Expose Jacobian point equivalency func ([decred/dcrd#3436](https://github.com/decred/dcrd/pull/3436))
- netsync: Track best known blocks per peer ([decred/dcrd#3444](https://github.com/decred/dcrd/pull/3444))
- netsync: Track peer for requested blocks ([decred/dcrd#3444](https://github.com/decred/dcrd/pull/3444))
- netsync: Dont let limitAdd shrink map below limit ([decred/dcrd#3445](https://github.com/decred/dcrd/pull/3445))
- secp256k1: Return normalized val from DecompressY ([decred/dcrd#3441](https://github.com/decred/dcrd/pull/3441))
- mixclient: Avoid jitter calculation panic ([decred/dcrd#3448](https://github.com/decred/dcrd/pull/3448))
- mixclient: Detect exited csppsolver processes ([decred/dcrd#3451](https://github.com/decred/dcrd/pull/3451))
- mixclient: Sort roots for slot assignment ([decred/dcrd#3453](https://github.com/decred/dcrd/pull/3453))
- rand: Use stdlib crypto/rand.Read on OpenBSD + Go 1.24 ([decred/dcrd#3464](https://github.com/decred/dcrd/pull/3464))
- mixclient: Wait for KEs from all attempted sessions ([decred/dcrd#3463](https://github.com/decred/dcrd/pull/3463))
- mixclient: Wait for runs to finish before closing client ([decred/dcrd#3467](https://github.com/decred/dcrd/pull/3467))
- blockchain: Avoid racy math/rand access ([decred/dcrd#3480](https://github.com/decred/dcrd/pull/3480))
- peer: Synchronize net.Conn with a mutex ([decred/dcrd#3478](https://github.com/decred/dcrd/pull/3478))
- mixclient: Error out of run if gen/ke errors ([decred/dcrd#3484](https://github.com/decred/dcrd/pull/3484))
- multi: Misc error simplification and cleanup ([decred/dcrd#3491](https://github.com/decred/dcrd/pull/3491))
- blockchain: Consolidate not in main chain err fmt ([decred/dcrd#3492](https://github.com/decred/dcrd/pull/3492))
- mixclient: Avoid nil deref on errored run ([decred/dcrd#3505](https://github.com/decred/dcrd/pull/3505))
- mixclient: Use helper func to send peer errors ([decred/dcrd#3506](https://github.com/decred/dcrd/pull/3506))
- mixclient: Don't serve canceled local peers ([decred/dcrd#3507](https://github.com/decred/dcrd/pull/3507))
- netsync: Implement separate mutex for peers ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Separate initial hdr sync done handling ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Track if a peer serves data ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make sync peer candidacy concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Use atomic for sync height ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Use atomic for is current flag ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Use atomic for headers synced flag ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make sync peer concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make request maps concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make next block fetch concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make stall timeout concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make add peer connect logic synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make add peer dc logic synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Use atomic for num consec orphan hdrs ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make best ann tracking concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Split initial header and chain sync ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make hdr sync transition concurrent safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make header handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make chain sync transition conc safe ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make block handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make inv handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make notfound handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make transaction handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make mix message handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make data request handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make block processing synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Make get sync peer handling synchronous ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Remove unused msg channel ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Rename event handler to stall handler ([decred/dcrd#3496](https://github.com/decred/dcrd/pull/3496))
- netsync: Simplify request maps ([decred/dcrd#3498](https://github.com/decred/dcrd/pull/3498))
- netsync: Separate mix message request ([decred/dcrd#3499](https://github.com/decred/dcrd/pull/3499))
- netsync: Rework external request from peer ([decred/dcrd#3500](https://github.com/decred/dcrd/pull/3500))
- mixclient: Prevent busy looping with canceled local peers ([decred/dcrd#3511](https://github.com/decred/dcrd/pull/3511))
- mixclient: Suppress ERR log for no active local peers ([decred/dcrd#3512](https://github.com/decred/dcrd/pull/3512))
- blockchain: Sum treasury changes in single db tx ([decred/dcrd#3513](https://github.com/decred/dcrd/pull/3513))
- blockchain: Remove deprecated trsy subsidy check ([decred/dcrd#3514](https://github.com/decred/dcrd/pull/3514))
- blockchain: Remove unnecessary func param ([decred/dcrd#3514](https://github.com/decred/dcrd/pull/3514))
- blockchain: Remove unneeded assert error ([decred/dcrd#3514](https://github.com/decred/dcrd/pull/3514))
- blockchain: Improve some treasury error messages ([decred/dcrd#3514](https://github.com/decred/dcrd/pull/3514))
- blockchain: Improve some treasury log messages ([decred/dcrd#3514](https://github.com/decred/dcrd/pull/3514))
- rpcclient: Check context before performing requests ([decred/dcrd#3518](https://github.com/decred/dcrd/pull/3518))
- blockchain: Allow trsy spends of maturing funds ([decred/dcrd#3520](https://github.com/decred/dcrd/pull/3520))
- mixclient: Don't exit blame assignment for no local peers ([decred/dcrd#3532](https://github.com/decred/dcrd/pull/3532))
- mixclient: Generate SR, DC keys at local peer creation ([decred/dcrd#3533](https://github.com/decred/dcrd/pull/3533))
- mixclient: Correct requeueing of unmixed peers ([decred/dcrd#3534](https://github.com/decred/dcrd/pull/3534))
- peer: Use dcrd crypto/rand for self detect nonce ([decred/dcrd#3545](https://github.com/decred/dcrd/pull/3545))
- mixclient: Do not cancel mixes before next epoch ([decred/dcrd#3553](https://github.com/decred/dcrd/pull/3553))
- blockchain: Use consolidated agenda detection ([decred/dcrd#3550](https://github.com/decred/dcrd/pull/3550))
- rpcclient: Add the clientcert authorization method ([decred/dcrd#3552](https://github.com/decred/dcrd/pull/3552))

### Developer-related module management:

- rpcclient: Require golang/x/net 0.25.0 ([decred/dcrd#3291](https://github.com/decred/dcrd/pull/3291))
- mixing: Use stdaddr.Hash160 instead of dcrutil ([decred/dcrd#3297](https://github.com/decred/dcrd/pull/3297))
- dcrjson: Prepare v4.1.0 ([decred/dcrd#3313](https://github.com/decred/dcrd/pull/3313))
- rpc/jsonrpc/types: Prepare v4.3.0 ([decred/dcrd#3314](https://github.com/decred/dcrd/pull/3314))
- main: Update to use all new module versions ([decred/dcrd#3349](https://github.com/decred/dcrd/pull/3349))
- lru: Deprecate module ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- lru: Remove old module in favor of container/lru ([decred/dcrd#3360](https://github.com/decred/dcrd/pull/3360))
- multi: Bump rand consumer mods to require 1.18 ([decred/dcrd#3381](https://github.com/decred/dcrd/pull/3381))
- multi: Use release version of dcrd/crypto/rand ([decred/dcrd#3384](https://github.com/decred/dcrd/pull/3384))
- addrmgr: Prepare v3 ([decred/dcrd#3405](https://github.com/decred/dcrd/pull/3405))
- certgen: Prepare v1.2.0 ([decred/dcrd#3430](https://github.com/decred/dcrd/pull/3430))
- peer: Prepare v3.1.3 ([decred/dcrd#3455](https://github.com/decred/dcrd/pull/3455))
- mixing: Prepare v0.4.2 ([decred/dcrd#3456](https://github.com/decred/dcrd/pull/3456))
- crypto/rand: Prepare v1.0.1 ([decred/dcrd#3471](https://github.com/decred/dcrd/pull/3471))
- mixing: Prepare v0.5.0 ([decred/dcrd#3472](https://github.com/decred/dcrd/pull/3472))
- secp256k1: Prepare v4.4.0 ([decred/dcrd#3469](https://github.com/decred/dcrd/pull/3469))
- main: Remove Go <= 1.18 memory limits ([decred/dcrd#3483](https://github.com/decred/dcrd/pull/3483))
- main: Use latest Go 1.25 features if possible ([decred/dcrd#3504](https://github.com/decred/dcrd/pull/3504))
- chaincfg/chainhash: Prepare v1.0.5 ([decred/dcrd#3528](https://github.com/decred/dcrd/pull/3528))
- dcrec/edwards: Prepare v2.0.4 ([decred/dcrd#3527](https://github.com/decred/dcrd/pull/3527))
- dcrjson: Prepare v4.2.0 ([decred/dcrd#3531](https://github.com/decred/dcrd/pull/3531))
- wire: Prepare v1.7.1 ([decred/dcrd#3535](https://github.com/decred/dcrd/pull/3535))
- blockchain/standalone: Prepare v2.2.2 ([decred/dcrd#3536](https://github.com/decred/dcrd/pull/3536))
- addrmgr: Prepare v3.0.0 ([decred/dcrd#3537](https://github.com/decred/dcrd/pull/3537))
- connmgr: Prepare v3.1.3 ([decred/dcrd#3538](https://github.com/decred/dcrd/pull/3538))
- gcs: Remove old conditional Go 1.12 code ([decred/dcrd#3539](https://github.com/decred/dcrd/pull/3539))
- multi: Remove old format build tags ([decred/dcrd#3540](https://github.com/decred/dcrd/pull/3540))
- version: Remove old conditional Go 1.18 code ([decred/dcrd#3543](https://github.com/decred/dcrd/pull/3543))
- chaincfg: Prepare v3.3.0 ([decred/dcrd#3559](https://github.com/decred/dcrd/pull/3559))
- txscript: Prepare v4.1.2 ([decred/dcrd#3560](https://github.com/decred/dcrd/pull/3560))
- hdkeychain: Prepare v3.1.3 ([decred/dcrd#3561](https://github.com/decred/dcrd/pull/3561))
- peer: Prepare v3.2.0 ([decred/dcrd#3562](https://github.com/decred/dcrd/pull/3562))
- dcrutil: Prepare v4.0.3 ([decred/dcrd#3563](https://github.com/decred/dcrd/pull/3563))
- mixing: Prepare v0.6.0 ([decred/dcrd#3564](https://github.com/decred/dcrd/pull/3564))
- database: Prepare v3.0.3 ([decred/dcrd#3565](https://github.com/decred/dcrd/pull/3565))
- blockchain/stake: Prepare v5.0.2 ([decred/dcrd#3566](https://github.com/decred/dcrd/pull/3566))
- gcs: Prepare v4.1.1 ([decred/dcrd#3567](https://github.com/decred/dcrd/pull/3567))
- blockchain: Prepare v5.1.0 ([decred/dcrd#3568](https://github.com/decred/dcrd/pull/3568))
- rpcclient: Prepare v8.1.0 ([decred/dcrd#3569](https://github.com/decred/dcrd/pull/3569))
- rpc/jsonrpc/types: Prepare v4.4.0 ([decred/dcrd#3571](https://github.com/decred/dcrd/pull/3571))
- main: Update to use all new module versions ([decred/dcrd#3572](https://github.com/decred/dcrd/pull/3572))
- main: Remove module replacements ([decred/dcrd#3573](https://github.com/decred/dcrd/pull/3573))

### Testing and Quality Assurance:

- build: upgrade linter to v1.58.1 ([decred/dcrd#3287](https://github.com/decred/dcrd/pull/3287))
- build: Rework lint script to use go.mod files ([decred/dcrd#3365](https://github.com/decred/dcrd/pull/3365))
- build: Rework test script to use go.mod files ([decred/dcrd#3368](https://github.com/decred/dcrd/pull/3368))
- addrmgr: Reorganize tests ([decred/dcrd#3402](https://github.com/decred/dcrd/pull/3402))
- addrmgr: Increase test coverage ([decred/dcrd#3402](https://github.com/decred/dcrd/pull/3402))
- blake256: Add comprehensive tests ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add benchmarks ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add test infra for specialized impls ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- blake256: Add bench infra for specialized impls ([decred/dcrd#3407](https://github.com/decred/dcrd/pull/3407))
- secp256k1: Address some linter complaints ([decred/dcrd#3420](https://github.com/decred/dcrd/pull/3420))
- secp256k1: Add benchmarks for field inverse ([decred/dcrd#3421](https://github.com/decred/dcrd/pull/3421))
- blockchain: Fix return amts test for Go 1.23+ ([decred/dcrd#3424](https://github.com/decred/dcrd/pull/3424))
- multi: Address some vet and linter complaints ([decred/dcrd#3426](https://github.com/decred/dcrd/pull/3426))
- build: Update to latest action versions ([decred/dcrd#3425](https://github.com/decred/dcrd/pull/3425))
- build: Update golangci-lint to v1.60.1 ([decred/dcrd#3425](https://github.com/decred/dcrd/pull/3425))
- build: Test against Go 1.23 ([decred/dcrd#3425](https://github.com/decred/dcrd/pull/3425))
- build: Add tenv linter ([decred/dcrd#3428](https://github.com/decred/dcrd/pull/3428))
- secp256k1/schnorr: Add tests for R and S methods ([decred/dcrd#3437](https://github.com/decred/dcrd/pull/3437))
- secp256k1: Move curve benchmarks to own file ([decred/dcrd#3436](https://github.com/decred/dcrd/pull/3436))
- secp256k1: Move pubkey benchmarks to own file ([decred/dcrd#3436](https://github.com/decred/dcrd/pull/3436))
- secp256k1: Add Jacobian point equivalency bench ([decred/dcrd#3436](https://github.com/decred/dcrd/pull/3436))
- build: Update to latest action versions ([decred/dcrd#3470](https://github.com/decred/dcrd/pull/3470))
- certgen: Use t.TempDir in tests ([decred/dcrd#3470](https://github.com/decred/dcrd/pull/3470))
- build: Update golangci-lint to v1.64.5 ([decred/dcrd#3470](https://github.com/decred/dcrd/pull/3470))
- build: Test against Go 1.24 ([decred/dcrd#3470](https://github.com/decred/dcrd/pull/3470))
- mixclient: Fix test timeouts ([decred/dcrd#3501](https://github.com/decred/dcrd/pull/3501))
- mixing: Minor linting ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- main: Don't disable a linter which isnt enabled ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- dcrjson: Don't ignore linter which doesnt exist ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- wire: Remove unnecessary linter ignore ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- tickettreap: Dont ignore linter which doesnt exist ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- build: Update golangci-lint to v2.0.2 ([decred/dcrd#3488](https://github.com/decred/dcrd/pull/3488))
- build: Update to latest action versions ([decred/dcrd#3502](https://github.com/decred/dcrd/pull/3502))
- build: Update golangci-lint to v2.4.0 ([decred/dcrd#3502](https://github.com/decred/dcrd/pull/3502))
- build: Test against Go 1.25 ([decred/dcrd#3502](https://github.com/decred/dcrd/pull/3502))
- blockchain: Cleanup casting in treasury tests ([decred/dcrd#3516](https://github.com/decred/dcrd/pull/3516))
- blockchain: Consolidate trsy balance test asserts ([decred/dcrd#3517](https://github.com/decred/dcrd/pull/3517))
- blockchain: Remove params clone in treasury tests ([decred/dcrd#3522](https://github.com/decred/dcrd/pull/3522))
- chaingen: Add decentralized treasury support ([decred/dcrd#3523](https://github.com/decred/dcrd/pull/3523))
- blockchain: Use chaingen trsy support in tests ([decred/dcrd#3524](https://github.com/decred/dcrd/pull/3524))
- blockchain: Remove unused test funcs ([decred/dcrd#3524](https://github.com/decred/dcrd/pull/3524))
- chaingen: Add treasury spend vote support ([decred/dcrd#3525](https://github.com/decred/dcrd/pull/3525))
- blockchain: Use chaingen trsy spend vote support ([decred/dcrd#3526](https://github.com/decred/dcrd/pull/3526))
- blockchain: Use forced deployments in trsy tests ([decred/dcrd#3529](https://github.com/decred/dcrd/pull/3529))
- build: Update to latest action versions ([decred/dcrd#3541](https://github.com/decred/dcrd/pull/3541))
- build: Update golangci-lint to v2.6.1 ([decred/dcrd#3541](https://github.com/decred/dcrd/pull/3541))
- wire: Remove TestRandomUint64 ([decred/dcrd#3544](https://github.com/decred/dcrd/pull/3544))
- build: Use golangci action and make parallel ([decred/dcrd#3542](https://github.com/decred/dcrd/pull/3542))
- build: More descriptive name for job output ([decred/dcrd#3547](https://github.com/decred/dcrd/pull/3547))
- build: More descriptive ID for job ([decred/dcrd#3547](https://github.com/decred/dcrd/pull/3547))
- build: Reduce amount of hand-crafted JSON ([decred/dcrd#3547](https://github.com/decred/dcrd/pull/3547))
- build: Give build step a friendly name ([decred/dcrd#3547](https://github.com/decred/dcrd/pull/3547))
- build: Ensure root dir is linted ([decred/dcrd#3547](https://github.com/decred/dcrd/pull/3547))
- blockchain: Non-zero tspendFee in treasury tests ([decred/dcrd#3556](https://github.com/decred/dcrd/pull/3556))
- blockchain: Use local defs for agenda vote IDs ([decred/dcrd#3548](https://github.com/decred/dcrd/pull/3548))

### Misc:

- release: Bump for 2.1 release cycle ([decred/dcrd#3290](https://github.com/decred/dcrd/pull/3290))
- blockchain: Fix some function names in comments ([decred/dcrd#3341](https://github.com/decred/dcrd/pull/3341))
- multi: Fix some function names in comments ([decred/dcrd#3383](https://github.com/decred/dcrd/pull/3383))
- multi: Add missing periods to comments ([decred/dcrd#3398](https://github.com/decred/dcrd/pull/3398))
- multi: Remove unhelpful/outdated comments ([decred/dcrd#3398](https://github.com/decred/dcrd/pull/3398))
- netsync: Correct function name in comments ([decred/dcrd#3413](https://github.com/decred/dcrd/pull/3413))
- main: Add .editorconfig ([decred/dcrd#3415](https://github.com/decred/dcrd/pull/3415))
- secp256k1: Cleanup and correct some comments ([decred/dcrd#3420](https://github.com/decred/dcrd/pull/3420))
- secp256k1: Correct some comment typos ([decred/dcrd#3429](https://github.com/decred/dcrd/pull/3429))
- secp256k1: Correct prime minus order func comment ([decred/dcrd#3438](https://github.com/decred/dcrd/pull/3438))
- secp256k1: Tighten max magnitudes in comments ([decred/dcrd#3442](https://github.com/decred/dcrd/pull/3442))
- multi: Fix some function names in comments ([decred/dcrd#3461](https://github.com/decred/dcrd/pull/3461))
- server: Replace mixed tab and spaces in comment ([decred/dcrd#3477](https://github.com/decred/dcrd/pull/3477))
- peer: Update comment for initial chain sync ([decred/dcrd#3490](https://github.com/decred/dcrd/pull/3490))
- blockchain: Update comments for initial chain sync ([decred/dcrd#3490](https://github.com/decred/dcrd/pull/3490))
- blockchain: Correct process test comment ([decred/dcrd#3493](https://github.com/decred/dcrd/pull/3493))
- release: Bump for 2.1.0 ([decred/dcrd#3574](https://github.com/decred/dcrd/pull/3574))

### Code Contributors (alphabetical order):

- Dave Collins
- David Hill
- Jamie Holdstock
- Josh Rickmar
- linchizhen
- Matt Hawkins
- tongjicoder
- wanxiangchwng

# dcrd v1.8.0

This is a new major release of dcrd.  Some of the key highlights are:

* Two new consensus vote agendas which allow stakeholders to decide whether or
  not to activate support for the following:
  * Changing the Proof-of-Work hashing algorithm to BLAKE3 and the difficulty algorithm to ASERT
  * Changing the Proof-of-Work and Proof-of-Stake subsidy split from 10%/80% to 1%/89%
* Separation of block hash from Proof-of-Work hash
* BLAKE3 CPU mining support
* Initial sync time reduced by about 20%
* Runtime memory management optimizations
* Faster cryptographic signature validation
* Low fee transaction rejection
* Unspent transaction output set size reduction
* No more checkpoints
* Improved network protocol message responsiveness
* Header proof commitment hash storage
* Address index removal
* Several CLI options deprecated
* Various updates to the RPC server:
  * Total coin supply output correction
  * More stable global communication over WebSockets
  * Winning ticket notifications when unsynced mining on test networks
  * Several other notable updates, additions, and removals related to the JSON-RPC API
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
- [Change PoW/PoS Subsidy Split to 1/89 and Change PoW Algorithm to BLAKE3](https://proposals.decred.org/record/a8501bc)

The following Decred Change Proposals (DCPs) describe the proposed changes in
detail and provide full technical specifications:
- [DCP0011: Change PoW to BLAKE3 and ASERT](https://github.com/decred/dcps/blob/master/dcp-0011/dcp-0011.mediawiki)
- [DCP0012: Change PoW/PoS Subsidy Split To 1/89](https://github.com/decred/dcps/blob/master/dcp-0012/dcp-0012.mediawiki)

## Upgrade Required

**It is extremely important for everyone to upgrade their software to this
latest release even if you don't intend to vote in favor of the agenda.  This
particularly applies to PoW miners as failure to upgrade will result in lost
rewards after block height 777240.  That is estimated to be around June 29th,
2023.**

## Downgrade Warning

The database format in v1.8.0 is not compatible with previous versions of the
software.  This only affects downgrades as users upgrading from previous
versions will see a one time database migration.

Once this migration has been completed, it will no longer be possible to
downgrade to a previous version of the software without having to delete the
database and redownload the chain.

The database migration typically takes around 4-6 minutes on HDDs and 2-3
minutes on SSDs.

## Notable Changes

### Two New Consensus Change Votes

Two new consensus change votes are now available as of this release.  After
upgrading, stakeholders may set their preferences through their wallet.

#### Change PoW to BLAKE3 and ASERT

The first new vote available as of this release has the id `blake3pow`.

The primary goals of this change are to:

* Increase decentralization of proof of work mining by obsoleting the current
  specialized hardware (ASICs) that is only realistically available to the
  existing highly centralized mining monopoly
* Improve the proof of work mining difficulty adjustment algorithm responsiveness
* Provide more equal profitability to steady state PoW miners versus hit and run
  miners

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/a8501bc)
* [DCP0011](https://github.com/decred/dcps/blob/master/dcp-0011/dcp-0011.mediawiki)

#### Change PoW/PoS Subsidy Split to 1/89 Vote

The second new vote available as of this release has the id `changesubsidysplitr2`.

The proposed modification to the subsidy split in tandem with the change to the
PoW hashing function is intended to break up the mining cartel and further
improve decentralization of the issuance process.

See the following for more details:

* [Politeia proposal](https://proposals.decred.org/record/a8501bc)
* [DCP0012](https://github.com/decred/dcps/blob/master/dcp-0012/dcp-0012.mediawiki)

### Separation of Block Hash from Proof-of-Work Hash

A new Proof-of-Work (PoW) hash that is distinct from the existing block hash is
now used for all consensus rules related to PoW verification.

Block hashes have historically served multiple roles which include those related
to proof of work (PoW).  As of this release, the roles related to PoW are now
solely the domain of the new PoW hash.

Some key points related to this change are:

* The new PoW hash will be exactly the same as the existing block hash for all
  blocks prior to the activation of the stakeholder vote to change the PoW
  hashing algorithm
* The block hash continues to use the existing hashing algorithm
* The block hash will no longer have the typical pattern of leading zeros upon
  activation of the PoW hashing algorithm
* The PoW hash will have the typical pattern of leading zeros both before and
  after the activation of the new PoW hashing algorithm

### BLAKE3 CPU Mining Support

The internal CPU miner has been significantly optimized to provide much higher
hash rates, especially when using multiple cores, and now automatically mines
using the BLAKE3 algorithm when the `blake3pow` agenda is active.

### Initial Sync Time Reduced by About 20%

The amount of time it takes to complete the initial chain synchronization
process with default settings has been reduced by about 20% versus the previous
release.

### Runtime Memory Management Optimizations

The way memory is managed has been optimized to provide performance enhancements
to both steady-state operation as well as the initial chain sync process.

The primary benefits are:

* Lower maximum memory usage during transient periods of high demand
* Approximately a 10% reduction to the duration of the initial sync process
* Significantly reduced overall total memory allocations (~42%)
* Less overall CPU usage for the same amount of work

### Faster Cryptographic Signature Validation

Similar to the previous release, this release further improves some aspects of
the underlying crypto code to increase its execution speed and reduce the number
of memory allocations.  The overall result is a 52% reduction in allocations and
about a 1% reduction to the verification time for a single signature.

The primary benefits are:

* Improved vote times since blocks and transactions propagate more quickly
  throughout the network
* Approximately a 4% reduction to the duration of the initial sync process

### Low Fee Transaction Rejection

The default transaction acceptance and relay policy is no longer based on
priority and instead now immediately rejects all transactions that do not pay
the minimum required fee.

This provides a better user experience for transactions that do not pay enough
fees.

For some insight into the motivation for this change, prior to the introduction
of support for child pays for parent (CPFP), it was possible for transactions to
essentially become stuck forever if they didn't pay a high enough fee for miners
to include them in a block.

In order to prevent this, a policy was introduced that allowed relaying
transactions that do not pay enough fees based on a priority calculated from the
fee as well as the age of coins being spent.  The result is that the priority
slowly increased over time as the coins aged to ensure such transactions would
eventually be relayed and mined.  In order to prevent abuse the behavior could
otherwise allow, the policy also included additional rate-limiting of these
types of transactions.

While the policy served its purpose, it had some downsides such as:

* A confusing user experience where transactions that do not pay enough fees and
  also are not old enough to meet the dynamically changing priority requirements
  are rejected due to having insufficient priority instead of not paying enough
  fees as the user might expect
* The priority requirements dynamically change over time which leads to
  non-deterministic behavior and thus ultimately results in what appear to be
  intermittent/transient failures to users

The policy is no longer necessary or desirable given such transactions can now
use CPFP to increase the overall fee of the entire transaction chain thereby
ensuring they are mined.

### Unspent Transaction Output Set Size Reduction

The set of all unspent transaction outputs (UTXO set) no longer contains
unspendable `treasurybase` outputs.

A `treasurybase` output is a special output that increases the balance of the
decentralized treasury account which requires stakeholder approval to spend
funds.  As a result, they do not operate like normal transaction outputs and
therefore are never directly spendable.

Removing these unspendable outputs from the UTXO set reduces its overall size.

### No More Checkpoints

This release introduces a new model for deciding when to reject old forks to
make use of the hard-coded assumed valid block that is updated with each release
to a recent block thereby removing the final remaining usage of checkpoints.

Consequently, the `--nocheckpoints` command-line option and separate
`findcheckpoints` utility have been removed.

### Improved Network Protocol Message Responsiveness (`getheaders`/`getcfilterv2`)

All protocol message requests for headers (`getheaders`) and version 2 compact
filters (`getcfilterv2`) will now receive empty responses when there is not any
available data or the peer is otherwise unwilling to serve the data for a
variety of reasons.

For example, a peer might be unwilling to serve data because they are still
performing the initial sync or temporarily no longer consider themselves synced
with the network due to recently coming back online after being unable to
communicate with the network for a long time.

This change helps improve network robustness by preventing peers from appearing
unresponsive or stalled in such cases.

### Header Proof Commitment Hash Storage

The individual commitment hashes covered by the commitment root field of the
header of each block are now stored in the database for fast access.  This
provides better scaling for generating and serving inclusion proofs as more
commitments are added to the header proof in future upgrades.

### Address Index Removal (`--addrindex`, `--dropaddrindex`)

The previously deprecated optional address index that could be enabled via
`--addrindex` and removed via `--dropaddrindex` is no longer available.  All of
the information previously provided from the address index, and much more, is
available via [dcrdata](https://github.com/decred/dcrdata/).

### Several CLI Options Deprecated

The following CLI options no longer have any effect and are now deprecated:

* `--norelaypriority`
* `--limitfreerelay`
* `--blockminsize`
* `--blockprioritysize`

They will be removed in a future release.

### RPC Server Changes

The RPC server version as of this release is 8.0.0.

#### Total Coin Supply Output Correction (`getcoinsupply`)

The total coin supply reported by `getcoinsupply` will now correctly include the
coins generated as a part of the block reward for the decentralized treasury as
intended.

As a result, the amount reported will now be higher than it was previously.  It
is important to note that this issue was only an RPC display issue and did not
affect consensus in any way.

#### More Stable Global Communication over WebSockets

WebSocket connections now have longer timeouts and remain connected through
transient network timeouts.  This significantly improves the stability of
high-latency connections such as those communicating across multiple continents.

#### Winning Ticket Notifications when Unsynced Mining on Test Networks (`winningtickets`)

Clients that subscribe to receive `winningtickets` notifications via WebSockets
with `notifywinningtickets` will now also receive the notifications on test
networks prior to being fully synced when the `--allowunsyncedmining` CLI option
is provided.

See the following for API details:

* [notifywinningtickets JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#notifywinningtickets)
* [winningtickets JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#winningtickets)

#### Transaction Fee Priority Fields on Mempool RPC Deprecated (`getrawmempool`)

Due to the removal of the policy related to low fee transaction priority, the
`startingpriority` and `currentpriority` fields of the results of the verbose
output of the `getrawmempool` RPC are now deprecated.  They will always be set
to 0 and are scheduled to be removed in a future version.

See the
[getrawmempool JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getrawmempool)
for API details.

#### Removal of Raw Transaction Search RPC (`searchrawtransactions`)

The deprecated `searchrawtransactions` RPC, which could previously be used to
obtain all transactions that either credit or debit a given address via RPC is
no longer available.

Callers that wish to access details related to addresses are encouraged to use
[dcrdata](https://github.com/decred/dcrdata/) instead.

#### Removal of Address Index Status Field on Info RPC (`getinfo`)

The `addrindex` field of the `getinfo` RPC is no longer available.

See the
[getinfo JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getinfo)
for API details.

#### Removal of Missed and Expired Ticket RPCs

Now that missed and expired tickets are automatically revoked by the consensus
rules, all RPCs related to querying and requesting notifications for missed and
expired tickets are no longer available.

In particular, the following deprecated RPCs are no longer available:

* `missedtickets`
* `rebroadcastmissed`
* `existsmissedtickets`
* `existsexpiredtickets`
* `notifyspentandmissedtickets`

#### Updates to Work RPC (`getwork`)

The `getwork` RPC will now return an error message immediately if block template
generation is temporarily unable to generate a template indicating the reason.
Previously, the RPC would block until a new template was eventually generated
which could potentially be an exceedingly long time.

Additionally, cancelling a `getwork` invocation before the work has been fully
generated will now cancel the underlying request which allows the RPC server to
immediately service other queued work requests.

See the
[getwork JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getwork)
for API details.

## Changelog

This release consists of 439 git commits from 18 contributors which total to 408
files changed, 25840 additional lines of code, and 22871 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.7.7...release-v1.8.0).

### Protocol and network:

- server: Force PoW upgrade to v9 ([decred/dcrd#2874](https://github.com/decred/dcrd/pull/2874))
- blockchain: Optimize old block ver upgrade checks ([decred/dcrd#2912](https://github.com/decred/dcrd/pull/2912))
- server: Fix syncNotified race ([decred/dcrd#2932](https://github.com/decred/dcrd/pull/2932))
- multi: Rework old fork rejection logic ([decred/dcrd#2945](https://github.com/decred/dcrd/pull/2945))
- blockchain: Implement header proof storage ([decred/dcrd#2938](https://github.com/decred/dcrd/pull/2938))
- chaincfg: Remove planetdecred seeders ([decred/dcrd#2974](https://github.com/decred/dcrd/pull/2974))
- netsync: Improve sync height tracking ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- blockchain: Enforce testnet difficulty throttling ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- chaincfg: Deprecate min diff reduction params ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- blockchain: Don't add treasurybase utxos ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Remove unspendable utxo set entries ([decred/dcrd#2996](https://github.com/decred/dcrd/pull/2996))
- server: Update peer attempt timestamp before dialing ([decred/dcrd#3014](https://github.com/decred/dcrd/pull/3014))
- server: Bump supported wire pver for reject removal ([decred/dcrd#3017](https://github.com/decred/dcrd/pull/3017))
- peer: Use latest pver by default ([decred/dcrd#3019](https://github.com/decred/dcrd/pull/3019))
- server: Always respond to getheaders ([decred/dcrd#3030](https://github.com/decred/dcrd/pull/3030))
- server: Always serve known getcfilterv2 filters ([decred/dcrd#3035](https://github.com/decred/dcrd/pull/3035))
- chaincfg: Enforce globally unique vote IDs ([decred/dcrd#3057](https://github.com/decred/dcrd/pull/3057))
- multi: Add forced deployment result network param ([decred/dcrd#3060](https://github.com/decred/dcrd/pull/3060))
- peer: Correct known inventory check ([decred/dcrd#3074](https://github.com/decred/dcrd/pull/3074))
- netsync: Re-request data sooner after peer disconnect ([decred/dcrd#3067](https://github.com/decred/dcrd/pull/3067))
- chaincfg: Introduce BLAKE3 PoW agenda ([decred/dcrd#3089](https://github.com/decred/dcrd/pull/3089))
- chaincfg: Introduce subsidy split change r2 agenda ([decred/dcrd#3090](https://github.com/decred/dcrd/pull/3090))
- multi: Implement DCP0012 subsidy consensus vote ([decred/dcrd#3092](https://github.com/decred/dcrd/pull/3092))
- multi: Separate block hash and proof of work hash ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- chaincfg: Add params for DCP0011 blake3 diff calc ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- multi: Implement DCP0011 PoW hash consensus vote ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- chaincfg: Update assume valid for release ([decred/dcrd#3122](https://github.com/decred/dcrd/pull/3122))
- chaincfg: Update min known chain work for release ([decred/dcrd#3123](https://github.com/decred/dcrd/pull/3123))
- blockchain: Reject old block vers for pow vote ([decred/dcrd#3135](https://github.com/decred/dcrd/pull/3135))
- server: Force PoW upgrade to v10 ([decred/dcrd#3137](https://github.com/decred/dcrd/pull/3137))

### Transaction relay (memory pool):

- mempool: Invert reorg transaction handling ([decred/dcrd#2956](https://github.com/decred/dcrd/pull/2956))
- mempool: Explicitly reject standalone treasurybase ([decred/dcrd#2963](https://github.com/decred/dcrd/pull/2963))
- mempool: Do not accept low-fee/free transactions ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- mempool: Remove unused verbose tx curprio field ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- mempool: Remove unused tx desc starting prio field ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))

### Mining:

- mining: Use test net block ver on simnet ([decred/dcrd#2868](https://github.com/decred/dcrd/pull/2868))
- mining: Wait for initial sync to generate template ([decred/dcrd#2897](https://github.com/decred/dcrd/pull/2897))
- cpuminer: Rework speed stat tracking ([decred/dcrd#2977](https://github.com/decred/dcrd/pull/2977))
- cpuminer: Significantly optimize mining workers ([decred/dcrd#2977](https://github.com/decred/dcrd/pull/2977))
- mining: Remove high prio/free tx mining code ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- mining: Remove testnet min diff reduction support ([decred/dcrd#3109](https://github.com/decred/dcrd/pull/3109))
- mining: Update to latest block vers for pow vote ([decred/dcrd#3136](https://github.com/decred/dcrd/pull/3136))

### RPC:

- rpcserver: Update websocket ping timeout handling ([decred/dcrd#2865](https://github.com/decred/dcrd/pull/2865))
- rpcserver: Fix websocket auth failure ([decred/dcrd#2877](https://github.com/decred/dcrd/pull/2877))
- rpcserver: Remove missedtickets RPC ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcserver: Remove rebroadcastmissed RPC ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcserver: Remove existsmissedtickets RPC ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcserver: Remove existsexpiredtickets RPC ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcserver: Remove notifyspentandmissedtickets RPC ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcserver: Remove searchrawtransactions ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- rpcserver: Remove unused AddrIndexer interface ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- rpcserver: Remove getinfo addrindex field ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- rpcserver: Return template errors from getwork RPC ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- server: Send winning tickets when unsynced mining ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- rpcserver: Return 0 for deprecated priority fields ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- rpcserver: Support getwork cancellation ([decred/dcrd#3027](https://github.com/decred/dcrd/pull/3027))
- rpcserver: Remove unused method refs from limited ([decred/dcrd#3033](https://github.com/decred/dcrd/pull/3033))
- rpcserver: Decouple RPC agenda info status strings ([decred/dcrd#3071](https://github.com/decred/dcrd/pull/3071))
- blockchain: Correct total subsidy calculation ([decred/dcrd#3112](https://github.com/decred/dcrd/pull/3112))

### dcrd command-line flags and configuration:

- server/indexers: Remove address index support ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- config: Deprecate norelaypriority CLI option ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- config: Deprecate limitfreerelay CLI option ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- config: Deprecate min block size CLI option ([decred/dcrd#3002](https://github.com/decred/dcrd/pull/3002))
- config: Deprecate block prio size CLI option ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- config: Minor description consistency cleanup ([decred/dcrd#3041](https://github.com/decred/dcrd/pull/3041))

### dcrd server runtime interface changes:

- dcrd: Support SIGTERM on Win and all unix variants ([decred/dcrd#2958](https://github.com/decred/dcrd/pull/2958))
- main: Remove no longer needed max cores config ([decred/dcrd#3016](https://github.com/decred/dcrd/pull/3016))
- main: Tweak runtime GC params for Go 1.19 ([decred/dcrd#3016](https://github.com/decred/dcrd/pull/3016))
- server: Add bound addresses IPC events ([decred/dcrd#3020](https://github.com/decred/dcrd/pull/3020))

### findcheckpoint utility changes:

- findcheckpoints: Remove utility ([decred/dcrd#2945](https://github.com/decred/dcrd/pull/2945))

### Documentation:

- docs: Add release notes for v1.7.0 ([decred/dcrd#2858](https://github.com/decred/dcrd/pull/2858))
- docs: Add release notes for v1.7.1 ([decred/dcrd#2881](https://github.com/decred/dcrd/pull/2881))
- docs: Update Min Recommended Disc Space ([decred/dcrd#2906](https://github.com/decred/dcrd/pull/2906))
- docs: Update doc.go with latest arguments ([decred/dcrd#2924](https://github.com/decred/dcrd/pull/2924))
- docs: add backport documentation ([decred/dcrd#2934](https://github.com/decred/dcrd/pull/2934))
- primitives: Update README.md for subsidy calcs ([decred/dcrd#2933](https://github.com/decred/dcrd/pull/2933))
- docs: Remove missedtickets JSON-RPC API ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- docs: Remove rebroadcastmissed JSON-RPC API ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- docs: Remove existsmissedtickets JSON-RPC API ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- docs: Remove existsexpiredtickets JSON-RPC API ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- docs: Remove (notify)spentandmissed JSON-RPC API ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- docs: Add release notes for v1.7.2 ([decred/dcrd#2940](https://github.com/decred/dcrd/pull/2940))
- blockchain: Comment concurrency semantics ([decred/dcrd#2946](https://github.com/decred/dcrd/pull/2946))
- docs: Remove searchrawtransactions JSON-RPC API ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- docs: Remove getinfo addrindex field JSON-RPC API ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- docs: Update indexers readme module path ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- docs: Update indexers readme for removed indexes ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- docs: Update chaingen readme module path ([decred/dcrd#2952](https://github.com/decred/dcrd/pull/2952))
- mempool/docs: Update low-fee/free tx policy removal ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- docs: Update dcrutil/v3 references to v4 ([decred/dcrd#2980](https://github.com/decred/dcrd/pull/2980))
- docs: Add release notes for v1.7.4 ([decred/dcrd#2982](https://github.com/decred/dcrd/pull/2982))
- docs: Update README.md to required Go 1.18/1.19 ([decred/dcrd#2981](https://github.com/decred/dcrd/pull/2981))
- rpcserver: Remove unused result help text ([decred/dcrd#3001](https://github.com/decred/dcrd/pull/3001))
- docs: Deprecate JSON-RPC API getrawmempool prio ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- docs: Add release notes for v1.7.5 ([decred/dcrd#3009](https://github.com/decred/dcrd/pull/3009))
- rpcserver: Update rescan help to match reality ([decred/dcrd#3032](https://github.com/decred/dcrd/pull/3032))
- docs: Make JSON-RPC rescan docs match reality ([decred/dcrd#3032](https://github.com/decred/dcrd/pull/3032))
- docs: Remove {stop,}notifyreceived JSON-RPC API ([decred/dcrd#3034](https://github.com/decred/dcrd/pull/3034))
- docs: Remove recvtx ntfn JSON-RPC API ([decred/dcrd#3034](https://github.com/decred/dcrd/pull/3034))
- docs: Remove {stop,}notifyspent JSON-RPC API ([decred/dcrd#3034](https://github.com/decred/dcrd/pull/3034))
- docs: Remove redeemingtx ntfn JSON-RPC API ([decred/dcrd#3034](https://github.com/decred/dcrd/pull/3034))
- docs: Update chaincfg examples ([decred/dcrd#3040](https://github.com/decred/dcrd/pull/3040))
- docs: Don't use deprecated ioutil package ([decred/dcrd#3046](https://github.com/decred/dcrd/pull/3046))
- docs: Update README.md to required Go 1.19/1.20 ([decred/dcrd#3052](https://github.com/decred/dcrd/pull/3052))
- docs: Add release notes for v1.7.7 ([decred/dcrd#3088](https://github.com/decred/dcrd/pull/3088))
- docs: Update for rpc/jsonrpc/types/v4 module ([decred/dcrd#3103](https://github.com/decred/dcrd/pull/3103))
- docs: Update simnet env docs for subsidy split r2 ([decred/dcrd#3092](https://github.com/decred/dcrd/pull/3092))
- docs: Update for blockchain/stake v5 module ([decred/dcrd#3131](https://github.com/decred/dcrd/pull/3131))
- docs: Update for gcs v4 module ([decred/dcrd#3132](https://github.com/decred/dcrd/pull/3132))
- docs: Update for blockchain v5 module ([decred/dcrd#3133](https://github.com/decred/dcrd/pull/3133))
- docs: Update for rpcclient v8 module ([decred/dcrd#3134](https://github.com/decred/dcrd/pull/3134))

### Contrib changes:

- contrib: Use bash builtins instead of seq ([decred/dcrd#2867](https://github.com/decred/dcrd/pull/2867))
- docker: Update image to golang:1.18.0-alpine3.15 ([decred/dcrd#2907](https://github.com/decred/dcrd/pull/2907))
- contrib: Add Go multimod workspace setup script ([decred/dcrd#2904](https://github.com/decred/dcrd/pull/2904))
- docker: Update image to golang:1.18.3-alpine3.16 ([decred/dcrd#2960](https://github.com/decred/dcrd/pull/2960))
- docker: Update image to golang:1.19.0-alpine3.16 ([decred/dcrd#2983](https://github.com/decred/dcrd/pull/2983))
- docker: Update image to golang:1.19.1-alpine3.16 ([decred/dcrd#2992](https://github.com/decred/dcrd/pull/2992))
- docker: Update image to golang:1.19.5-alpine3.17 ([decred/dcrd#3043](https://github.com/decred/dcrd/pull/3043))
- contrib: Docker forward entrypoint signals ([decred/dcrd#3044](https://github.com/decred/dcrd/pull/3044))
- contrib: Finish Docker documentation ([decred/dcrd#3045](https://github.com/decred/dcrd/pull/3045))
- docker: Add ability to build versioned docker images ([decred/dcrd#3048](https://github.com/decred/dcrd/pull/3048))
- docker: Update image to golang:1.20.1-alpine3.17 ([decred/dcrd#3063](https://github.com/decred/dcrd/pull/3063))
- docker: Add dcrctl version output ([decred/dcrd#3062](https://github.com/decred/dcrd/pull/3062))

### Developer-related package and module changes:

- blockchain: Change tspend pass log level to debug ([decred/dcrd#2862](https://github.com/decred/dcrd/pull/2862))
- stdscript: Reject multisig neg thresholds ([decred/dcrd#2859](https://github.com/decred/dcrd/pull/2859))
- stdaddr: Limit v0 addr decode to max possible size ([decred/dcrd#2860](https://github.com/decred/dcrd/pull/2860))
- hdkeychain: Limit decode to max possible size ([decred/dcrd#2860](https://github.com/decred/dcrd/pull/2860))
- dcrutil: Limit WIF decode to max possible size ([decred/dcrd#2860](https://github.com/decred/dcrd/pull/2860))
- txscript: Support min int64 script num encoding ([decred/dcrd#2863](https://github.com/decred/dcrd/pull/2863))
- edwards: More strict pubkey parsing ([decred/dcrd#2869](https://github.com/decred/dcrd/pull/2869))
- server: sync peer handling sends/receives ([decred/dcrd#2864](https://github.com/decred/dcrd/pull/2864))
- indexers: Cleanup hash and string handling ([decred/dcrd#2873](https://github.com/decred/dcrd/pull/2873))
- ffldb: Add dbErr to error description ([decred/dcrd#2876](https://github.com/decred/dcrd/pull/2876))
- blockchain: Add ldbErr to error description ([decred/dcrd#2876](https://github.com/decred/dcrd/pull/2876))
- secp256k1: Support go generate w/o removing file ([decred/dcrd#2885](https://github.com/decred/dcrd/pull/2885))
- secp256k1: Optimize precomp value storage ([decred/dcrd#2885](https://github.com/decred/dcrd/pull/2885))
- chain: Add currentDeploymentVersion helper ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- chain: Add nextDeploymentVersion helper ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- chain: Add deployment version to db ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- chain: Track deployment version ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- chain: Add newDeploymentsStartTime helper ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- chain: Revalidate blocks for new deployments ([decred/dcrd#2878](https://github.com/decred/dcrd/pull/2878))
- secp256k1: Decouple precomputation generation ([decred/dcrd#2886](https://github.com/decred/dcrd/pull/2886))
- secp256k1: Rework k splitting tests ([decred/dcrd#2888](https://github.com/decred/dcrd/pull/2888))
- secp256k1: Generate all endomorphism params ([decred/dcrd#2888](https://github.com/decred/dcrd/pull/2888))
- secp256k1: Optimize k splitting with mod n scalar ([decred/dcrd#2888](https://github.com/decred/dcrd/pull/2888))
- txscript: Reduce checkmultisig allocs ([decred/dcrd#2890](https://github.com/decred/dcrd/pull/2890))
- secp256k1: Reduce scalar base mult copies ([decred/dcrd#2898](https://github.com/decred/dcrd/pull/2898))
- indexers: fix indexer wait for sync ([decred/dcrd#2871](https://github.com/decred/dcrd/pull/2871))
- secp256k1/ecdsa: Accept nonce in internal signing ([decred/dcrd#2908](https://github.com/decred/dcrd/pull/2908))
- secp256k1/ecdsa: Consistent sig recovery errors ([decred/dcrd#2914](https://github.com/decred/dcrd/pull/2914))
- chaincfg: Deprecate subsidy split params ([decred/dcrd#2916](https://github.com/decred/dcrd/pull/2916))
- internal/mining: createCoinbaseTx never returns an error ([decred/dcrd#2917](https://github.com/decred/dcrd/pull/2917))
- blockchain: Remove unused params ([decred/dcrd#2918](https://github.com/decred/dcrd/pull/2918))
- stake: Use a single copy instead of a for loop ([decred/dcrd#2919](https://github.com/decred/dcrd/pull/2919))
- primitives: Add subsidy calcs ([decred/dcrd#2920](https://github.com/decred/dcrd/pull/2920))
- primitives: Add subsidy calc benchmarks ([decred/dcrd#2920](https://github.com/decred/dcrd/pull/2920))
- rpcserver: cleanup queueHandler process ([decred/dcrd#2929](https://github.com/decred/dcrd/pull/2929))
- rpcclient: Remove missedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- jsonrpc/types: Remove deprecated missedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- blockchain: Remove unused MissedTickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- jsonrpc/types: Remove rebroadcastmissed ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcclient: Remove existsmissedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- blockchain: Remove unused CheckMissedTickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- jsonrpc/types: Remove existsmissedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcclient: Remove existsexpiredtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- blockchain: Remove unused CheckExpiredTickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- jsonrpc/types: Remove existsexpiredtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- rpcclient: Remove notifyspentandmissedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- blockchain: Remove unused NTSpentAndMissedTickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- jsonrpc/types: Remove notifyspentandmissedtickets ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- internal/mempool: remove unused isTreasuryEnabled param ([decred/dcrd#2917](https://github.com/decred/dcrd/pull/2917))
- multi: Remove auto revoc flag from CheckSSRtx ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove auto revoc flag from IsSSRtx ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove treasury flag from CheckSSGenVotes ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove treasury flag from CheckSSGen ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove treasury flag from IsSSGen ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove treasury flag from several funcs ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- stake: Tx ver as agenda proxy in DetermineTxType ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove agenda flags from DetermineTxType ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- multi: Remove agenda flags from several funcs ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- server: Remove unnecessary vote ntfn check ([decred/dcrd#2942](https://github.com/decred/dcrd/pull/2942))
- blockchain: Export block index flush ([decred/dcrd#2943](https://github.com/decred/dcrd/pull/2943))
- blockchain: Consolidate best header access ([decred/dcrd#2943](https://github.com/decred/dcrd/pull/2943))
- blockchain: Optimize stake node pruning ([decred/dcrd#2943](https://github.com/decred/dcrd/pull/2943))
- blockchain: Remove unused errors ([decred/dcrd#2947](https://github.com/decred/dcrd/pull/2947))
- multi: Consolidate header proof logic ([decred/dcrd#2937](https://github.com/decred/dcrd/pull/2937))
- blockchain: Fix revocation fee limit bug ([decred/dcrd#2948](https://github.com/decred/dcrd/pull/2948))
- rpc/jsonrpc/types: Add agenda status constants ([decred/dcrd#2939](https://github.com/decred/dcrd/pull/2939))
- rpcserver: Decouple agenda info status strings ([decred/dcrd#2939](https://github.com/decred/dcrd/pull/2939))
- blockchain: Avoid repeated blks in 2 weeks calc ([decred/dcrd#2945](https://github.com/decred/dcrd/pull/2945))
- chaincfg: Deprecate LatestCheckpointHeight ([decred/dcrd#2945](https://github.com/decred/dcrd/pull/2945))
- chaincfg: Deprecate Checkpoints and remove entries ([decred/dcrd#2945](https://github.com/decred/dcrd/pull/2945))
- rpcclient: Remove searchrawtransactions ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- rpc/jsonrpc/types: Remove searchrawtransactions ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- rpc/jsonrpc/types: Remove getinfo addrindex field ([decred/dcrd#2930](https://github.com/decred/dcrd/pull/2930))
- standalone: Add transaction sanity check ([decred/dcrd#2949](https://github.com/decred/dcrd/pull/2949))
- blockchain: Use standalone check tx sanity ([decred/dcrd#2950](https://github.com/decred/dcrd/pull/2950))
- main: Only use server peer accessors ([decred/dcrd#2953](https://github.com/decred/dcrd/pull/2953))
- blockchain: Remove unused difficulty error ([decred/dcrd#2951](https://github.com/decred/dcrd/pull/2951))
- blockchain: Rename error for no treasury payout ([decred/dcrd#2951](https://github.com/decred/dcrd/pull/2951))
- fullblocktests: Decouple from blockchain ([decred/dcrd#2951](https://github.com/decred/dcrd/pull/2951))
- blockchain: Add filter hash to hdr cmt data struct ([decred/dcrd#2938](https://github.com/decred/dcrd/pull/2938))
- blockchain: Avoid db for filters of unknown blocks ([decred/dcrd#2938](https://github.com/decred/dcrd/pull/2938))
- blockchain: Move package to internal ([decred/dcrd#2952](https://github.com/decred/dcrd/pull/2952))
- mempool: Remove agendas from removeOrphan ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from limitNumOrphans ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from addOrphan ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from maybeAddOrphan ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove removeOrphanDoubleSpends agendas ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from removeTransaction ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agenda from addTransaction ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agenda from pruneStakeTx ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from pruneExpiredTx ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from RemoveOrphan ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from RemoveOrphansByTag ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from RemoveTransaction ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- mempool: Remove agendas from RemoveDoubleSpends ([decred/dcrd#2954](https://github.com/decred/dcrd/pull/2954))
- blockchain: Use new uint256 for work sums ([decred/dcrd#2957](https://github.com/decred/dcrd/pull/2957))
- blockchain: Return uint256 from chain work method ([decred/dcrd#2959](https://github.com/decred/dcrd/pull/2959))
- multi: remove spend pruner ([decred/dcrd#2961](https://github.com/decred/dcrd/pull/2961))
- mempool: Remove unused insufficient priority error ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- mempool: Remove maybeAcceptTransaction rate limit ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- mempool: Remove MaybeAcceptTransaction rate limit ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- mempool: Remove ProcessTransaction rate limit ([decred/dcrd#2964](https://github.com/decred/dcrd/pull/2964))
- blockchain: Remove leftover treasury debug logging ([decred/dcrd#2966](https://github.com/decred/dcrd/pull/2966))
- rpcserver: Avoid error in handleRebroadcastWinners ([decred/dcrd#2968](https://github.com/decred/dcrd/pull/2968))
- secp256k1: Expose IsZeroBit on mod n scalar type ([decred/dcrd#2971](https://github.com/decred/dcrd/pull/2971))
- secp256k1: Implement direct key generation ([decred/dcrd#2971](https://github.com/decred/dcrd/pull/2971))
- secp256k1: Store constant on stack instead of heap ([decred/dcrd#2972](https://github.com/decred/dcrd/pull/2972))
- blockchain: Consistency pass for next req dif calc ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- mining: Copy regular txns for alternate templates ([decred/dcrd#2978](https://github.com/decred/dcrd/pull/2978))
- mempool: Delete unreachable code in tests ([decred/dcrd#2987](https://github.com/decred/dcrd/pull/2987))
- indexers: Remove unused PrevScripts from idx ntfn ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- indexers: Remove unused indexNeedsInputs ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- indexers: Remove unused NeedsInputser ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- blockchain: Remove unused PrevScripts from ntfns ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- blockchain: Remove unused prev scripts snapshots ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- indexers: Remove unused ChainQueryer.PrevScripts ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- blockchain: Remove unused stxosToScriptSource ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- indexers: Remove unused PrevScripter interface ([decred/dcrd#2989](https://github.com/decred/dcrd/pull/2989))
- dcrutil: Correct off-by-1 index check ([decred/dcrd#2991](https://github.com/decred/dcrd/pull/2991))
- blockchain: Avoid unneeded view script deep copies ([decred/dcrd#2993](https://github.com/decred/dcrd/pull/2993))
- mining: Don't fetch inputs for coinbase ([decred/dcrd#2994](https://github.com/decred/dcrd/pull/2994))
- blockchain: Remove legacy spend pruner bucket ([decred/dcrd#2998](https://github.com/decred/dcrd/pull/2998))
- blockchain: Remove unused internal spendpruner pkg ([decred/dcrd#2998](https://github.com/decred/dcrd/pull/2998))
- blockchain: Remove unused disable verify method ([decred/dcrd#2999](https://github.com/decred/dcrd/pull/2999))
- blockchain: Remove disconnected spend journal ([decred/dcrd#2997](https://github.com/decred/dcrd/pull/2997))
- blockchain: PrevScripter interface for script val ([decred/dcrd#3000](https://github.com/decred/dcrd/pull/3000))
- blockchain: Misc consistency cleanup pass ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Pre-allocate in-flight utxoview tx map ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Remove exported utxo cache SpendEntry ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Remove exported utxo cache AddEntry ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Remove unused utxo cache add entry err ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Remove tip param from utxo cache init ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Fix rare unclean utxo cache recovery ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Don't fetch trsy{base,spend} inputs ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Separate utxo cache vs view state ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Improve utxo cache spend robustness ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Split regular/stake view tx connect ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Bypass utxo cache for zero conf spends ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Update utxodb info during upgrade ([decred/dcrd#2996](https://github.com/decred/dcrd/pull/2996))
- rpc/jsonrpc/types: Deprecate mempool prio fields ([decred/dcrd#3003](https://github.com/decred/dcrd/pull/3003))
- txscript: remove obsolete requireMinimal comment ([decred/dcrd#3004](https://github.com/decred/dcrd/pull/3004))
- blockchain: Remove incorrect upgrade version check ([decred/dcrd#3010](https://github.com/decred/dcrd/pull/3010))
- server: Use the server context when connecting to peers ([decred/dcrd#3011](https://github.com/decred/dcrd/pull/3011))
- server: Make sure the peer address exists in the manager ([decred/dcrd#3013](https://github.com/decred/dcrd/pull/3013))
- mempool: Store transaction descs in pools ([decred/dcrd#3015](https://github.com/decred/dcrd/pull/3015))
- rpcserver: Use atomic for ws client disconnect ([decred/dcrd#3024](https://github.com/decred/dcrd/pull/3024))
- rpcserver: Cast JSON-RPC req method once in parse ([decred/dcrd#3025](https://github.com/decred/dcrd/pull/3025))
- rpcserver: Consistent block connected ntfn skip ([decred/dcrd#3025](https://github.com/decred/dcrd/pull/3025))
- rpcserver: Convert ws client lifecycle to context ([decred/dcrd#3025](https://github.com/decred/dcrd/pull/3025))
- rpcserver: Pass request context to handlers ([decred/dcrd#3026](https://github.com/decred/dcrd/pull/3026))
- rpcserver: Detect disconnect of ws clients ([decred/dcrd#3031](https://github.com/decred/dcrd/pull/3031))
- mempool: Accept expired prune height ([decred/dcrd#3042](https://github.com/decred/dcrd/pull/3042))
- addrmgr: break after selecting random address ([decred/dcrd#3047](https://github.com/decred/dcrd/pull/3047))
- addrmgr: set min value and optimize address chance ([decred/dcrd#3047](https://github.com/decred/dcrd/pull/3047))
- multi: Use atomic types in unexported modules ([decred/dcrd#3053](https://github.com/decred/dcrd/pull/3053))
- connmgr: Access conn req id via accessor ([decred/dcrd#3055](https://github.com/decred/dcrd/pull/3055))
- connmgr: Synchronize existing conn req ID assign ([decred/dcrd#3055](https://github.com/decred/dcrd/pull/3055))
- chaincfg: Rework init time deployment val logic ([decred/dcrd#3056](https://github.com/decred/dcrd/pull/3056))
- blockchain: Simplify threshold state determination ([decred/dcrd#3059](https://github.com/decred/dcrd/pull/3059))
- multi: Remove unused last state depl ver param ([decred/dcrd#3059](https://github.com/decred/dcrd/pull/3059))
- multi: Remove unused next state depl ver param ([decred/dcrd#3059](https://github.com/decred/dcrd/pull/3059))
- blockchain: Remove unused lookup depl ver method ([decred/dcrd#3059](https://github.com/decred/dcrd/pull/3059))
- blockchain: Remove unused db update tracking ([decred/dcrd#3065](https://github.com/decred/dcrd/pull/3065))
- blockchain: Validate deployment chain params ([decred/dcrd#3068](https://github.com/decred/dcrd/pull/3068))
- blockchain: Remove deployment checker abstraction ([decred/dcrd#3069](https://github.com/decred/dcrd/pull/3069))
- blockchain: Use iota for threshold states ([decred/dcrd#3072](https://github.com/decred/dcrd/pull/3072))
- blockchain: Reject params with mask approval bit ([decred/dcrd#3073](https://github.com/decred/dcrd/pull/3073))
- blockchain: Reject params with shared mask bits ([decred/dcrd#3077](https://github.com/decred/dcrd/pull/3077))
- blockchain: Reject params with blank choice id ([decred/dcrd#3079](https://github.com/decred/dcrd/pull/3079))
- blockchain: Make zero val threshold tuple invalid ([decred/dcrd#3080](https://github.com/decred/dcrd/pull/3080))
- secp256k1: Add GeneratePrivateKeyFromRand function ([decred/dcrd#3096](https://github.com/decred/dcrd/pull/3096))
- secp256k1: Require concerete rand for privkey gen ([decred/dcrd#3097](https://github.com/decred/dcrd/pull/3097))
- secp256k1: Update PrivKeyFromBytes comment ([decred/dcrd#3098](https://github.com/decred/dcrd/pull/3098))
- dcrutil: Remove unused block assertion ([decred/dcrd#3106](https://github.com/decred/dcrd/pull/3106))
- peer: Minor summary debug log cleanup ([decred/dcrd#3108](https://github.com/decred/dcrd/pull/3108))
- standalone: Add modified subsidy split calcs ([decred/dcrd#3092](https://github.com/decred/dcrd/pull/3092))
- blockchain: Remove unused next threshold state err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove unused last changed state err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove unused deployment state err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove unused max block size err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove unused stake diff v1 err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove unused calc next stake diff err ([decred/dcrd#3110](https://github.com/decred/dcrd/pull/3110))
- blockchain: Remove deprecated subscribers method ([decred/dcrd#3113](https://github.com/decred/dcrd/pull/3113))
- multi: Remove unused tip generation error ([decred/dcrd#3112](https://github.com/decred/dcrd/pull/3112))
- blockchain: Correct total subsidy database entry ([decred/dcrd#3112](https://github.com/decred/dcrd/pull/3112))
- wire: Add PowHashV2 using blake3 ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- rpcserver: Consolidate work data serialization ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- standalone: Add separate proof of work hash check ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- chaincfg: Use consts for pow limit bits ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- standalone: Add ASERT difficulty calculation ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))

### Developer-related module management:

- multi: Update module requirements to go1.13 ([decred/dcrd#2891](https://github.com/decred/dcrd/pull/2891))
- main: Update to use new module versions ([decred/dcrd#2892](https://github.com/decred/dcrd/pull/2892))
- blockchain: Start v5 module dev cycle ([decred/dcrd#2903](https://github.com/decred/dcrd/pull/2903))
- multi: Support module graph prune and lazy load ([decred/dcrd#2905](https://github.com/decred/dcrd/pull/2905))
- rpc/jsonrpc/types: Start v4 module dev cycle ([decred/dcrd#2910](https://github.com/decred/dcrd/pull/2910))
- rpcclient: Start v8 module dev cycle ([decred/dcrd#2909](https://github.com/decred/dcrd/pull/2909))
- rpcserver: Bump version to 8.0.0 ([decred/dcrd#2911](https://github.com/decred/dcrd/pull/2911))
- gcs: Start v4 module dev cycle ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- blockchain/stake: Start v5 module dev cycle ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- fullblocktests: Update readme module path ([decred/dcrd#2951](https://github.com/decred/dcrd/pull/2951))
- main: Update to use latest sys module ([decred/dcrd#3049](https://github.com/decred/dcrd/pull/3049))
- main: Update to use latest term module ([decred/dcrd#3050](https://github.com/decred/dcrd/pull/3050))
- apbf: Prepare v1.0.1 ([decred/dcrd#3061](https://github.com/decred/dcrd/pull/3061))
- chaincfg/chainhash: Prepare v1.0.4 ([decred/dcrd#3094](https://github.com/decred/dcrd/pull/3094))
- secp256k1: Prepare v4.2.0 ([decred/dcrd#3101](https://github.com/decred/dcrd/pull/3101))
- dcrjson: Prepare v4.0.1 ([decred/dcrd#3102](https://github.com/decred/dcrd/pull/3102))
- rpc/jsonrpc/types: Prepare v4.0.0 ([decred/dcrd#3103](https://github.com/decred/dcrd/pull/3103))
- wire: Prepare v1.6.0 ([decred/dcrd#3119](https://github.com/decred/dcrd/pull/3119))
- blockchain/standalone: Prepare v2.2.0 ([decred/dcrd#3120](https://github.com/decred/dcrd/pull/3120))
- addrmgr: Prepare v2.0.2 ([decred/dcrd#3121](https://github.com/decred/dcrd/pull/3121))
- chaincfg: Prepare v3.2.0 ([decred/dcrd#3125](https://github.com/decred/dcrd/pull/3125))
- connmgr: Prepare v3.1.1 ([decred/dcrd#3124](https://github.com/decred/dcrd/pull/3124))
- txscript: Prepare v4.1.0 ([decred/dcrd#3126](https://github.com/decred/dcrd/pull/3126))
- hdkeychain: Prepare v3.1.1 ([decred/dcrd#3127](https://github.com/decred/dcrd/pull/3127))
- peer: Prepare v3.0.2 ([decred/dcrd#3128](https://github.com/decred/dcrd/pull/3128))
- dcrutil: Prepare v4.0.1 ([decred/dcrd#3129](https://github.com/decred/dcrd/pull/3129))
- database: Prepare v3.0.1 ([decred/dcrd#3130](https://github.com/decred/dcrd/pull/3130))
- blockchain/stake: Prepare v5.0.0 ([decred/dcrd#3131](https://github.com/decred/dcrd/pull/3131))
- gcs: Prepare v4.0.0 ([decred/dcrd#3132](https://github.com/decred/dcrd/pull/3132))
- blockchain: Prepare v5.0.0 ([decred/dcrd#3133](https://github.com/decred/dcrd/pull/3133))
- rpcclient: Prepare v8.0.0 ([decred/dcrd#3134](https://github.com/decred/dcrd/pull/3134))
- main: Update to use all new module versions ([decred/dcrd#3138](https://github.com/decred/dcrd/pull/3138))
- main: Remove module replacements ([decred/dcrd#3139](https://github.com/decred/dcrd/pull/3139))

### Testing and Quality Assurance:

- dcrutil: Rework WIF tests ([decred/dcrd#2860](https://github.com/decred/dcrd/pull/2860))
- build: update golangci-lint to v1.44.1 ([decred/dcrd#2882](https://github.com/decred/dcrd/pull/2882))
- build: Set token permissions for go.yml ([decred/dcrd#2896](https://github.com/decred/dcrd/pull/2896))
- secp256k1: Benchmark consistency pass ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Consolidate Jacobian group chk in tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Consolidate affine group check in tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup and move affine addition tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup and move affine double tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup and move key generation tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup and move affine scalar mul tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup affine scalar base mult tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup and move base mult rand tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup private key tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup Jacobian addition tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Cleanup Jacobian double tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Add test gen for random mod n scalars ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Add Jacobian scalar base mult tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- secp256k1: Rework Jacobian rand scalar mult tests ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- build: Replace deprecated terminal dep ([decred/dcrd#2894](https://github.com/decred/dcrd/pull/2894))
- rpctest: Pass and use context versus channel ([decred/dcrd#2895](https://github.com/decred/dcrd/pull/2895))
- secp256k1: Add benchmark for splitK ([decred/dcrd#2888](https://github.com/decred/dcrd/pull/2888))
- build: Use recommended golangci-lint installer ([decred/dcrd#2899](https://github.com/decred/dcrd/pull/2899))
- build: Update to latest action versions ([decred/dcrd#2900](https://github.com/decred/dcrd/pull/2900))
- blockchain: Use TempDir to create temp test dirs ([decred/dcrd#2902](https://github.com/decred/dcrd/pull/2902))
- secp256k1/ecdsa: Correct test comment ([decred/dcrd#2908](https://github.com/decred/dcrd/pull/2908))
- secp256k1/ecdsa: Add sign and verify tests ([decred/dcrd#2908](https://github.com/decred/dcrd/pull/2908))
- secp256k1/ecdsa: Add rand sign and verify tests ([decred/dcrd#2908](https://github.com/decred/dcrd/pull/2908))
- secp256k1/ecdsa: Add compact signature tests ([decred/dcrd#2915](https://github.com/decred/dcrd/pull/2915))
- secp256k1/ecdsa: Rework rand compact sig tests ([decred/dcrd#2915](https://github.com/decred/dcrd/pull/2915))
- ecdsa: Fix test that randomly picks a component ([decred/dcrd#2919](https://github.com/decred/dcrd/pull/2919))
- chaingen: Update for deprecated subsidy params ([decred/dcrd#2928](https://github.com/decred/dcrd/pull/2928))
- stake: Set test vote transactions as version 1 ([decred/dcrd#2922](https://github.com/decred/dcrd/pull/2922))
- mempool: Use valid tx fees in test harness ([decred/dcrd#2962](https://github.com/decred/dcrd/pull/2962))
- secp256k1: Add benchmark for private key gen ([decred/dcrd#2971](https://github.com/decred/dcrd/pull/2971))
- build: Update to latest action versions ([decred/dcrd#2981](https://github.com/decred/dcrd/pull/2981))
- build: Update golangci-lint to v1.48.0 ([decred/dcrd#2981](https://github.com/decred/dcrd/pull/2981))
- build: Test against Go 1.19 ([decred/dcrd#2981](https://github.com/decred/dcrd/pull/2981))
- blockchain: Make longer running tests parallel ([decred/dcrd#2988](https://github.com/decred/dcrd/pull/2988))
- blockchain: Allow tests to override cache flushing ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Improve utxo cache initialize tests ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Consolidate utxo cache test entries ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Rework utxo cache spend entry tests ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Rework utxo cache commit tests ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- blockchain: Rework utxo cache add entry tests ([decred/dcrd#2995](https://github.com/decred/dcrd/pull/2995))
- build: Update to golangci-lint v1.50.0 ([decred/dcrd#3011](https://github.com/decred/dcrd/pull/3011))
- rpctest:  Use context provided by the user ([decred/dcrd#3012](https://github.com/decred/dcrd/pull/3012))
- build: Enable run_tests.sh to work with go.work ([decred/dcrd#3021](https://github.com/decred/dcrd/pull/3021))
- build: Only invoke tests once ([decred/dcrd#3023](https://github.com/decred/dcrd/pull/3023))
- build: Rename root pkg path vars ([decred/dcrd#3022](https://github.com/decred/dcrd/pull/3022))
- multi: Move rpctest-based tests to own package ([decred/dcrd#3028](https://github.com/decred/dcrd/pull/3028))
- rpctests: Switch to dcrtest/dcrdtest package ([decred/dcrd#3028](https://github.com/decred/dcrd/pull/3028))
- rpctest: Remove package ([decred/dcrd#3028](https://github.com/decred/dcrd/pull/3028))
- rpcserver: Add limited methods exist test ([decred/dcrd#3033](https://github.com/decred/dcrd/pull/3033))
- rpctests: Build constraint for util too ([decred/dcrd#3037](https://github.com/decred/dcrd/pull/3037))
- hdkeychain: Use errors for test compare ([decred/dcrd#3038](https://github.com/decred/dcrd/pull/3038))
- build: Update to latest action versions ([decred/dcrd#3052](https://github.com/decred/dcrd/pull/3052))
- build: Update golangci-lint to v1.51.1 ([decred/dcrd#3052](https://github.com/decred/dcrd/pull/3052))
- build: Test against Go 1.20 ([decred/dcrd#3052](https://github.com/decred/dcrd/pull/3052))
- chaincfg: Rework deployment validation tests ([decred/dcrd#3056](https://github.com/decred/dcrd/pull/3056))
- blockchain: Rework vote tests ([decred/dcrd#3075](https://github.com/decred/dcrd/pull/3075))
- blockchain: Convert several test helpers ([decred/dcrd#3076](https://github.com/decred/dcrd/pull/3076))
- blockchain: Use local mock votes in tests ([decred/dcrd#3076](https://github.com/decred/dcrd/pull/3076))
- blockchain: Reassign non-overlapping test params ([decred/dcrd#3077](https://github.com/decred/dcrd/pull/3077))
- build: move golangci flags to its own config file ([decred/dcrd#3081](https://github.com/decred/dcrd/pull/3081))
- secp256k1: Add GeneratePrivateKeyFromRand tests ([decred/dcrd#3100](https://github.com/decred/dcrd/pull/3100))
- rpcserver: Use solved mock orphan block in tests ([decred/dcrd#3104](https://github.com/decred/dcrd/pull/3104))
- rpcserver: Consolidate mock mining addr ([decred/dcrd#3105](https://github.com/decred/dcrd/pull/3105))
- blockchain: Agenda test consistency pass ([decred/dcrd#3107](https://github.com/decred/dcrd/pull/3107))
- build: golangci-lint v1.53.1 ([decred/dcrd#3117](https://github.com/decred/dcrd/pull/3117))
- chaingen: Move mining solve to generate state ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- chaingen: Add blake3 support ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- chaingen: Add ASERT difficulty algorithm support ([decred/dcrd#3115](https://github.com/decred/dcrd/pull/3115))
- rpctest: Enable treasury integration test ([decred/dcrd#3118](https://github.com/decred/dcrd/pull/3118))

### Misc:

- release: Bump for 1.8 release cycle ([decred/dcrd#2854](https://github.com/decred/dcrd/pull/2854))
- secp256k1: Correct several comments ([decred/dcrd#2887](https://github.com/decred/dcrd/pull/2887))
- main: Update .gitignore for Go1.18 ([decred/dcrd#2893](https://github.com/decred/dcrd/pull/2893))
- multi: Update Go versions in README.md and .github/workflows/go.yml ([decred/dcrd#2906](https://github.com/decred/dcrd/pull/2906))
- multi: Fix a few typos ([decred/dcrd#2923](https://github.com/decred/dcrd/pull/2923))
- blockchain: Address some linter complaints ([decred/dcrd#2927](https://github.com/decred/dcrd/pull/2927))
- dcrjson: Address some linter complaints ([decred/dcrd#2927](https://github.com/decred/dcrd/pull/2927))
- connmgr: Address some linter complaints ([decred/dcrd#2927](https://github.com/decred/dcrd/pull/2927))
- blockchain/stake: Address some linter complaints ([decred/dcrd#2927](https://github.com/decred/dcrd/pull/2927))
- multi: Ensure newline at end of file ([decred/dcrd#2941](https://github.com/decred/dcrd/pull/2941))
- blockchain: Correct a few error comment typos ([decred/dcrd#2951](https://github.com/decred/dcrd/pull/2951))
- blockchain: Correct some db cfilterv2 comments ([decred/dcrd#2938](https://github.com/decred/dcrd/pull/2938))
- blockchain: Address some linter complaints ([decred/dcrd#2965](https://github.com/decred/dcrd/pull/2965))
- rpcserver: Address unused param linter complaints ([decred/dcrd#2970](https://github.com/decred/dcrd/pull/2970))
- multi: Go 1.19 doc comment formatting ([decred/dcrd#2976](https://github.com/decred/dcrd/pull/2976))
- schnorr: Go 1.19 doc comment formatting ([decred/dcrd#2981](https://github.com/decred/dcrd/pull/2981))
- main: Remove old style build constraints ([decred/dcrd#3036](https://github.com/decred/dcrd/pull/3036))
- secp256k1: Fix typo in a doc comment ([decred/dcrd#3091](https://github.com/decred/dcrd/pull/3091))
- release: Bump for 1.8.0 ([decred/dcrd#3140](https://github.com/decred/dcrd/pull/3140))

### Code Contributors (alphabetical order):

- Abirdcfly
- Dave Collins
- David Hill
- Donald Adu-Poku
- Eng Zer Jun
- Jamie Holdstock
- JoeGruff
- Jonathan Chappelow
- Josh Rickmar
- Julian Y
- Matheus Degiovani
- Ryan Staudt
- Sef Boukenken
- arjundashrath
- matthawkins90
- norwnd
- peterzen
- 刘昆

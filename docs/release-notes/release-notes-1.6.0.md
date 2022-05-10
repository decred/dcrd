# dcrd v1.6.0

This release of dcrd introduces a large number of updates.  Some of the key
highlights are:

* A new consensus vote agenda which allows the stakeholders to decide whether or
  not to activate support for a decentralized treasury
* Aggregate fee transaction selection in block templates (Child Pays For Parent)
* Improved peer discovery via HTTPS seeding with filtering capabilities
* Major performance enhancements for signature validation and other
  cryptographic operations
* Approximately 15% less overall resident memory usage
* Proactive signature cache eviction
* Improved support for single-party Schnorr signatures
* Ticket exhaustion prevention
* Various updates to the RPC server such as:
  * A new method to retrieve the current treasury balance
  * A new method to query treasury spend transaction vote details
* Infrastructure improvements
* Quality assurance changes

For those unfamiliar with the
[voting process](https://docs.decred.org/governance/consensus-rule-voting/overview/)
in Decred, all code needed in order to support a decentralized treasury is
already included in this release, however it will remain dormant until the
stakeholders vote to activate it.

For reference, the decentralized treasury work was originally proposed and
approved for initial implementation via the following Politeia proposal:
- [Decentralized Treasury Consensus Change](https://proposals.decred.org/proposals/c96290a2478d0a1916284438ea2c59a1215fe768a87648d04d45f6b7ecb82c3f)

The following Decred Change Proposal (DCP) describes the proposed changes in
detail and provides a full technical specification:
- [DCP0006](https://github.com/decred/dcps/blob/master/dcp-0006/dcp-0006.mediawiki)

**It is important for everyone to upgrade their software to this latest release
even if you don't intend to vote in favor of the agenda.**

## Downgrade Warning

The database format in v1.6.0 is not compatible with previous versions of the
software.  This only affects downgrades as users upgrading from previous
versions will see a one time database migration.

Once this migration has been completed, it will no longer be possible to
downgrade to a previous version of the software without having to delete the
database and redownload the chain.

The database migration typically takes about 5 to 10 minutes on HDDs and 2 to 4
minutes on SSDs.

## Notable Changes

### Decentralized Treasury Vote

A new vote with the id `treasury` is now available as of this release.  After
upgrading, stakeholders may set their preferences through their wallet or Voting
Service Provider's (VSP) website.

The primary goal of this change is to fully decentralize treasury spending so
that it is controlled by the stakeholders via ticket voting.

See the initial
[Politeia proposal](https://proposals.decred.org/proposals/c96290a2478d0a1916284438ea2c59a1215fe768a87648d04d45f6b7ecb82c3f)
for more details.

### Aggregate Fee Block Template Transaction Selection (Child Pays For Parent)

The transactions that are selected for inclusion in block templates that
Proof-of-Work miners solve now prioritize the overall fees of the entire
transaction ancestor graph.

This is beneficial for both miners and end users as it:

- Helps maximize miner profit by ensuring that unconfirmed transaction chains
  with higher aggregate fees are given priority over others with lower aggregate
  fees
- Provides a mechanism for users to increase the priority of an unconfirmed
  transaction by spending its outputs with another transaction that pays higher
  fees

This is commonly referred to as Child Pays For Parent (CPFP) as the spending
("child") transaction is able to increase the priority of the spent ("parent")
transaction.

### HTTPS Seeding

The initial bootstrap process that contacts seeders to discover other nodes to
connect to now uses a REST-based API over HTTPS.

This change will be imperceptible for most users, with the exception that it
accelerates the process of finding suitable candidate nodes that support desired
services, particularly in the case of recently-introduced services that have not
yet achieved widespread adoption on the network.

The following are some key benefits of HTTPS seeders over the previous DNS-based
seeders:

- Support for non-standard ports
- Advertisement of supported service
- Better scalability both in terms of network load and new features
- Native support for TLS-secured communication channels
- Native support for proxies which allows the use of anonymous overlay networks
  such as Tor and I2P
- No need for a large DNSSEC dependency surface
- Uses better audited infrastructure
- More secure
- Increases flexibility

### Signature Validation And Other Crypto Operation Optimizations

The underlying crypto code has been reworked to significantly improve its
execution speed and reduce the number of memory allocations.  While this has
more benefits than enumerated here, probably the most important ones for most
stakeholders are:

- Improved vote times since blocks and transactions propagate more quickly
  throughout the network
- The initial sync process is around 15% faster

### Proactive Signature Cache Eviction

Signature cache entries that are nearly guaranteed to no longer be useful are
now immediately and proactively evicted resulting in overall faster validation
during steady state operation due to fewer cache misses.

The primary purpose of the cache is to avoid double checking signatures that are
already known to be valid.

### Orphan Transaction Relay Policy Refinement

Transactions that spend outputs which are not known to nodes relaying them,
known as orphan transactions, now have the same size restrictions applied to
them as standard non-orphan transactions.

This ensures that transactions chains are not artificially hindered from
relaying regardless of the order they are received.

In order to keep memory usage of the now potentially larger orphan transactions
under control, more intelligent orphan eviction has been implemented and the
maximum number of allowed orphans before random eviction occurs has been
lowered.

These changes, in conjunction with other related changes, mean that nodes are
better about orphan transaction management and thus missing ancestors will
typically either be broadcast or mined fairly quickly resulting in fewer overall
orphans and smaller actual run-time orphan pools.

### Ticket Exhaustion Prevention

Mining templates that would lead to the chain becoming unrecoverable due to
inevitable ticket exhaustion will no longer be generated.

This is primarily aimed at the testing networks, but it could also theoretically
affect the main network in some far future if the demand for tickets were to
ever dry up for some unforeseen reason.

### New Initial State Protocol Messages (`getinitstate`/`initstate`)

This release introduces a pair of peer-to-peer protocol messages named
`getinitstate` and `initstate` which support querying one or more pieces of
information that are useful to acquire when a node first connects in a
consolidated fashion.

Some examples of the aforementioned information are the mining state as of the
current tip block and, with the introduction of the decentralized treasury, any
outstanding treasury spend transactions that are being voted on.

### Mining State Protocol Messages Deprecated (`getminings`/`minings`)

Due to the addition of the previously-described initial state peer-to-peer
protocol messages, the `getminings` and `minings` protocol messages are now
deprecated.  Use the new `getinitstate` and `initstate` messages with the
`headblocks` and `headblockvotes` state types instead.

### RPC Server Changes

The RPC server version as of this release is 6.2.0.

#### New Treasury Balance Query RPC (`gettreasurybalance`)

A new RPC named `gettreasurybalance` is now available to query the current
balance of the decentralized treasury.  Please note that this requires the
decentralized treasury vote to pass and become active, so it will return an
appropriate error indicating the decentralized treasury is inactive until that
time.

See the
[gettreasurybalance JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#gettreasurybalance)
for API details.

#### New Treasury Spend Vote Query RPC (`gettreasuryspendvotes`)

A new RPC named `gettreasuryspendvotes` is now available to query vote
information about one or more treasury spend transactions.  Please note that
this requires the decentralized treasury vote to pass and become active to
produce a meaningful result since treasury spend transactions are invalid until
that time.

See the
[gettreasuryspendvotes JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#gettreasuryspendvotes)
for API details.

#### New Force Mining Template Regeneration RPC (`regentemplate`)

A new RPC named `regentemplate` is now available which can be used to force the
current background block template to be regenerated.

See the
[regentemplate JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#regentemplate)
for API details.

#### New Unspent Transaction Output Set Query RPC (`gettxoutsetinfo`)

A new RPC named `gettxoutsetinfo` is now available which can be used to retrieve
statistics about the current global set of unspent transaction outputs (UTXOs).

See the
[gettxoutsetinfo JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#gettxoutsetinfo)
for API details.

#### Updates to Peer Information Query RPC (`getpeerinfo`)

The results of the `getpeerinfo` RPC are now sorted by the `id` field.

See the
[getpeerinfo JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getpeerinfo)
for API details.

#### Enforced Results Limit on Transaction Search RPC (`searchrawtransactions`)

The maximum number of transactions returned by a single request to the
`searchrawtransactions` RPC is now limited to 10,000 transactions.  This far
exceeds the number of results for all typical cases; however, for the rare cases
where it does not, the caller can make use of the `skip` parameter in subsequent
requests to access additional data if they require access to more results.

See the
[searchrawtransactions JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#searchrawtransactions)
for API details.

#### New Index Status Fields on Info Query RPC (`getinfo`)

The results of the `getinfo` RPC now include `txindex` and `addrindex` fields
that specify whether or not the respective indexes are active.

See the
[getinfo JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#getinfo)
for API details.

### Version 1 Block Filters Deprecated

Support for version 1 block filters is deprecated and is scheduled to be removed
in the next release.   Use
[version 2 block filters](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#version-2-block-filters)
with their associated [block header commitment](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#block-header-commitments)
and [inclusion proof](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#verifying-commitment-root-inclusion-proofs)
instead.

## Changelog

This release consists of 616 commits from 17 contributors which total to 526
files changed, 63090 additional lines of code, and 26279 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v1.5.2...release-v1.6.0).

### Protocol and network:

- chaincfg: Add checkpoints for upcoming release ([decred/dcrd#2370](https://github.com/decred/dcrd/pull/2370))
- multi: Introduce initial sync min known chain work ([decred/dcrd#2000](https://github.com/decred/dcrd/pull/2000))
- chaincfg: Update min known chain work for release ([decred/dcrd#2371](https://github.com/decred/dcrd/pull/2371))
- server: improve address discovery ([decred/dcrd#1838](https://github.com/decred/dcrd/pull/1838))
- connmgr: unexport newConnReq ([decred/dcrd#1729](https://github.com/decred/dcrd/pull/1729))
- connmgr: Add context to Dial and DialAddr ([decred/dcrd#1729](https://github.com/decred/dcrd/pull/1729))
- dcrd: adapt to new connmgr API ([decred/dcrd#1729](https://github.com/decred/dcrd/pull/1729))
- server: Simplify logic to bind listeners ([decred/dcrd#1972](https://github.com/decred/dcrd/pull/1972))
- server: Fix peer state update ([decred/dcrd#1981](https://github.com/decred/dcrd/pull/1981))
- chaincfg: introduce Seeder ([decred/dcrd#2017](https://github.com/decred/dcrd/pull/2017))
- connmgr: add SeedAddrs ([decred/dcrd#2017](https://github.com/decred/dcrd/pull/2017))
- server: seed with https versus dns ([decred/dcrd#2017](https://github.com/decred/dcrd/pull/2017))
- chaincfg: deprecate type DNSSeed and Params.DNSSeeds ([decred/dcrd#2017](https://github.com/decred/dcrd/pull/2017))
- connmgr: Allow pending outbound conn removal ([decred/dcrd#2033](https://github.com/decred/dcrd/pull/2033))
- connmgr: Cleanup pending outbound conn removal ([decred/dcrd#2033](https://github.com/decred/dcrd/pull/2033))
- connmgr: add Timeout config option ([decred/dcrd#2068](https://github.com/decred/dcrd/pull/2068))
- connmgr: Remove deprecated DisableLog func ([decred/dcrd#2187](https://github.com/decred/dcrd/pull/2187))
- connmgr: Remove deprecated SeedFromDNS func ([decred/dcrd#2187](https://github.com/decred/dcrd/pull/2187))
- connmgr: Remove deprecated TorLookupIP func ([decred/dcrd#2187](https://github.com/decred/dcrd/pull/2187))
- connmgr: Rename TorLookupIPContext to TorLookupIP ([decred/dcrd#2191](https://github.com/decred/dcrd/pull/2191))
- connmgr: Rework HTTPS seeding ([decred/dcrd#2188](https://github.com/decred/dcrd/pull/2188))
- server: ban peers on wire protocol errors ([decred/dcrd#2110](https://github.com/decred/dcrd/pull/2110))
- dcrd: use a context w/ timeout when fetching seeds ([decred/dcrd#2337](https://github.com/decred/dcrd/pull/2337))
- multi: Add decentralized treasury support ([decred/dcrd#2170](https://github.com/decred/dcrd/pull/2170))
- wire: Introduce InitState messages ([decred/dcrd#2349](https://github.com/decred/dcrd/pull/2349))
- peer: Handle InitState messages ([decred/dcrd#2349](https://github.com/decred/dcrd/pull/2349))
- server: Send and respond to InitState msgs ([decred/dcrd#2349](https://github.com/decred/dcrd/pull/2349))
- connmgr: limit addresses returned by seeders ([decred/dcrd#2337](https://github.com/decred/dcrd/pull/2337))
- connmgr: Enforce max http seeder response size ([decred/dcrd#2338](https://github.com/decred/dcrd/pull/2338))
- chaincfg: Make simnet votes standard txs ([decred/dcrd#2348](https://github.com/decred/dcrd/pull/2348))
- server: Check whitelist before ban on read errs ([decred/dcrd#2362](https://github.com/decred/dcrd/pull/2362))
- server: Consolidate ban disable/whitelist logic ([decred/dcrd#2363](https://github.com/decred/dcrd/pull/2363))
- blockmanager: handle notfound messages from peers ([decred/dcrd#2253](https://github.com/decred/dcrd/pull/2253))
- blockmanager: limit the requested maps ([decred/dcrd#2253](https://github.com/decred/dcrd/pull/2253))
- server: increase ban score for notfound messages ([decred/dcrd#2253](https://github.com/decred/dcrd/pull/2253))
- server: return whether addBanScore disconnected the peer ([decred/dcrd#2253](https://github.com/decred/dcrd/pull/2253))
- blockchain: Whitelist DCP0005 violations ([decred/dcrd#2533](https://github.com/decred/dcrd/pull/2533))

### Transaction relay (memory pool):

- mempool: Implement orphan expiration ([decred/dcrd#1974](https://github.com/decred/dcrd/pull/1974))
- mempool: Associated tag with orphan txns ([decred/dcrd#1982](https://github.com/decred/dcrd/pull/1982))
- mempool: Expose RemoveOrphansByTag function ([decred/dcrd#1982](https://github.com/decred/dcrd/pull/1982))
- server/mempool: Evict orphans on peer disconnect ([decred/dcrd#1982](https://github.com/decred/dcrd/pull/1982))
- mempool: Modify default orphan tx policy ([decred/dcrd#1984](https://github.com/decred/dcrd/pull/1984))
- mempool: Tighten allowed votes range for mainnet ([decred/dcrd#2047](https://github.com/decred/dcrd/pull/2047))
- multi: Track tickets with non-approved inputs ([decred/dcrd#1852](https://github.com/decred/dcrd/pull/1852))
- mempool: Remove deprecated ErrToRejectErr func ([decred/dcrd#2273](https://github.com/decred/dcrd/pull/2273))
- mempool: Remove deprecated tx rule err reject code ([decred/dcrd#2273](https://github.com/decred/dcrd/pull/2273))
- mempool: Track tspends separately ([decred/dcrd#2350](https://github.com/decred/dcrd/pull/2350))
- mempool: Special case tspends for insertion ([decred/dcrd#2350](https://github.com/decred/dcrd/pull/2350))
- mempool: Fix wrong tx type in error message ([decred/dcrd#2350](https://github.com/decred/dcrd/pull/2350))
- dcrd: trickle mempool response to peer ([decred/dcrd#2359](https://github.com/decred/dcrd/pull/2359))
- mempool: Allow treasury txn vers as standard ([decred/dcrd#2412](https://github.com/decred/dcrd/pull/2412))
- mempool: Limit ancestor tracking in mempool ([decred/dcrd#2468](https://github.com/decred/dcrd/pull/2468))

### Mining:

- mining: Introduce PriorityInputser interface ([decred/dcrd#1966](https://github.com/decred/dcrd/pull/1966))
- mining: Correct priority calcs for Decred sizes ([decred/dcrd#1967](https://github.com/decred/dcrd/pull/1967))
- cpuminer: convert from a quit channel to a context ([decred/dcrd#1978](https://github.com/decred/dcrd/pull/1978))
- mining: Prevent potential shutdown hang ([decred/dcrd#2196](https://github.com/decred/dcrd/pull/2196))
- mining: Improve comment for UpdateBlockTime ([decred/dcrd#2276](https://github.com/decred/dcrd/pull/2276))
- cpuminer: Refactor code to its own package ([decred/dcrd#2276](https://github.com/decred/dcrd/pull/2276))
- cpuminer: Rework to use bg template generator ([decred/dcrd#2277](https://github.com/decred/dcrd/pull/2277))
- cpuminer: Improve already discrete mining error ([decred/dcrd#2341](https://github.com/decred/dcrd/pull/2341))
- mining: Remove unneeded disapproval check ([decred/dcrd#2397](https://github.com/decred/dcrd/pull/2397))
- mining: Add ticket exhaustion check ([decred/dcrd#2398](https://github.com/decred/dcrd/pull/2398))
- mempool/mining: Implement aggregate fee sorting ([decred/dcrd#1829](https://github.com/decred/dcrd/pull/1829))
- multi: Decouple blockManager from mining ([decred/dcrd#1965](https://github.com/decred/dcrd/pull/1965))
- multi: Hide CPUMiner WaitGroup ([decred/dcrd#1965](https://github.com/decred/dcrd/pull/1965))
- multi: Move mining code into mining package ([decred/dcrd#1965](https://github.com/decred/dcrd/pull/1965))
- mining: Remove unused methods ([decred/dcrd#2419](https://github.com/decred/dcrd/pull/2419))
- mining: Update to latest block vers for trsy vote ([decred/dcrd#2402](https://github.com/decred/dcrd/pull/2402))
- multi: add rpcserver.CPUMiner ([decred/dcrd#2286](https://github.com/decred/dcrd/pull/2286))
- mining: Prevent panic in child prio item handling ([decred/dcrd#2435](https://github.com/decred/dcrd/pull/2435))

### RPC:

- rpcserver: decouple from server ([decred/dcrd#1730](https://github.com/decred/dcrd/pull/1730))
- rpcserver: refactor listener logic to server ([decred/dcrd#1734](https://github.com/decred/dcrd/pull/1734))
- rpcserver: Start separate internal package impl ([decred/dcrd#1954](https://github.com/decred/dcrd/pull/1954))
- rpcserver: Move rpc connmgr iface to internal pkg ([decred/dcrd#1954](https://github.com/decred/dcrd/pull/1954))
- rpcserver: Move rpc syncmgr iface to internal pkg ([decred/dcrd#1954](https://github.com/decred/dcrd/pull/1954))
- rpcserver: Add logging to internal package ([decred/dcrd#1954](https://github.com/decred/dcrd/pull/1954))
- rpcserver: Add basic initial package documentation ([decred/dcrd#1954](https://github.com/decred/dcrd/pull/1954))
- rpcserver: Cleanup getvoteinfo RPC ([decred/dcrd#1964](https://github.com/decred/dcrd/pull/1964))
- rpcclient: add automatic pinging ([decred/dcrd#1898](https://github.com/decred/dcrd/pull/1898))
- rpcserver: Bump to 6.1.1 ([decred/dcrd#1970](https://github.com/decred/dcrd/pull/1970))
- rpcserver: Warn on alt DNS names when certs exist ([decred/dcrd#1971](https://github.com/decred/dcrd/pull/1971))
- rpcserver: replace close channel with context ([decred/dcrd#1976](https://github.com/decred/dcrd/pull/1976))
- websocket: attach context to inHandler ([decred/dcrd#1976](https://github.com/decred/dcrd/pull/1976))
- multi: add gettxoutsetinfo JSON-RPC ([decred/dcrd#1909](https://github.com/decred/dcrd/pull/1909))
- rpcserver: Move error check for generate RPC ([decred/dcrd#1977](https://github.com/decred/dcrd/pull/1977))
- rpcserver: add ping and pong handers ([decred/dcrd#1995](https://github.com/decred/dcrd/pull/1995))
- multi: Introduce regentemplate command ([decred/dcrd#1979](https://github.com/decred/dcrd/pull/1979))
- rpcwebsocket: Remove client from missed maps ([decred/dcrd#2027](https://github.com/decred/dcrd/pull/2027))
- rpcwebsocket: Use nonblocking messages and ntfns ([decred/dcrd#2026](https://github.com/decred/dcrd/pull/2026))
- multi: fix rpc listener error ([decred/dcrd#2065](https://github.com/decred/dcrd/pull/2065))
- rpcserver: Correctly assign TxIn amounts ([decred/dcrd#2071](https://github.com/decred/dcrd/pull/2071))
- rpcclient: use NewRequestWithContext ([decred/dcrd#2101](https://github.com/decred/dcrd/pull/2101))
- rpcclient: Resurrect validateaddress/verifymessage ([decred/dcrd#2205](https://github.com/decred/dcrd/pull/2205))
- rpcclient: Stop client on ctx done ([decred/dcrd#2198](https://github.com/decred/dcrd/pull/2198))
- rpcclient: Add a lifetime to requests ([decred/dcrd#2198](https://github.com/decred/dcrd/pull/2198))
- rpc: Add AddrIndex and TxIndex bools to getinfo ([decred/dcrd#2207](https://github.com/decred/dcrd/pull/2207))
- rpcserver: Avoid panic during hash decode ([decred/dcrd#2213](https://github.com/decred/dcrd/pull/2213))
- rpcserver: Internal err on gettxout utxo fetch err ([decred/dcrd#2214](https://github.com/decred/dcrd/pull/2214))
- rpcserver: Correct JSON-RPC request unmarshal ([decred/dcrd#2218](https://github.com/decred/dcrd/pull/2218))
- rpcserver: Limit getstakeversioninfo count ([decred/dcrd#2221](https://github.com/decred/dcrd/pull/2221))
- rpcclient: Reregister work ntfns on reconnect ([decred/dcrd#2228](https://github.com/decred/dcrd/pull/2228))
- rpcserver: Remove global config dependency ([decred/dcrd#2228](https://github.com/decred/dcrd/pull/2228))
- rpcserver: Remove server.go dependencies ([decred/dcrd#2228](https://github.com/decred/dcrd/pull/2228))
- rpcserver: Remove log config dependencies ([decred/dcrd#2228](https://github.com/decred/dcrd/pull/2228))
- rpcserver: Remove PeerNotifier dependency ([decred/dcrd#2228](https://github.com/decred/dcrd/pull/2228))
- rpcserver: Handle genesis in getblockchaininfo ([decred/dcrd#2237](https://github.com/decred/dcrd/pull/2237))
- rpcserver: Export RPC server, config, and new ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Export rpcwebsocket Notify functions ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Move genCertPair to server.go ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Rename RpcserverConfig to Config ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Rename NewRPCServer to New ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Rename RPCServer to Server ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- rpcserver: Remove math/rand init and import ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- multi: add SanityChecker interface ([decred/dcrd#2289](https://github.com/decred/dcrd/pull/2289))
- rpcserver: Use func for semver string ([decred/dcrd#2290](https://github.com/decred/dcrd/pull/2290))
- rpcserver: Use internal quit chan for ws sync ([decred/dcrd#2297](https://github.com/decred/dcrd/pull/2297))
- rpcserver: Sort getpeerinfo results by ID ([decred/dcrd#2311](https://github.com/decred/dcrd/pull/2311))
- rpcserver: Add Filterer and FiltererV2 interfaces ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- rpcserver: Add exists upper bounds TODOs ([decred/dcrd#2291](https://github.com/decred/dcrd/pull/2291))
- multi: Fix incorrect RPC comments ([decred/dcrd#2332](https://github.com/decred/dcrd/pull/2332))
- server: Remove unnecessary rpcadaptors ([decred/dcrd#2347](https://github.com/decred/dcrd/pull/2347))
- jsonrpc/types: Register rebroadcast as websocket ([decred/dcrd#2355](https://github.com/decred/dcrd/pull/2355))
- jsonrpc: Add gettreasuryspendvotes types ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- rpcclient: Add GetTreasurySpendVotes command ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- rpcserver: Add support for gettreasuryspendvotes ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- rpcserver: Forward HTTP server err msgs to logger ([decred/dcrd#2378](https://github.com/decred/dcrd/pull/2378))
- rpcserver: Add searchrawtransactions count limit ([decred/dcrd#2386](https://github.com/decred/dcrd/pull/2386))
- rpcserver: Fix race in TestHandleTSpendVotes ([decred/dcrd#2393](https://github.com/decred/dcrd/pull/2393))
- rpcserver: Correct known wallet method handling ([decred/dcrd#2416](https://github.com/decred/dcrd/pull/2416))
- rpcserver: Update known wallet RPC methods ([decred/dcrd#2416](https://github.com/decred/dcrd/pull/2416))
- multi: Add TAdd support to getrawmempool ([decred/dcrd#2448](https://github.com/decred/dcrd/pull/2448))
- config: Use the P-256 curve by default for RPC ([decred/dcrd#2459](https://github.com/decred/dcrd/pull/2459))
- rpcserver: Correct getpeerinfo for peers w/o conn ([decred/dcrd#2465](https://github.com/decred/dcrd/pull/2465))
- rpcserver: Correct treasury vote status handling ([decred/dcrd#2469](https://github.com/decred/dcrd/pull/2469))
- multi: Add tx inputs treasurybase RPC support ([decred/dcrd#2470](https://github.com/decred/dcrd/pull/2470))
- multi: Add tx inputs treasuryspend RPC support ([decred/dcrd#2472](https://github.com/decred/dcrd/pull/2472))
- rpcserver: Fix count tspend votes in mined block ([decred/dcrd#2565](https://github.com/decred/dcrd/pull/2565))

### dcrd command-line flags and configuration:

- server: Add tlscurve config parameter ([decred/dcrd#1983](https://github.com/decred/dcrd/pull/1983))
- config: Add flag to allow unsynced testnet mining ([decred/dcrd#2023](https://github.com/decred/dcrd/pull/2023))
- config: add --dialtimeout defaulting to 30 seconds ([decred/dcrd#2068](https://github.com/decred/dcrd/pull/2068))
- multi: add --peeridletimeout defaulting to 120s ([decred/dcrd#2067](https://github.com/decred/dcrd/pull/2067))

### gencerts utility changes:

- gencerts: Rewrite for additional use cases ([decred/dcrd#2425](https://github.com/decred/dcrd/pull/2425))
- gencerts: Add missing newline for unknown algorithm error ([decred/dcrd#2427](https://github.com/decred/dcrd/pull/2427))
- gencerts: Use the P-256 curve by default ([decred/dcrd#2461](https://github.com/decred/dcrd/pull/2461))

### dcrctl utility changes:

- multi: Split dcrctl to own repo and update docs ([decred/dcrd#2175](https://github.com/decred/dcrd/pull/2175))

### Documentation:

- rpcserver: Refactor and update documentation ([decred/dcrd#2066](https://github.com/decred/dcrd/pull/2066))
- multi: replace godoc.org with pkg.go.dev ([decred/dcrd#2091](https://github.com/decred/dcrd/pull/2091))
- LICENSE: update year ([decred/dcrd#2092](https://github.com/decred/dcrd/pull/2092))
- hdkeychain: Fix references to methods in package docs ([decred/dcrd#2115](https://github.com/decred/dcrd/pull/2115))
- secp256k1: Update field val docs to public facing ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- schnorr: Add README.md ([decred/dcrd#2149](https://github.com/decred/dcrd/pull/2149))
- schnorr: Add doc.go ([decred/dcrd#2149](https://github.com/decred/dcrd/pull/2149))
- ecdsa: Correct README.md documentation links ([decred/dcrd#2165](https://github.com/decred/dcrd/pull/2165))
- secp256k1: Update README.md and doc.go ([decred/dcrd#2166](https://github.com/decred/dcrd/pull/2166))
- docs: Update README.md to reflect reality ([decred/dcrd#2168](https://github.com/decred/dcrd/pull/2168))
- schnorr: Correct a couple of typos in README.md ([decred/dcrd#2169](https://github.com/decred/dcrd/pull/2169))
- docs: Clarify README.md installation guides ([decred/dcrd#2171](https://github.com/decred/dcrd/pull/2171))
- docs: Remove outdated btcd refs from README.md ([decred/dcrd#2172](https://github.com/decred/dcrd/pull/2172))
- docs: Remove stray trailing spaces in README.md ([decred/dcrd#2172](https://github.com/decred/dcrd/pull/2172))
- docs: Update Code Contribution Guidelines ([decred/dcrd#2200](https://github.com/decred/dcrd/pull/2200))
- docs: Update links to avoid redirects ([decred/dcrd#2201](https://github.com/decred/dcrd/pull/2201))
- docs: Update JSON-RPC spec link to latest ([decred/dcrd#2216](https://github.com/decred/dcrd/pull/2216))
- docs: Fix chaingen broken markdown link ([decred/dcrd#2226](https://github.com/decred/dcrd/pull/2226))
- indexers: Fix existsaddridx description ([decred/dcrd#2234](https://github.com/decred/dcrd/pull/2234))
- docs: Update for removal of mempool module ([decred/dcrd#2274](https://github.com/decred/dcrd/pull/2274))
- docs: Update for removal of mining module ([decred/dcrd#2275](https://github.com/decred/dcrd/pull/2275))
- docs: Update for removal of fees module ([decred/dcrd#2287](https://github.com/decred/dcrd/pull/2287))
- docs: Add documentation for getcfilterheader ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- rpcserver: Document v1 cfilters as deprecated ([decred/dcrd#2314](https://github.com/decred/dcrd/pull/2314))
- docs: Add several historical release notes ([decred/dcrd#2317](https://github.com/decred/dcrd/pull/2317))
- contrib: Add README.md ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- multi: Add simnet documentation and setup script ([decred/dcrd#2315](https://github.com/decred/dcrd/pull/2315))
- docs: Document additional ws notifications ([decred/dcrd#2316](https://github.com/decred/dcrd/pull/2316))
- contrib: Move service config examples to contrib ([decred/dcrd#2317](https://github.com/decred/dcrd/pull/2317))
- peer: Update README.md/doc.go to reflect reality ([decred/dcrd#2325](https://github.com/decred/dcrd/pull/2325))
- docs: Update README.md to require Go 1.14/1.15 ([decred/dcrd#2335](https://github.com/decred/dcrd/pull/2335))
- docs: Update searchrawtransactions JSON-RPC docs ([decred/dcrd#2330](https://github.com/decred/dcrd/pull/2330))
- sampleconfig: Make constant a function instead ([decred/dcrd#2340](https://github.com/decred/dcrd/pull/2340))
- docs: Add release notes for v1.5.2 ([decred/dcrd#2346](https://github.com/decred/dcrd/pull/2346))
- docs: Update rebroadcast JSON-RPC docs ([decred/dcrd#2355](https://github.com/decred/dcrd/pull/2355))
- docs: Update README CLI suite link to ref latest ([decred/dcrd#2361](https://github.com/decred/dcrd/pull/2361))
- docs: Add missing gettreasurybalance documentation ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- contrib: More restrictive dcrd service privileges ([decred/dcrd#2357](https://github.com/decred/dcrd/pull/2357))
- docs: Update for connmgr v3 module ([decred/dcrd#2376](https://github.com/decred/dcrd/pull/2376))
- docs: Update for dcrec/secp256k1/v3 module ([decred/dcrd#2377](https://github.com/decred/dcrd/pull/2377))
- docs: Update for chaincfg v3 module ([decred/dcrd#2381](https://github.com/decred/dcrd/pull/2381))
- docs: Update for dcrutil v3 module ([decred/dcrd#2383](https://github.com/decred/dcrd/pull/2383))
- docs: Update for txscript v3 module ([decred/dcrd#2384](https://github.com/decred/dcrd/pull/2384))
- docs: Update for hdkeychain v3 module ([decred/dcrd#2392](https://github.com/decred/dcrd/pull/2392))
- docs: Update for blockchain/standalone v2 module ([decred/dcrd#2395](https://github.com/decred/dcrd/pull/2395))
- docs: Update simnet env docs for ticket exhaustion ([decred/dcrd#2403](https://github.com/decred/dcrd/pull/2403))
- docs: Update JSON-RPC API examples ([decred/dcrd#2404](https://github.com/decred/dcrd/pull/2404))
- docs: Update for blockchain/stake v3 module ([decred/dcrd#2418](https://github.com/decred/dcrd/pull/2418))
- docs: Update for peer/v2 module ([decred/dcrd#2422](https://github.com/decred/dcrd/pull/2422))
- docs: Update for rpcclient/v6 module ([decred/dcrd#2423](https://github.com/decred/dcrd/pull/2423))
- docs: Update for blockchain v3 module ([decred/dcrd#2424](https://github.com/decred/dcrd/pull/2424))
- docs: Update several JSON-RPC APIs ([decred/dcrd#2470](https://github.com/decred/dcrd/pull/2470))
- docs: Update several JSON-RPC APIs ([decred/dcrd#2472](https://github.com/decred/dcrd/pull/2472))

### Developer-related package and module changes:

- blockmanager: remove serverPeer from blockmanager completely ([decred/dcrd#1735](https://github.com/decred/dcrd/pull/1735))
- txscript: Add signature type to KeyClosure API ([decred/dcrd#1961](https://github.com/decred/dcrd/pull/1961))
- server: Convert lifecycle to context ([decred/dcrd#1952](https://github.com/decred/dcrd/pull/1952))
- dcrutil: drop chainec ([decred/dcrd#1957](https://github.com/decred/dcrd/pull/1957))
- txscript: drop chainec ([decred/dcrd#1957](https://github.com/decred/dcrd/pull/1957))
- blockchain: drop chainec ([decred/dcrd#1957](https://github.com/decred/dcrd/pull/1957))
- mempool: drop chainec ([decred/dcrd#1957](https://github.com/decred/dcrd/pull/1957))
- blockchain: removed unused params ([decred/dcrd#1973](https://github.com/decred/dcrd/pull/1973))
- blockchain: Decouple indexers from blockchain ([decred/dcrd#1968](https://github.com/decred/dcrd/pull/1968))
- indexers: Use spend journal for index catchup ([decred/dcrd#1969](https://github.com/decred/dcrd/pull/1969))
- blockchain: replace scriptval quit channel with context ([decred/dcrd#1991](https://github.com/decred/dcrd/pull/1991))
- indexers: Remove unused code ([decred/dcrd#1987](https://github.com/decred/dcrd/pull/1987))
- chaincfg: Gate mustPayout with subsidy generation ([decred/dcrd#1988](https://github.com/decred/dcrd/pull/1988))
- database: Remove unused code ([decred/dcrd#1989](https://github.com/decred/dcrd/pull/1989))
- edwards: Remove unused code ([decred/dcrd#1990](https://github.com/decred/dcrd/pull/1990))
- dcrd: attach shutdown context to listeners ([decred/dcrd#1992](https://github.com/decred/dcrd/pull/1992))
- blockchain: Remove unconfigurable chain var ([decred/dcrd#1996](https://github.com/decred/dcrd/pull/1996))
- multi: remove global activeNetParams ([decred/dcrd#1999](https://github.com/decred/dcrd/pull/1999))
- lru: add kv cache ([decred/dcrd#2002](https://github.com/decred/dcrd/pull/2002))
- sampleconfig: add export dcrctl sample config ([decred/dcrd#2003](https://github.com/decred/dcrd/pull/2003))
- blockmanager: Simplify dynamic peer height updates ([decred/dcrd#1998](https://github.com/decred/dcrd/pull/1998))
- indexers: convert to contexts ([decred/dcrd#1985](https://github.com/decred/dcrd/pull/1985))
- blockchain: Rename KnownValid to HasValidated ([decred/dcrd#1997](https://github.com/decred/dcrd/pull/1997))
- blockchain: Remove unused error from HaveBlock ([decred/dcrd#2007](https://github.com/decred/dcrd/pull/2007))
- blockchain: Use skip list for ancestor traversal ([decred/dcrd#2010](https://github.com/decred/dcrd/pull/2010))
- multi: Decouple orphan handling from blockchain ([decred/dcrd#2008](https://github.com/decred/dcrd/pull/2008))
- blockchain: Remove easiest diff checkpoint checks ([decred/dcrd#2012](https://github.com/decred/dcrd/pull/2012))
- blockchain: Make checkpoints configurable ([decred/dcrd#2013](https://github.com/decred/dcrd/pull/2013))
- config: Use TorLookupIPContext ([decred/dcrd#2021](https://github.com/decred/dcrd/pull/2021))
- bech32: Ensure HRP is lowercase when encoding ([decred/dcrd#2024](https://github.com/decred/dcrd/pull/2024))
- bech32: Add base256 conversion convenience funcs ([decred/dcrd#2025](https://github.com/decred/dcrd/pull/2025))
- blockchain: Explicit hash in next work diff calcs ([decred/dcrd#2022](https://github.com/decred/dcrd/pull/2022))
- blockchain: Remove unused CalcNextRequiredDiffNode ([decred/dcrd#2022](https://github.com/decred/dcrd/pull/2022))
- blockmanager: Remove unused diff calc code ([decred/dcrd#2022](https://github.com/decred/dcrd/pull/2022))
- blockchain: Support hdr checkpoints and simplify ([decred/dcrd#2014](https://github.com/decred/dcrd/pull/2014))
- txscript: Optimize conditional execution mem usage ([decred/dcrd#2011](https://github.com/decred/dcrd/pull/2011))
- fix regenHandler shutdown ([decred/dcrd#2041](https://github.com/decred/dcrd/pull/2041))
- secp256k1: Remove unused chainec code ([decred/dcrd#2042](https://github.com/decred/dcrd/pull/2042))
- secp256k1: Consistent function formatting ([decred/dcrd#2044](https://github.com/decred/dcrd/pull/2044))
- secp256k1: Optimize NonceRFC6979 ([decred/dcrd#2044](https://github.com/decred/dcrd/pull/2044))
- secp256k1: Never fail signing ([decred/dcrd#2044](https://github.com/decred/dcrd/pull/2044))
- schnorr: Remove unused threshold code ([decred/dcrd#2045](https://github.com/decred/dcrd/pull/2045))
- rpcclient: add context ([decred/dcrd#1980](https://github.com/decred/dcrd/pull/1980))
- multi: replace GetScriptClass consensus calls ([decred/dcrd#2031](https://github.com/decred/dcrd/pull/2031))
- secp256k1: Split funcs for crypto/elliptic iface ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make params standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Rename generation related code ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Move big int to field adaptor code ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make point doubling funcs standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make point addition funcs standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Move group operations to new curve.go ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Remove unnecessary QPlus1Div4 export ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make endormophism bits standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Decouple internals from ecdsa.PublicKey ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Decouple signing from ecdsa.PrivateKey ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make k splitting func standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make k mod reduce func standalone ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Move naf func to curve file ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Refactor isOnCurve logic from adaptor ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Refactor scalar mult logic from adaptor ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make private key independent type ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Make public key independent type ([decred/dcrd#2056](https://github.com/decred/dcrd/pull/2056))
- secp256k1: Introduce jacobian point struct ([decred/dcrd#2057](https://github.com/decred/dcrd/pull/2057))
- secp256k1: Implement direct signature verification ([decred/dcrd#2058](https://github.com/decred/dcrd/pull/2058))
- secp256k1: Add specialized field check for one ([decred/dcrd#2059](https://github.com/decred/dcrd/pull/2059))
- multi: Convert rpcserver lifecycle to context ([decred/dcrd#2043](https://github.com/decred/dcrd/pull/2043))
- txscript: Don't use GetScriptClass in consensus ([decred/dcrd#2070](https://github.com/decred/dcrd/pull/2070))
- txscript: Remove unused isStakeOutput function ([decred/dcrd#2070](https://github.com/decred/dcrd/pull/2070))
- multi:  define wire error types ([decred/dcrd#2055](https://github.com/decred/dcrd/pull/2055))
- hdkeychain: Provide SerializedPubKey method ([decred/dcrd#2073](https://github.com/decred/dcrd/pull/2073))
- dcrutil: Provide privkey access for WIFs ([decred/dcrd#2078](https://github.com/decred/dcrd/pull/2078))
- hdkeychain: Remove ECPubKey ([decred/dcrd#2080](https://github.com/decred/dcrd/pull/2080))
- dcrutil: Use intended method names ([decred/dcrd#2079](https://github.com/decred/dcrd/pull/2079))
- hdkeychain: ECPrivKey -> SerializedPrivKey ([decred/dcrd#2081](https://github.com/decred/dcrd/pull/2081))
- hdkeychain: Use direct hashes and remove dcrutil dep ([decred/dcrd#2086](https://github.com/decred/dcrd/pull/2086))
- stake: Remove exported FindTicketIdxs ([decred/dcrd#2089](https://github.com/decred/dcrd/pull/2089))
- secp256k1: Add fixed-precision group order type ([decred/dcrd#2060](https://github.com/decred/dcrd/pull/2060))
- secp256k1: Make private key opaque ([decred/dcrd#2061](https://github.com/decred/dcrd/pull/2061))
- secp256k1: Follow RFC6979 for too large nonce data ([decred/dcrd#2062](https://github.com/decred/dcrd/pull/2062))
- secp256k1: Return new scalar from NonceRFC6979 ([decred/dcrd#2063](https://github.com/decred/dcrd/pull/2063))
- secp256k1: Use new mod n scalar in ec mults ([decred/dcrd#2064](https://github.com/decred/dcrd/pull/2064))
- secp256k1: Add non-const inverse for mod n scalar ([decred/dcrd#2072](https://github.com/decred/dcrd/pull/2072))
- secp256k1: Optimize sig verify with mod n scalar ([decred/dcrd#2083](https://github.com/decred/dcrd/pull/2083))
- secp256k1: Make signature opaque ([decred/dcrd#2084](https://github.com/decred/dcrd/pull/2084))
- secp256k1: Use mod n scalar when signing ([decred/dcrd#2085](https://github.com/decred/dcrd/pull/2085))
- secp256k1: Use mod n scalar in sig serialization ([decred/dcrd#2087](https://github.com/decred/dcrd/pull/2087))
- secp256k1: Add optimized sqrt field calc ([decred/dcrd#2088](https://github.com/decred/dcrd/pull/2088))
- secp256k1: Add field func to determine when >= P-N ([decred/dcrd#2093](https://github.com/decred/dcrd/pull/2093))
- secp256k1: Use field val for y coord decompression ([decred/dcrd#2094](https://github.com/decred/dcrd/pull/2094))
- secp256k1: Return num instead of bool for overflow ([decred/dcrd#2095](https://github.com/decred/dcrd/pull/2095))
- secp256k1: Overhaul compact signatures ([decred/dcrd#2095](https://github.com/decred/dcrd/pull/2095))
- schnorr: Zero internal bytes of big ints ([decred/dcrd#2103](https://github.com/decred/dcrd/pull/2103))
- edwards: Zero internal bytes of big ints ([decred/dcrd#2104](https://github.com/decred/dcrd/pull/2104))
- secp256k1: Remove BER signature parsing ([decred/dcrd#2105](https://github.com/decred/dcrd/pull/2105))
- secp256k1: Rework DER signature parsing code ([decred/dcrd#2106](https://github.com/decred/dcrd/pull/2106))
- connmgr: Fix dynamic ban score stringer deadlock ([decred/dcrd#2114](https://github.com/decred/dcrd/pull/2114))
- secp256k1: Use mod n scalar in signature type ([decred/dcrd#2107](https://github.com/decred/dcrd/pull/2107))
- secp256k1: Make public keys opaque ([decred/dcrd#2108](https://github.com/decred/dcrd/pull/2108))
- main: Use errors api and require go 1.13+ ([decred/dcrd#2096](https://github.com/decred/dcrd/pull/2096))
- stake: Use errors api and require go 1.13 ([decred/dcrd#2097](https://github.com/decred/dcrd/pull/2097))
- blockchain: Use errors api and require go 1.13+ ([decred/dcrd#2098](https://github.com/decred/dcrd/pull/2098))
- hdkeychain: Remove Neuter error return ([decred/dcrd#2116](https://github.com/decred/dcrd/pull/2116))
- secp256k1: Add Zero method to private key ([decred/dcrd#2117](https://github.com/decred/dcrd/pull/2117))
- schnorr: Remove unused pubkey recovery bits ([decred/dcrd#2120](https://github.com/decred/dcrd/pull/2120))
- schnorr: Remove deprecated chainec methods ([decred/dcrd#2122](https://github.com/decred/dcrd/pull/2122))
- schnorr: Remove GetCode method from Error type ([decred/dcrd#2123](https://github.com/decred/dcrd/pull/2123))
- schnorr: Remove generalized Verify ([decred/dcrd#2124](https://github.com/decred/dcrd/pull/2124))
- schnorr: Make signature opaque ([decred/dcrd#2125](https://github.com/decred/dcrd/pull/2125))
- schnorr: Move sig code to signature files ([decred/dcrd#2127](https://github.com/decred/dcrd/pull/2127))
- schnorr: Remove unused internal signing params ([decred/dcrd#2121](https://github.com/decred/dcrd/pull/2121))
- schnorr: Accept sig type in internal verify func ([decred/dcrd#2129](https://github.com/decred/dcrd/pull/2129))
- schnorr: Remove internal hash func callback ([decred/dcrd#2130](https://github.com/decred/dcrd/pull/2130))
- secp256k1: Reduce privkey copies ([decred/dcrd#2131](https://github.com/decred/dcrd/pull/2131))
- schnorr: Remove unused GenerateKey ([decred/dcrd#2132](https://github.com/decred/dcrd/pull/2132))
- mempool: Correct MaybeAcceptDependents mutex ([decred/dcrd#2135](https://github.com/decred/dcrd/pull/2135))
- secp256k1: Avoid inversion in sig verify ([decred/dcrd#2118](https://github.com/decred/dcrd/pull/2118))
- secp256k1: Reduce EC operation normalizes ([decred/dcrd#2119](https://github.com/decred/dcrd/pull/2119))
- secp256k1: Remove unused q curve param ([decred/dcrd#2136](https://github.com/decred/dcrd/pull/2136))
- secp256k1: Improve exported curve params ([decred/dcrd#2137](https://github.com/decred/dcrd/pull/2137))
- secp256k1: Make field value set int take uint16 ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Make field value add int take uint16 ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Make field value mul int take uint8 ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Make field set byte slice const time ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Export field value type ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Expose IsOddBit on field val type ([decred/dcrd#2138](https://github.com/decred/dcrd/pull/2138))
- secp256k1: Expose IsOneBit on field val type ([decred/dcrd#2138](https://github.com/decred/dcrd/pull/2138))
- secp256k1: Expose IsZeroBit on field val type ([decred/dcrd#2138](https://github.com/decred/dcrd/pull/2138))
- schnorr: Remove internal verify func bool ret ([decred/dcrd#2142](https://github.com/decred/dcrd/pull/2142))
- secp256k1: Export JacobianPoint type ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export AddNonConst ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export DoubleNonConst ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export SclarMultNonConst ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export ScalarBaseMultNonConst ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export DecompressY ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Add AsJacobian method to pubkey ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Export scalar from PrivateKey ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1: Split nonce code into separate files ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- secp256k1/ecdsa: Decouple ECDSA from secp256k1 ([decred/dcrd#2139](https://github.com/decred/dcrd/pull/2139))
- schnorr: Use extra data for RFC6979 nonces ([decred/dcrd#2143](https://github.com/decred/dcrd/pull/2143))
- schnorr: Add error support for errors.Is/As ([decred/dcrd#2145](https://github.com/decred/dcrd/pull/2145))
- hdkeychain: Use secp256k1 privkey to pubkey method ([decred/dcrd#2156](https://github.com/decred/dcrd/pull/2156))
- secp256k1: Add overflow check to field val set ([decred/dcrd#2147](https://github.com/decred/dcrd/pull/2147))
- schnorr: Rework signature parsing ([decred/dcrd#2148](https://github.com/decred/dcrd/pull/2148))
- schnorr: Remove unused copyBytes func ([decred/dcrd#2148](https://github.com/decred/dcrd/pull/2148))
- schnorr: Use specialized types when signing ([decred/dcrd#2150](https://github.com/decred/dcrd/pull/2150))
- schnorr: Optimize sig verify with specialized types ([decred/dcrd#2151](https://github.com/decred/dcrd/pull/2151))
- schnorr: Use specialized types in signature type ([decred/dcrd#2152](https://github.com/decred/dcrd/pull/2152))
- schnorr: Remove unused error codes ([decred/dcrd#2153](https://github.com/decred/dcrd/pull/2153))
- schnorr: Rename error codes to better match reality ([decred/dcrd#2153](https://github.com/decred/dcrd/pull/2153))
- secp256k1: Add PutBytesUnchecked to FieldVal ([decred/dcrd#2154](https://github.com/decred/dcrd/pull/2154))
- secp256k1: Add PutBytesUnchecked to ModNScalar ([decred/dcrd#2154](https://github.com/decred/dcrd/pull/2154))
- hdkeychain: Use specialized secp256k1 types ([decred/dcrd#2157](https://github.com/decred/dcrd/pull/2157))
- schnorr: Use PutBytesUnchecked for serialize ([decred/dcrd#2158](https://github.com/decred/dcrd/pull/2158))
- ecdsa: Use PutBytesUnchecked for serialize ([decred/dcrd#2159](https://github.com/decred/dcrd/pull/2159))
- secp256k1: Add pubkey parsing error infrastructure ([decred/dcrd#2160](https://github.com/decred/dcrd/pull/2160))
- secp256k1: Add IsOnCurve method to PublicKey ([decred/dcrd#2162](https://github.com/decred/dcrd/pull/2162))
- secp256k1: Use specialized types in public key ([decred/dcrd#2163](https://github.com/decred/dcrd/pull/2163))
- schnorr: Add sign message example ([decred/dcrd#2164](https://github.com/decred/dcrd/pull/2164))
- schnorr: Add verify signature example ([decred/dcrd#2164](https://github.com/decred/dcrd/pull/2164))
- secp256k1: Optimize pubkey parse ([decred/dcrd#2167](https://github.com/decred/dcrd/pull/2167))
- connmgr: Fix potential panic via RPC ([decred/dcrd#2177](https://github.com/decred/dcrd/pull/2177))
- peer: Set a default idle timeout if not specified ([decred/dcrd#2180](https://github.com/decred/dcrd/pull/2180))
- wire: Improve error handling ([decred/dcrd#2179](https://github.com/decred/dcrd/pull/2179))
- rpcclient: Remove dcrwallet methods ([decred/dcrd#2178](https://github.com/decred/dcrd/pull/2178))
- server: Remove unused interrupt chan param ([decred/dcrd#2186](https://github.com/decred/dcrd/pull/2186))
- multi: CancelPending error for no pending conns ([decred/dcrd#2199](https://github.com/decred/dcrd/pull/2199))
- connmgr: Convert lifecycle to context ([decred/dcrd#2195](https://github.com/decred/dcrd/pull/2195))
- dcrutil: Add VerifyMessage API ([decred/dcrd#2203](https://github.com/decred/dcrd/pull/2203))
- blocklogger: Always log when sync height reached ([decred/dcrd#2204](https://github.com/decred/dcrd/pull/2204))
- connmgr: define connmgr error types ([decred/dcrd#2206](https://github.com/decred/dcrd/pull/2206))
- connmgr: Finish recent connmgr err type additions ([decred/dcrd#2208](https://github.com/decred/dcrd/pull/2208))
- stakeext: Fix comments on concurrency ([decred/dcrd#2210](https://github.com/decred/dcrd/pull/2210))
- txscript: Add support for errors.Is/As ([decred/dcrd#2209](https://github.com/decred/dcrd/pull/2209))
- secp256k1: Remove Encrypt/Decrypt functions ([decred/dcrd#2222](https://github.com/decred/dcrd/pull/2222))
- rpcserver: Create Chain and UtxoEntry interfaces ([decred/dcrd#2211](https://github.com/decred/dcrd/pull/2211))
- blockchain: Correct mempool view construction ([decred/dcrd#2232](https://github.com/decred/dcrd/pull/2232))
- rpcserver: Correct adaptor for utxo entry fetch ([decred/dcrd#2233](https://github.com/decred/dcrd/pull/2233))
- server: Log remote peer IP in several messages ([decred/dcrd#2233](https://github.com/decred/dcrd/pull/2233))
- peer: Add IsKnownInventory ([decred/dcrd#2239](https://github.com/decred/dcrd/pull/2239))
- txscript: Export several useful funcs for treasury ([decred/dcrd#2243](https://github.com/decred/dcrd/pull/2243))
- peer: check all peer deadlines in the stall ticker ([decred/dcrd#2251](https://github.com/decred/dcrd/pull/2251))
- txscript: Export script num type and constructor ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export MathOpCodeMaxScriptNumLen ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export CltvMaxScriptNumLen ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export CsvMaxScriptNumLen ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export IsSmallInt ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export AsSmallInt ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Export ExtractScriptHash ([decred/dcrd#2240](https://github.com/decred/dcrd/pull/2240))
- txscript: Remove deprecated code ([decred/dcrd#2241](https://github.com/decred/dcrd/pull/2241))
- txscript: Optimize sig enc check with mod n scalar ([decred/dcrd#2246](https://github.com/decred/dcrd/pull/2246))
- connmgr: Remain responsive with simul failed conns ([decred/dcrd#2254](https://github.com/decred/dcrd/pull/2254))
- secp256k1: Harden const time field normalization ([decred/dcrd#2258](https://github.com/decred/dcrd/pull/2258))
- rpcclient: Protect websocket connection with mutex ([decred/dcrd#2260](https://github.com/decred/dcrd/pull/2260))
- wire: formatting fixes - no functional change ([decred/dcrd#2266](https://github.com/decred/dcrd/pull/2266))
- wire: return detectable err from makeEmptyMessage ([decred/dcrd#2266](https://github.com/decred/dcrd/pull/2266))
- blockchain: Rename last prune time field ([decred/dcrd#2294](https://github.com/decred/dcrd/pull/2294))
- blockchain: Set pruning interval to tgt block time ([decred/dcrd#2294](https://github.com/decred/dcrd/pull/2294))
- blockchain: Optimize stake node pruning ([decred/dcrd#2294](https://github.com/decred/dcrd/pull/2294))
- txscript: Check equality via secp256k1 methods ([decred/dcrd#2299](https://github.com/decred/dcrd/pull/2299))
- blockchain: Remove internal dbnamespace package ([decred/dcrd#2305](https://github.com/decred/dcrd/pull/2305))
- txscript: Optimize alt stack drop ([decred/dcrd#2298](https://github.com/decred/dcrd/pull/2298))
- txscript: Optimize trace logging ([decred/dcrd#2301](https://github.com/decred/dcrd/pull/2301))
- peer: Optimize logging ([decred/dcrd#2303](https://github.com/decred/dcrd/pull/2303))
- blockchain: Optimize chain tip tracking ([decred/dcrd#2302](https://github.com/decred/dcrd/pull/2302))
- blockchain: Move stxo source to chain ([decred/dcrd#2304](https://github.com/decred/dcrd/pull/2304))
- blockchain: Use static log funcs for static logs ([decred/dcrd#2321](https://github.com/decred/dcrd/pull/2321))
- blockchain: Remove superfluous blockidx fields ([decred/dcrd#2321](https://github.com/decred/dcrd/pull/2321))
- blockchain: Migration for v3 block index ([decred/dcrd#2321](https://github.com/decred/dcrd/pull/2321))
- config: Categorize options in the code ([decred/dcrd#2320](https://github.com/decred/dcrd/pull/2320))
- main: Unexport main package exports ([decred/dcrd#2339](https://github.com/decred/dcrd/pull/2339))
- txscript: Correct JSON test data comment ([decred/dcrd#2354](https://github.com/decred/dcrd/pull/2354))
- blockchain: Decentralized Treasury db migration ([decred/dcrd#2336](https://github.com/decred/dcrd/pull/2336))
- blockchain: Add exported TSpendCountVotes func ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- txscript: Add shortTxHash ([decred/dcrd#2358](https://github.com/decred/dcrd/pull/2358))
- txscript: Store short tx hash in sigcache ([decred/dcrd#2358](https://github.com/decred/dcrd/pull/2358))
- txscript: Proactively evict SigCache entries ([decred/dcrd#2358](https://github.com/decred/dcrd/pull/2358))
- config: Consolidate error reporting ([decred/dcrd#2379](https://github.com/decred/dcrd/pull/2379))
- dcrutil: Update example to avoid chaincfg dep ([decred/dcrd#2382](https://github.com/decred/dcrd/pull/2382))
- blockchain: Remove need to RLock some treasury funcs ([decred/dcrd#2380](https://github.com/decred/dcrd/pull/2380))
- multi: Fix treasury-related comments ([decred/dcrd#2380](https://github.com/decred/dcrd/pull/2380))
- multi: update blockchain/standalone error types ([decred/dcrd#2380](https://github.com/decred/dcrd/pull/2380))
- standalone: Retain coinbase detection semantics ([decred/dcrd#2391](https://github.com/decred/dcrd/pull/2391))
- standalone: Introduce CalcTSpendWindow ([decred/dcrd#2389](https://github.com/decred/dcrd/pull/2389))
- standalone: Rename CalcTSpendExpiry ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- standalone: IsTVI code consistency pass ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- standalone: Misc comment consistency cleanup ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- blockchain: Add ticket exhaustion check ([decred/dcrd#2398](https://github.com/decred/dcrd/pull/2398))
- blockchain: Reject old block vers for tsry vote ([decred/dcrd#2400](https://github.com/decred/dcrd/pull/2400))
- blockchain: Simplify old block ver upgrade checks ([decred/dcrd#2401](https://github.com/decred/dcrd/pull/2401))
- multi: update blockchain and mempool error types ([decred/dcrd#2278](https://github.com/decred/dcrd/pull/2278))
- blockchain/mempool: Update for recent err convrsn ([decred/dcrd#2421](https://github.com/decred/dcrd/pull/2421))
- blockchain: Create treasury buckets during upgrade ([decred/dcrd#2441](https://github.com/decred/dcrd/pull/2441))
- blockchain: Fix stxosToScriptSource ([decred/dcrd#2444](https://github.com/decred/dcrd/pull/2444))
- blockchain: Make ver 5 to 6 db upgrades work again ([decred/dcrd#2446](https://github.com/decred/dcrd/pull/2446))
- blockchain: Clear failed block flags for HF ([decred/dcrd#2447](https://github.com/decred/dcrd/pull/2447))
- blockchain: Handle db upgrade paths for ver < 5 ([decred/dcrd#2449](https://github.com/decred/dcrd/pull/2449))
- blockchain: No context dep checks for orphans ([decred/dcrd#2474](https://github.com/decred/dcrd/pull/2474))

### Developer-related module management:

- mining: Start v3 module dev cycle ([decred/dcrd#1955](https://github.com/decred/dcrd/pull/1955))
- dcrutil: Start v3 module dev cycle ([decred/dcrd#1956](https://github.com/decred/dcrd/pull/1956))
- txscript: Start v3 module dev cycle ([decred/dcrd#1958](https://github.com/decred/dcrd/pull/1958))
- blockchain: Start v3 module dev cycle ([decred/dcrd#1959](https://github.com/decred/dcrd/pull/1959))
- stake: Start v3 module dev cycle ([decred/dcrd#1960](https://github.com/decred/dcrd/pull/1960))
- mempool: Start v4 module dev cycle ([decred/dcrd#1963](https://github.com/decred/dcrd/pull/1963))
- connmgr: Start v3 module dev cycle ([decred/dcrd#1975](https://github.com/decred/dcrd/pull/1975))
- multi: Use latest base58 module ([decred/dcrd#2016](https://github.com/decred/dcrd/pull/2016))
- dcrctl: Update dcrwallet RPC types package ([decred/dcrd#2018](https://github.com/decred/dcrd/pull/2018))
- multi: Update to prerel module release versions ([decred/dcrd#2032](https://github.com/decred/dcrd/pull/2032))
- multi: switch to syndtr/goleveldb ([decred/dcrd#2034](https://github.com/decred/dcrd/pull/2034))
- chaincfg: Start v3 module dev cycle ([decred/dcrd#2038](https://github.com/decred/dcrd/pull/2038))
- chaincfg: Remove chainec package ([decred/dcrd#2039](https://github.com/decred/dcrd/pull/2039))
- secp256k1: Start v3 module dev cycle ([decred/dcrd#2040](https://github.com/decred/dcrd/pull/2040))
- rpcclient: Start v6 module dev cycle ([decred/dcrd#1980](https://github.com/decred/dcrd/pull/1980))
- database, fees:  use latest leveldb ([decred/dcrd#2054](https://github.com/decred/dcrd/pull/2054))
- multi: Update to prerel module release versions ([decred/dcrd#2074](https://github.com/decred/dcrd/pull/2074))
- hdkeychain: Start v3 module dev cycle ([decred/dcrd#2076](https://github.com/decred/dcrd/pull/2076))
- multi: Update all prerel module release versions ([decred/dcrd#2082](https://github.com/decred/dcrd/pull/2082))
- multi: More prerel module release version updates ([decred/dcrd#2082](https://github.com/decred/dcrd/pull/2082))
- multi: Round 3 prerel module release ver updates ([decred/dcrd#2082](https://github.com/decred/dcrd/pull/2082))
- multi: Round 4 prerel module release ver updates ([decred/dcrd#2082](https://github.com/decred/dcrd/pull/2082))
- chaincfg: Remove unused modules ([decred/dcrd#2144](https://github.com/decred/dcrd/pull/2144))
- dcrutil: use errors api; require go 1.13+ ([decred/dcrd#2099](https://github.com/decred/dcrd/pull/2099))
- mempool: use errors api; require go 1.13+ ([decred/dcrd#2100](https://github.com/decred/dcrd/pull/2100))
- rpcclient: use errors api; require go 1.13+ ([decred/dcrd#2101](https://github.com/decred/dcrd/pull/2101))
- txscript: use errors api; require go 1.13+ ([decred/dcrd#2102](https://github.com/decred/dcrd/pull/2102))
- hdkeychain: Use errors api and require go 1.13+ ([decred/dcrd#2161](https://github.com/decred/dcrd/pull/2161))
- wire: use std errors api ([decred/dcrd#2182](https://github.com/decred/dcrd/pull/2182))
- rpcclient: bump to newer modules ([decred/dcrd#2190](https://github.com/decred/dcrd/pull/2190))
- multi: Run go mod tidy on all modules ([decred/dcrd#2185](https://github.com/decred/dcrd/pull/2185))
- main: Update go.mod for recent rpcclient bumps ([decred/dcrd#2194](https://github.com/decred/dcrd/pull/2194))
- multi: Use latest base58 module ([decred/dcrd#2223](https://github.com/decred/dcrd/pull/2223))
- standalone: Start v2 module dev cycle ([decred/dcrd#2224](https://github.com/decred/dcrd/pull/2224))
- multi: go mod tidy cleanup and run in CI ([decred/dcrd#2225](https://github.com/decred/dcrd/pull/2225))
- mempool: Move to internal ([decred/dcrd#2274](https://github.com/decred/dcrd/pull/2274))
- mining: Move to internal ([decred/dcrd#2275](https://github.com/decred/dcrd/pull/2275))
- rpcserver: Move to internal ([decred/dcrd#2288](https://github.com/decred/dcrd/pull/2288))
- fees: Move to internal ([decred/dcrd#2287](https://github.com/decred/dcrd/pull/2287))
- main: go mod tidy ([decred/dcrd#2367](https://github.com/decred/dcrd/pull/2367))
- dcrjson: Prepare v3.1.0 ([decred/dcrd#2374](https://github.com/decred/dcrd/pull/2374))
- addrmgr: Prepare v1.2.0 ([decred/dcrd#2375](https://github.com/decred/dcrd/pull/2375))
- connmgr: Prepare v3.0.0 ([decred/dcrd#2376](https://github.com/decred/dcrd/pull/2376))
- multi: Update chaincfg dependers to wire/v1.4.0 ([decred/dcrd#2381](https://github.com/decred/dcrd/pull/2381))
- chaincfg: Prepare v3.0.0 ([decred/dcrd#2381](https://github.com/decred/dcrd/pull/2381))
- dcrutil: Prepare v3.0.0 ([decred/dcrd#2383](https://github.com/decred/dcrd/pull/2383))
- rpc/jsonrpc/types: Prepare v2.1.0 ([decred/dcrd#2385](https://github.com/decred/dcrd/pull/2385))
- txscript: Prepare v3.0.0 ([decred/dcrd#2384](https://github.com/decred/dcrd/pull/2384))
- blockchain: Update unreleased requires to master ([decred/dcrd#2364](https://github.com/decred/dcrd/pull/2364))
- rpcclient: Update unreleased requires to master ([decred/dcrd#2369](https://github.com/decred/dcrd/pull/2369))
- blockchain/standalone: Remove txscript dep ([decred/dcrd#2388](https://github.com/decred/dcrd/pull/2388))
- database: Prepare v2.0.2 ([decred/dcrd#2387](https://github.com/decred/dcrd/pull/2387))
- hdkeycahin: Prepare v3.0.0 ([decred/dcrd#2392](https://github.com/decred/dcrd/pull/2392))
- blockchain/standalone: Prepare v2.0.0 ([decred/dcrd#2395](https://github.com/decred/dcrd/pull/2395))
- blockchain/stake: Prepare v3.0.0 ([decred/dcrd#2418](https://github.com/decred/dcrd/pull/2418))
- gcs: Prepare v2.1.0 ([decred/dcrd#2420](https://github.com/decred/dcrd/pull/2420))
- peer: Prepare v2.2.0 ([decred/dcrd#2422](https://github.com/decred/dcrd/pull/2422))
- rpcclient: Prepare v6.0.0 ([decred/dcrd#2423](https://github.com/decred/dcrd/pull/2423))
- blockchain: Prepare v3.0.0 ([decred/dcrd#2424](https://github.com/decred/dcrd/pull/2424))
- rpcclient: Prepare v6.0.1 ([decred/dcrd#2455](https://github.com/decred/dcrd/pull/2455))
- main: Update to use all new module versions ([decred/dcrd#2426](https://github.com/decred/dcrd/pull/2426))
- main: Remove module replacements ([decred/dcrd#2428](https://github.com/decred/dcrd/pull/2428))
- main: Use backported module updates ([decred/dcrd#2456](https://github.com/decred/dcrd/pull/2456))

### Testing and Quality Assurance:

- build: update golangci-lint to v1.21.0 ([decred/dcrd#1951](https://github.com/decred/dcrd/pull/1951))
- mining: Add priority calculation tests ([decred/dcrd#1967](https://github.com/decred/dcrd/pull/1967))
- build: Add deadcode to linters for CI tests ([decred/dcrd#1993](https://github.com/decred/dcrd/pull/1993))
- multi: Updates for staticcheck results ([decred/dcrd#1994](https://github.com/decred/dcrd/pull/1994))
- blockchain: Separate processing order tests ([decred/dcrd#2004](https://github.com/decred/dcrd/pull/2004))
- blockchain: Add benchmark for ancestor traversal ([decred/dcrd#2010](https://github.com/decred/dcrd/pull/2010))
- multi: Address a bunch of lint issues ([decred/dcrd#2028](https://github.com/decred/dcrd/pull/2028))
- build: golangci-lint v1.22.2 ([decred/dcrd#2029](https://github.com/decred/dcrd/pull/2029))
- secpk256k1: Add benchmark for RFC6979 nonce gen ([decred/dcrd#2044](https://github.com/decred/dcrd/pull/2044))
- secp256k1: Cleanup signature tests ([decred/dcrd#2048](https://github.com/decred/dcrd/pull/2048))
- rpctest: adapt new API ([decred/dcrd#1980](https://github.com/decred/dcrd/pull/1980))
- rpcserver: Add handlers test ([decred/dcrd#2066](https://github.com/decred/dcrd/pull/2066))
- build: use golangci v1.23.6 ([decred/dcrd#2068](https://github.com/decred/dcrd/pull/2068))
- rpctest: Update for hdkeychain API changes ([decred/dcrd#2092](https://github.com/decred/dcrd/pull/2092))
- build: test against go 1.14 ([decred/dcrd#2092](https://github.com/decred/dcrd/pull/2092))
- secp256k1: Add benchmark for signing ([decred/dcrd#2085](https://github.com/decred/dcrd/pull/2085))
- seck256k1: Add benchmark for sig serialization ([decred/dcrd#2087](https://github.com/decred/dcrd/pull/2087))
- secp256k1: Add benchmark for pubkey decompression ([decred/dcrd#2094](https://github.com/decred/dcrd/pull/2094))
- secp256k1: Move sig benchmarks to separate file ([decred/dcrd#2095](https://github.com/decred/dcrd/pull/2095))
- secp256k1: Add benchmark for SignCompact ([decred/dcrd#2095](https://github.com/decred/dcrd/pull/2095))
- secp256k1: Add benchmark for RecoverCompact ([decred/dcrd#2095](https://github.com/decred/dcrd/pull/2095))
- secp256k1: Rework DER sig parsing tests ([decred/dcrd#2109](https://github.com/decred/dcrd/pull/2109))
- schnorr: Cleanup signature benchmarking ([decred/dcrd#2126](https://github.com/decred/dcrd/pull/2126))
- schnorr: Rework signing tests ([decred/dcrd#2128](https://github.com/decred/dcrd/pull/2128))
- secp256k1: Make field value tests more consistent ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- secp256k1: Move field val set hex to test file ([decred/dcrd#2134](https://github.com/decred/dcrd/pull/2134))
- schnorr: Add negative tests for sig verification ([decred/dcrd#2145](https://github.com/decred/dcrd/pull/2145))
- hdkeychain: Add child key with leading zeros test ([decred/dcrd#2155](https://github.com/decred/dcrd/pull/2155))
- schnorr: Add benchmark for Signature.Serialize ([decred/dcrd#2158](https://github.com/decred/dcrd/pull/2158))
- secp256k1: Rework pubkey tests ([decred/dcrd#2160](https://github.com/decred/dcrd/pull/2160))
- secp256k1: Explicit pubkey parsing errors in tests ([decred/dcrd#2160](https://github.com/decred/dcrd/pull/2160))
- secp256k1: Add compressed pubkey parse benchmark ([decred/dcrd#2167](https://github.com/decred/dcrd/pull/2167))
- secp256k1: Add uncompressed pubkey parse benchmark ([decred/dcrd#2167](https://github.com/decred/dcrd/pull/2167))
- build: use newer github and linter versions ([decred/dcrd#2182](https://github.com/decred/dcrd/pull/2182))
- wire: Test no-relay case in TestVersionWire ([decred/dcrd#2184](https://github.com/decred/dcrd/pull/2184))
- wire: Use new errors.Is capabilities in tests ([decred/dcrd#2183](https://github.com/decred/dcrd/pull/2183))
- connmgr: Add test for dial timeout ([decred/dcrd#2189](https://github.com/decred/dcrd/pull/2189))
- connmgr: Add test for connect context cancel ([decred/dcrd#2189](https://github.com/decred/dcrd/pull/2189))
- connmgr: Refactor conn req ID/state test asserts ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Update tests to ensure clean shutdown ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Improve TestConnectMode robustness ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Increase timeout in TestTargetOutbound ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Shore up TestMaxRetryDuration ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Tighten TestNetworkFailure ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Tighten TestStopFailed ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Tighten TestRemovePendingConnection ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- connmgr: Cleanup TestCancelIgnoreDelayedConnection ([decred/dcrd#2192](https://github.com/decred/dcrd/pull/2192))
- server: Actively prevent regnet network discovery ([decred/dcrd#2197](https://github.com/decred/dcrd/pull/2197))
- Add debug and trace facility to rpctest ([decred/dcrd#2176](https://github.com/decred/dcrd/pull/2176))
- build: use golangci-lint v1.27.0 ([decred/dcrd#2207](https://github.com/decred/dcrd/pull/2207))
- rpcserver: Add handler test coverage ([decred/dcrd#2230](https://github.com/decred/dcrd/pull/2230))
- rpcserver: Add handleDecodeScript test ([decred/dcrd#2238](https://github.com/decred/dcrd/pull/2238))
- txscript: Add tests for new strict null data func ([decred/dcrd#2248](https://github.com/decred/dcrd/pull/2248))
- rpcserver: Add default configs for tests ([decred/dcrd#2249](https://github.com/decred/dcrd/pull/2249))
- txscript: Rework check signature encoding test ([decred/dcrd#2244](https://github.com/decred/dcrd/pull/2244))
- rpcserver: Add tests for block related handlers ([decred/dcrd#2250](https://github.com/decred/dcrd/pull/2250))
- txscript: Rework check pubkey encoding test ([decred/dcrd#2247](https://github.com/decred/dcrd/pull/2247))
- txscript: Add benchmark for CheckSignatureEncoding ([decred/dcrd#2246](https://github.com/decred/dcrd/pull/2246))
- connmgr: Use t.Fatal when there are no params ([decred/dcrd#2254](https://github.com/decred/dcrd/pull/2254))
- rpcserver: Rework default configs for tests ([decred/dcrd#2257](https://github.com/decred/dcrd/pull/2257))
- rpcserver: Update tests to use default configs ([decred/dcrd#2257](https://github.com/decred/dcrd/pull/2257))
- rpcserver: Run tests in parallel ([decred/dcrd#2257](https://github.com/decred/dcrd/pull/2257))
- rpcserver: Update error case handling in tests ([decred/dcrd#2257](https://github.com/decred/dcrd/pull/2257))
- rpcserver: Add handleEstimateSmartFee test ([decred/dcrd#2255](https://github.com/decred/dcrd/pull/2255))
- rpcserver: Add handleEstimateStakeDiff test ([decred/dcrd#2269](https://github.com/decred/dcrd/pull/2269))
- rpcserver: Add handleGetTicketPoolValue test ([decred/dcrd#2272](https://github.com/decred/dcrd/pull/2272))
- rpcserver: Add handleGetStakeVersions test ([decred/dcrd#2272](https://github.com/decred/dcrd/pull/2272))
- rpcserver: Add handleGetStakeVersionInfo test ([decred/dcrd#2272](https://github.com/decred/dcrd/pull/2272))
- mempool: Don't use deprecated reject code in tests ([decred/dcrd#2273](https://github.com/decred/dcrd/pull/2273))
- build: golangci-lint v1.28.3 ([decred/dcrd#2266](https://github.com/decred/dcrd/pull/2266))
- rpcserver: add missed and live tickets rpc tests ([decred/dcrd#2284](https://github.com/decred/dcrd/pull/2284))
- rpcserver: add verifychain & getdifficulty tests ([decred/dcrd#2285](https://github.com/decred/dcrd/pull/2285))
- multi: add BlockTemplater interface ([decred/dcrd#2292](https://github.com/decred/dcrd/pull/2292))
- multi: add rpcCPUMiner adaptor ([decred/dcrd#2300](https://github.com/decred/dcrd/pull/2300))
- connmgr: Improve dial timeout test synchronization ([decred/dcrd#2309](https://github.com/decred/dcrd/pull/2309))
- rpcserver: Add handleGetCFilter tests ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- rpcserver: Add handleGetCFilterHeader tests ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- rpcserver: Add handleGetCFilterV2 tests ([decred/dcrd#2312](https://github.com/decred/dcrd/pull/2312))
- rpcserver: Add handleExistsAddress test ([decred/dcrd#2291](https://github.com/decred/dcrd/pull/2291))
- rpcserver: Add handleExistsAddresses test ([decred/dcrd#2291](https://github.com/decred/dcrd/pull/2291))
- contrib: Respect quoted args in simnet ctl scripts ([decred/dcrd#2322](https://github.com/decred/dcrd/pull/2322))
- contrib: Support MSYS2 in simnet setup script ([decred/dcrd#2323](https://github.com/decred/dcrd/pull/2323))
- multi: add getwork tests ([decred/dcrd#2306](https://github.com/decred/dcrd/pull/2306))
- rpcserver: add setgenerate & regentemplate tests ([decred/dcrd#2308](https://github.com/decred/dcrd/pull/2308))
- rpcserver: Add TxMempooler interface ([decred/dcrd#2324](https://github.com/decred/dcrd/pull/2324))
- rpcserver: Add handleExistsMempoolTxs test ([decred/dcrd#2324](https://github.com/decred/dcrd/pull/2324))
- contrib: Update simnet script for dcrwallet master ([decred/dcrd#2327](https://github.com/decred/dcrd/pull/2327))
- contrib: Support env var in simnet setup script ([decred/dcrd#2328](https://github.com/decred/dcrd/pull/2328))
- contrib: Use var for simnet wallet create answers ([decred/dcrd#2328](https://github.com/decred/dcrd/pull/2328))
- contrib: Update simnet script for wallet cointype ([decred/dcrd#2333](https://github.com/decred/dcrd/pull/2333))
- build: test against go 1.15 ([decred/dcrd#2334](https://github.com/decred/dcrd/pull/2334))
- blockchain: Add test func to remove deployment ([decred/dcrd#2343](https://github.com/decred/dcrd/pull/2343))
- rpcserver: Add AddrIndexer interface ([decred/dcrd#2330](https://github.com/decred/dcrd/pull/2330))
- rpcserver: Add TxIndexer interface ([decred/dcrd#2330](https://github.com/decred/dcrd/pull/2330))
- rpcserver: Add testDB and testDatabaseTx ([decred/dcrd#2330](https://github.com/decred/dcrd/pull/2330))
- rpcserver: Add handleSearchRawTransactions tests ([decred/dcrd#2330](https://github.com/decred/dcrd/pull/2330))
- rpcserver: Add handleGenerate test ([decred/dcrd#2342](https://github.com/decred/dcrd/pull/2342))
- mempool: Add TAdd Tests ([decred/dcrd#2350](https://github.com/decred/dcrd/pull/2350))
- mempool: Improve tspend expiry handling and tests ([decred/dcrd#2350](https://github.com/decred/dcrd/pull/2350))
- rpcserver: Verify tbase values in treasury rpctest ([decred/dcrd#2352](https://github.com/decred/dcrd/pull/2352))
- rpctest: Add ability to limit VotingWallet votes ([decred/dcrd#2352](https://github.com/decred/dcrd/pull/2352))
- rpcserver: Assert vote counts in treasury rpctest ([decred/dcrd#2351](https://github.com/decred/dcrd/pull/2351))
- rpctest: Make votingwallet txs standard ([decred/dcrd#2373](https://github.com/decred/dcrd/pull/2373))
- dcrutil: Cleanup verify tests and use mock params ([decred/dcrd#2382](https://github.com/decred/dcrd/pull/2382))
- standalone: Add IsTreasuryVoteInterval tests ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- standalone: Rework and add CalcTSpendExpiry tests ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- standalone: Add InsideTSpendWindow tests ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- standalone: Add IsTreasuryBase tests ([decred/dcrd#2394](https://github.com/decred/dcrd/pull/2394))
- chaingen: implement DCP0001 for generator ([decred/dcrd#2329](https://github.com/decred/dcrd/pull/2329))
- blockchain: Add chaingen harness AdvanceToHeight ([decred/dcrd#2090](https://github.com/decred/dcrd/pull/2090))
- blockchain: Rework AdvanceToHeight ([decred/dcrd#2090](https://github.com/decred/dcrd/pull/2090))
- rpcserver: Add --rejectnonstd to rpctest ([decred/dcrd#2415](https://github.com/decred/dcrd/pull/2415))

### Misc:

- release: Bump for 1.6 release cycle ([decred/dcrd#1948](https://github.com/decred/dcrd/pull/1948))
- multi: resolve todos ([decred/dcrd#1869](https://github.com/decred/dcrd/pull/1869))
- multi: remove whitespace ([decred/dcrd#2009](https://github.com/decred/dcrd/pull/2009))
- release: Add example OpenBSD rc.d service script ([decred/dcrd#2030](https://github.com/decred/dcrd/pull/2030))
- release: Remove build metadata from master branch ([decred/dcrd#2053](https://github.com/decred/dcrd/pull/2053))
- secp256k1: Improve NonceRFC6979 comment ([decred/dcrd#2044](https://github.com/decred/dcrd/pull/2044))
- secp256k1: Correct comments in signature.go ([decred/dcrd#2046](https://github.com/decred/dcrd/pull/2046))
- multi: Resolve go1.15 vet complaints ([decred/dcrd#2310](https://github.com/decred/dcrd/pull/2310))
- multi: Address some linter complaints ([decred/dcrd#2399](https://github.com/decred/dcrd/pull/2399))
- build: bump golangci-lint to 1.24.0 ([decred/dcrd#2141](https://github.com/decred/dcrd/pull/2141))
- main: Simplify startup logic slightly ([decred/dcrd#2293](https://github.com/decred/dcrd/pull/2293))
- docker: Update image to golang:1.14 ([decred/dcrd#2202](https://github.com/decred/dcrd/pull/2202))
- release: Remove no longer used release bits ([decred/dcrd#2317](https://github.com/decred/dcrd/pull/2317))
- docker: Update image to golang:1.15 ([decred/dcrd#2335](https://github.com/decred/dcrd/pull/2335))
- release: Bump for 1.6.0 ([decred/dcrd#2340](https://github.com/decred/dcrd/pull/2340))

### Code Contributors (alphabetical order):

- Brian Stafford
- Dave Collins
- David Hill
- degeri
- Donald Adu-Poku
- Jamie Holdstock
- Joe Gruffins
- Josh Rickmar
- Julian Yap
- Marco Peereboom
- Matheus Degiovani
- Matt Hawkins
- Ryan Riley
- Ryan Staudt
- Wisdom Arerosuoghene
- Youssef Boukenken
- zhizhongzhiwai

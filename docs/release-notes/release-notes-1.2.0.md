# dcrd v1.2.0

This release of dcrd contains significant performance enhancements,
infrastructure improvements, improved access to chain-related information for
providing better SPV (Simplified Payment Verification) support, and other
quality assurance changes.

A significant amount of infrastructure work has also been done this release
cycle towards being able to support several planned scalability optimizations.

## Downgrade Warning

The database format in v1.2.0 is not compatible with previous versions of the
software.  This only affects downgrades as users upgrading from previous
versions will see a one time database migration.

Once this migration has been completed, it will no longer be possible to
downgrade to a previous version of the software without having to delete the
database and redownload the chain.

## Notable Changes

### Significantly Faster Startup

The startup time has been improved by roughly 17x on slower hard disk drives
(HDDs) and 8x on solid state drives (SSDs).

In order to achieve these speedups, there is a one time database migration, as
previously mentioned, that will likely take a while to complete (typically
around 5 to 6 minutes on HDDs and 2 to 3 minutes on SSDs).

### Support For DNS Seed Filtering

In order to better support the forthcoming SPV wallets, support for finding
other peers based upon their enabled services has been added.  This is useful
for both SPV wallets and full nodes since SPV wallets will require access to
full nodes in order to retrieve the necessary proofs and full nodes are
generally not interested in making outgoing connections to SPV wallets.

### Committed Filters

With the intention of supporting light clients, such as SPV wallets, in a
privacy-preserving way while still minimizing the amount of data that needs to
be downloaded, this release adds support for committed filters.  A committed
filter is a combination of a probalistic data structure that is used to test
whether an element is a member of a set with a predetermined collision
probability along with a commitment by consensus-validating full nodes to that
data.

A committed filter is created for every block which allows light clients to
download the filters and match against them locally rather than uploading
personal data to other nodes.

A new service flag is also provided to allow clients to discover nodes that
provide access to filters.

There is a one time database update to build and store the filters for all
existing historical blocks which will likely take a while to complete (typically
around 2 to 3 minutes on HDDs and 1 to 1.5 minutes on SSDs).

### Updated Atomic Swap Contracts

The standard checks for atomic swap contracts have been updated to ensure the
contracts enforce the secret size for safer support between chains with
disparate script rules.

### RPC Server Changes

#### New `getchaintips` RPC

A new RPC named `getchaintips` has been added which allows callers to query
information about the status of known side chains and their branch lengths.
It currently only provides support for side chains that have been seen while the
current instance of the process is running.  This will be further improved in
future releases.

## Changelog

All commits since the last release may be viewed on GitHub [here](https://github.com/decred/dcrd/compare/v1.1.2...v1.2.0).

### Protocol and network:

- chaincfg: Add checkpoints for 1.2.0 release ([decred/dcrd#1139](https://github.com/decred/dcrd/pull/1139))
- chaincfg: Introduce new type DNSSeed ([decred/dcrd#961](https://github.com/decred/dcrd/pull/961))
- blockmanager: sync with the most updated peer ([decred/dcrd#984](https://github.com/decred/dcrd/pull/984))
- multi: remove MsgAlert ([decred/dcrd#1161](https://github.com/decred/dcrd/pull/1161))
- multi: Add initial committed filter (CF) support ([decred/dcrd#1151](https://github.com/decred/dcrd/pull/1151))

### Transaction relay (memory pool):

- txscript: Correct nulldata standardness check ([decred/dcrd#935](https://github.com/decred/dcrd/pull/935))
- mempool: Optimize orphan map limiting ([decred/dcrd#1117](https://github.com/decred/dcrd/pull/1117))
- mining: Fix duplicate txns in the prio heap ([decred/dcrd#1108](https://github.com/decred/dcrd/pull/1108))
- mining: Stop transactions losing their dependants ([decred/dcrd#1109](https://github.com/decred/dcrd/pull/1109))

### RPC:

- rpcserver: skip cert create when RPC is disabled ([decred/dcrd#949](https://github.com/decred/dcrd/pull/949))
- rpcserver: remove redundant checks in blockTemplateResult ([decred/dcrd#826](https://github.com/decred/dcrd/pull/826))
- rpcserver: assert network for validateaddress rpc ([decred/dcrd#963](https://github.com/decred/dcrd/pull/963))
- rpcserver: Do not rebroadcast stake transactions ([decred/dcrd#973](https://github.com/decred/dcrd/pull/973))
- dcrjson: add ticket fee field to PurchaseTicketCmd ([decred/dcrd#902](https://github.com/decred/dcrd/pull/902))
- dcrwalletextcmds: remove getseed ([decred/dcrd#985](https://github.com/decred/dcrd/pull/985))
- dcrjson: Add SweepAccountCmd & SweepAccountResult ([decred/dcrd#1027](https://github.com/decred/dcrd/pull/1027))
- rpcserver: add sweepaccount to the wallet list of commands ([decred/dcrd#1028](https://github.com/decred/dcrd/pull/1028))
- rpcserver: add batched request support (json 2.0) ([decred/dcrd#841](https://github.com/decred/dcrd/pull/841))
- dcrjson: include summary totals in GetBalanceResult ([decred/dcrd#1062](https://github.com/decred/dcrd/pull/1062))
- multi: Implement getchaintips JSON-RPC ([decred/dcrd#1098](https://github.com/decred/dcrd/pull/1098))
- rpcserver: Add dcrd version info to getversion RPC ([decred/dcrd#1097](https://github.com/decred/dcrd/pull/1097))
- rpcserver: Correct getblockheader result text ([decred/dcrd#1104](https://github.com/decred/dcrd/pull/1104))
- dcrjson: add StartAutoBuyerCmd & StopAutoBuyerCmd ([decred/dcrd#903](https://github.com/decred/dcrd/pull/903))
- dcrjson: fix typo for StartAutoBuyerCmd ([decred/dcrd#1146](https://github.com/decred/dcrd/pull/1146))
- dcrjson: require passphrase for StartAutoBuyerCmd ([decred/dcrd#1147](https://github.com/decred/dcrd/pull/1147))
- dcrjson: fix StopAutoBuyerCmd registration bug ([decred/dcrd#1148](https://github.com/decred/dcrd/pull/1148))
- blockchain: Support testnet stake diff estimation ([decred/dcrd#1115](https://github.com/decred/dcrd/pull/1115))
- rpcserver: fix jsonRPCRead data race ([decred/dcrd#1157](https://github.com/decred/dcrd/pull/1157))
- dcrjson: Add VerifySeedCmd ([decred/dcrd#1160](https://github.com/decred/dcrd/pull/1160))

### dcrd command-line flags and configuration:

- mempool: Rename RelayNonStd config option ([decred/dcrd#1024](https://github.com/decred/dcrd/pull/1024))
- sampleconfig: Update min relay fee ([decred/dcrd#959](https://github.com/decred/dcrd/pull/959))
- sampleconfig: Correct comment ([decred/dcrd#1063](https://github.com/decred/dcrd/pull/1063))
- multi: Expand ~ to correct home directory on all OSes ([decred/dcrd#1041](https://github.com/decred/dcrd/pull/1041))

### checkdevpremine utility changes:

- checkdevpremine: Remove --skipverify option ([decred/dcrd#969](https://github.com/decred/dcrd/pull/969))
- checkdevpremine: Implement --notls option ([decred/dcrd#969](https://github.com/decred/dcrd/pull/969))
- checkdevpremine: Make file naming consistent ([decred/dcrd#969](https://github.com/decred/dcrd/pull/969))
- checkdevpremine: Fix comment ([decred/dcrd#969](https://github.com/decred/dcrd/pull/969))
- checkdevpremine: Remove utility ([decred/dcrd#1068](https://github.com/decred/dcrd/pull/1068))

### Documentation:

- fullblocktests: Add missing doc.go file ([decred/dcrd#956](https://github.com/decred/dcrd/pull/956))
- docs: Add fullblocktests entry and make consistent ([decred/dcrd#956](https://github.com/decred/dcrd/pull/956))
- docs: Add mempool entry to developer tools section ([decred/dcrd#1058](https://github.com/decred/dcrd/pull/1058))
- mempool: Add docs.go and flesh out README.md ([decred/dcrd#1058](https://github.com/decred/dcrd/pull/1058))
- docs: document packages and fix typo  ([decred/dcrd#965](https://github.com/decred/dcrd/pull/965))
- docs: rpcclient is now part of the main dcrd repo ([decred/dcrd#970](https://github.com/decred/dcrd/pull/970))
- dcrjson: Update README.md ([decred/dcrd#982](https://github.com/decred/dcrd/pull/982))
- docs: Remove carriage return ([decred/dcrd#1106](https://github.com/decred/dcrd/pull/1106))
- Adjust README.md for new Go version ([decred/dcrd#1105](https://github.com/decred/dcrd/pull/1105))
- docs: document how to use go test -coverprofile ([decred/dcrd#1107](https://github.com/decred/dcrd/pull/1107))
- addrmgr: Improve documentation ([decred/dcrd#1125](https://github.com/decred/dcrd/pull/1125))
- docs: Fix links for internal packages ([decred/dcrd#1144](https://github.com/decred/dcrd/pull/1144))

### Developer-related package changes:

- chaingen: Add revocation generation infrastructure ([decred/dcrd#1120](https://github.com/decred/dcrd/pull/1120))
- txscript: Add null data script creator ([decred/dcrd#943](https://github.com/decred/dcrd/pull/943))
- txscript: Cleanup and improve NullDataScript tests ([decred/dcrd#943](https://github.com/decred/dcrd/pull/943))
- txscript: Allow external signature hash calc ([decred/dcrd#951](https://github.com/decred/dcrd/pull/951))
- secp256k1: update func signatures ([decred/dcrd#934](https://github.com/decred/dcrd/pull/934))
- txscript: enforce MaxDataCarrierSize for GenerateProvablyPruneableOut ([decred/dcrd#953](https://github.com/decred/dcrd/pull/953))
- txscript: Remove OP_SMALLDATA ([decred/dcrd#954](https://github.com/decred/dcrd/pull/954))
- blockchain: Accept header in CheckProofOfWork ([decred/dcrd#977](https://github.com/decred/dcrd/pull/977))
- blockchain: Make func definition style consistent ([decred/dcrd#983](https://github.com/decred/dcrd/pull/983))
- blockchain: only fetch the parent block in BFFastAdd ([decred/dcrd#972](https://github.com/decred/dcrd/pull/972))
- blockchain: Switch to FindSpentTicketsInBlock ([decred/dcrd#915](https://github.com/decred/dcrd/pull/915))
- stake: Add Hash256PRNG init vector support ([decred/dcrd#986](https://github.com/decred/dcrd/pull/986))
- blockchain/stake: Use Hash256PRNG init vector ([decred/dcrd#987](https://github.com/decred/dcrd/pull/987))
- blockchain: Don't store full header in block node ([decred/dcrd#988](https://github.com/decred/dcrd/pull/988))
- blockchain: Reconstruct headers from block nodes ([decred/dcrd#989](https://github.com/decred/dcrd/pull/989))
- stake/multi: Don't return errors for IsX functions ([decred/dcrd#995](https://github.com/decred/dcrd/pull/995))
- blockchain: Rename block index to main chain index ([decred/dcrd#996](https://github.com/decred/dcrd/pull/996))
- blockchain: Refactor main block index logic ([decred/dcrd#990](https://github.com/decred/dcrd/pull/990))
- blockchain: Use hash values in structs ([decred/dcrd#992](https://github.com/decred/dcrd/pull/992))
- blockchain: Remove unused dump function ([decred/dcrd#1001](https://github.com/decred/dcrd/pull/1001))
- blockchain: Generalize and optimize chain reorg ([decred/dcrd#997](https://github.com/decred/dcrd/pull/997))
- blockchain: Pass parent block in connection code ([decred/dcrd#998](https://github.com/decred/dcrd/pull/998))
- blockchain: Explicit block fetch semanticss ([decred/dcrd#999](https://github.com/decred/dcrd/pull/999))
- blockchain: Use next detach block in reorg chain ([decred/dcrd#1002](https://github.com/decred/dcrd/pull/1002))
- blockchain: Limit header sanity check to header ([decred/dcrd#1003](https://github.com/decred/dcrd/pull/1003))
- blockchain: Validate num votes in header sanity ([decred/dcrd#1005](https://github.com/decred/dcrd/pull/1005))
- blockchain: Validate max votes in header sanity ([decred/dcrd#1006](https://github.com/decred/dcrd/pull/1006))
- blockchain: Validate stake diff in header context ([decred/dcrd#1004](https://github.com/decred/dcrd/pull/1004))
- blockchain: No votes/revocations in header sanity ([decred/dcrd#1007](https://github.com/decred/dcrd/pull/1007))
- blockchain: Validate max purchases in header sanity ([decred/dcrd#1008](https://github.com/decred/dcrd/pull/1008))
- blockchain: Validate early votebits in header sanity ([decred/dcrd#1009](https://github.com/decred/dcrd/pull/1009))
- blockchain: Validate block height in header context ([decred/dcrd#1010](https://github.com/decred/dcrd/pull/1010))
- blockchain: Move check block context func ([decred/dcrd#1011](https://github.com/decred/dcrd/pull/1011))
- blockchain: Block sanity cleanup and consistency ([decred/dcrd#1012](https://github.com/decred/dcrd/pull/1012))
- blockchain: Remove dup ticket purchase value check ([decred/dcrd#1013](https://github.com/decred/dcrd/pull/1013))
- blockchain: Only tickets before SVH in block sanity ([decred/dcrd#1014](https://github.com/decred/dcrd/pull/1014))
- blockchain: Remove unused vote bits function ([decred/dcrd#1015](https://github.com/decred/dcrd/pull/1015))
- blockchain: Move upgrade-only code to upgrade.go ([decred/dcrd#1016](https://github.com/decred/dcrd/pull/1016))
- stake: Static assert of vote commitment ([decred/dcrd#1020](https://github.com/decred/dcrd/pull/1020))
- blockchain: Remove unused error code ([decred/dcrd#1021](https://github.com/decred/dcrd/pull/1021))
- blockchain: Improve readability of parent approval ([decred/dcrd#1022](https://github.com/decred/dcrd/pull/1022))
- peer: rename mruinvmap, mrunoncemap to lruinvmap, lrunoncemap ([decred/dcrd#976](https://github.com/decred/dcrd/pull/976))
- peer: rename noncemap to noncecache ([decred/dcrd#976](https://github.com/decred/dcrd/pull/976))
- peer: rename inventorymap to inventorycache ([decred/dcrd#976](https://github.com/decred/dcrd/pull/976))
- connmgr: convert state to atomic ([decred/dcrd#1025](https://github.com/decred/dcrd/pull/1025))
- blockchain/mining: Full checks in CCB ([decred/dcrd#1017](https://github.com/decred/dcrd/pull/1017))
- blockchain: Validate pool size in header context ([decred/dcrd#1018](https://github.com/decred/dcrd/pull/1018))
- blockchain: Vote commitments in block sanity ([decred/dcrd#1023](https://github.com/decred/dcrd/pull/1023))
- blockchain: Validate early final state is zero ([decred/dcrd#1031](https://github.com/decred/dcrd/pull/1031))
- blockchain: Validate final state in header context ([decred/dcrd#1034](https://github.com/decred/dcrd/pull/1033))
- blockchain: Max revocations in block sanity ([decred/dcrd#1034](https://github.com/decred/dcrd/pull/1034))
- blockchain: Allowed stake txns in block sanity ([decred/dcrd#1035](https://github.com/decred/dcrd/pull/1035))
- blockchain: Validate allowed votes in block context ([decred/dcrd#1036](https://github.com/decred/dcrd/pull/1036))
- blockchain: Validate allowed revokes in blk contxt ([decred/dcrd#1037](https://github.com/decred/dcrd/pull/1037))
- blockchain/stake: Rename tix spent to tix voted ([decred/dcrd#1038](https://github.com/decred/dcrd/pull/1038))
- txscript: Require atomic swap contracts to specify the secret size ([decred/dcrd#1039](https://github.com/decred/dcrd/pull/1039))
- blockchain: Remove unused struct ([decred/dcrd#1043](https://github.com/decred/dcrd/pull/1043))
- blockchain: Store side chain blocks in database ([decred/dcrd#1000](https://github.com/decred/dcrd/pull/1000))
- blockchain: Simplify initial chain state ([decred/dcrd#1045](https://github.com/decred/dcrd/pull/1045))
- blockchain: Rework database versioning ([decred/dcrd#1047](https://github.com/decred/dcrd/pull/1047))
- blockchain: Don't require chain for db upgrades ([decred/dcrd#1051](https://github.com/decred/dcrd/pull/1051))
- blockchain/indexers: Allow interrupts ([decred/dcrd#1052](https://github.com/decred/dcrd/pull/1052))
- blockchain: Remove old version information ([decred/dcrd#1055](https://github.com/decred/dcrd/pull/1055))
- stake: optimize FindSpentTicketsInBlock slightly ([decred/dcrd#1049](https://github.com/decred/dcrd/pull/1049))
- blockchain: Do not accept orphans/genesis block ([decred/dcrd#1057](https://github.com/decred/dcrd/pull/1057))
- blockchain: Separate node ticket info population ([decred/dcrd#1056](https://github.com/decred/dcrd/pull/1056))
- blockchain: Accept parent in blockNode constructor ([decred/dcrd#1056](https://github.com/decred/dcrd/pull/1056))
- blockchain: Combine ErrDoubleSpend & ErrMissingTx ([decred/dcrd#1064](https://github.com/decred/dcrd/pull/1064))
- blockchain: Calculate the lottery IV on demand ([decred/dcrd#1065](https://github.com/decred/dcrd/pull/1065))
- blockchain: Simplify add/remove node logic ([decred/dcrd#1067](https://github.com/decred/dcrd/pull/1067))
- blockchain: Infrastructure to manage block index ([decred/dcrd#1044](https://github.com/decred/dcrd/pull/1044))
- blockchain: Add block validation status to index ([decred/dcrd#1044](https://github.com/decred/dcrd/pull/1044))
- blockchain: Migrate to new block indexuse it ([decred/dcrd#1044](https://github.com/decred/dcrd/pull/1044))
- blockchain: Lookup child in force head reorg ([decred/dcrd#1070](https://github.com/decred/dcrd/pull/1070))
- blockchain: Refactor block idx entry serialization ([decred/dcrd#1069](https://github.com/decred/dcrd/pull/1069))
- blockchain: Limit GetStakeVersions count ([decred/dcrd#1071](https://github.com/decred/dcrd/pull/1071))
- blockchain: Remove dry run flag ([decred/dcrd#1073](https://github.com/decred/dcrd/pull/1073))
- blockchain: Remove redundant stake ver calc func ([decred/dcrd#1087](https://github.com/decred/dcrd/pull/1087))
- blockchain: Reduce GetGeneration to TipGeneration ([decred/dcrd#1083](https://github.com/decred/dcrd/pull/1083))
- blockchain: Add chain tip tracking ([decred/dcrd#1084](https://github.com/decred/dcrd/pull/1084))
- blockchain: Switch tip generation to chain tips ([decred/dcrd#1085](https://github.com/decred/dcrd/pull/1085))
- blockchain: Simplify voter version calculation ([decred/dcrd#1088](https://github.com/decred/dcrd/pull/1088))
- blockchain: Remove unused threshold serialization ([decred/dcrd#1092](https://github.com/decred/dcrd/pull/1092))
- blockchain: Simplify chain tip tracking ([decred/dcrd#1092](https://github.com/decred/dcrd/pull/1092))
- blockchain: Cache tip and parent at init ([decred/dcrd#1100](https://github.com/decred/dcrd/pull/1100))
- mining: Obtain block by hash instead of top block ([decred/dcrd#1094](https://github.com/decred/dcrd/pull/1094))
- blockchain: Remove unused GetTopBlock function ([decred/dcrd#1094](https://github.com/decred/dcrd/pull/1094))
- multi: Rename BIP0111Version to NodeBloomVersion ([decred/dcrd#1112](https://github.com/decred/dcrd/pull/1112))
- mining/mempool: Move priority code to mining pkg ([decred/dcrd#1110](https://github.com/decred/dcrd/pull/1110))
- mining: Use single uint64 coinbase extra nonce ([decred/dcrd#1116](https://github.com/decred/dcrd/pull/1116))
- mempool/mining: Clarify tree validity semantics ([decred/dcrd#1118](https://github.com/decred/dcrd/pull/1118))
- mempool/mining: TxSource separation ([decred/dcrd#1119](https://github.com/decred/dcrd/pull/1119))
- connmgr: Use same Dial func signature as net.Dial ([decred/dcrd#1113](https://github.com/decred/dcrd/pull/1113))
- addrmgr: Declutter package API ([decred/dcrd#1124](https://github.com/decred/dcrd/pull/1124))
- mining: Correct initial template generation ([decred/dcrd#1122](https://github.com/decred/dcrd/pull/1122))
- cpuminer: Use header for extra nonce ([decred/dcrd#1123](https://github.com/decred/dcrd/pull/1123))
- addrmgr: Make writing of peers file safer ([decred/dcrd#1126](https://github.com/decred/dcrd/pull/1126))
- addrmgr: Save peers file only if necessary ([decred/dcrd#1127](https://github.com/decred/dcrd/pull/1127))
- addrmgr: Factor out common code ([decred/dcrd#1138](https://github.com/decred/dcrd/pull/1138))
- addrmgr: Improve isBad() performance ([decred/dcrd#1134](https://github.com/decred/dcrd/pull/1134))
- dcrutil: Disallow creation of hybrid P2PK addrs ([decred/dcrd#1154](https://github.com/decred/dcrd/pull/1154))
- chainec/dcrec: Remove hybrid pubkey support ([decred/dcrd#1155](https://github.com/decred/dcrd/pull/1155))
- blockchain: Only fetch inputs once in connect txns ([decred/dcrd#1152](https://github.com/decred/dcrd/pull/1152))
- indexers: Provide interface for index removal ([decred/dcrd#1158](https://github.com/decred/dcrd/pull/1158))

### Testing and Quality Assurance:

- travis: set GOVERSION environment properly ([decred/dcrd#958](https://github.com/decred/dcrd/pull/958))
- stake: Override false positive vet error ([decred/dcrd#960](https://github.com/decred/dcrd/pull/960))
- docs: make example code compile ([decred/dcrd#970](https://github.com/decred/dcrd/pull/970))
- blockchain: Add median time tests ([decred/dcrd#991](https://github.com/decred/dcrd/pull/991))
- chaingen: Update vote commitments on hdr updates ([decred/dcrd#1023](https://github.com/decred/dcrd/pull/1023))
- fullblocktests: Add tests for early final state ([decred/dcrd#1031](https://github.com/decred/dcrd/pull/1031))
- travis: test in docker container ([decred/dcrd#1053](https://github.com/decred/dcrd/pull/1053))
- blockchain: Correct error stringer tests ([decred/dcrd#1066](https://github.com/decred/dcrd/pull/1066))
- blockchain: Remove superfluous reorg tests ([decred/dcrd#1072](https://github.com/decred/dcrd/pull/1072))
- blockchain: Use chaingen for forced reorg tests ([decred/dcrd#1074](https://github.com/decred/dcrd/pull/1074))
- blockchain: Remove superfluous test checks ([decred/dcrd#1075](https://github.com/decred/dcrd/pull/1075))
- blockchain: move block validation rule tests into fullblocktests ([decred/dcrd#1060](https://github.com/decred/dcrd/pull/1060))
- fullblocktests: Cleanup after refactor ([decred/dcrd#1080](https://github.com/decred/dcrd/pull/1080))
- chaingen: Prevent dup block names in NextBlock ([decred/dcrd#1079](https://github.com/decred/dcrd/pull/1079))
- blockchain: Remove duplicate val tests ([decred/dcrd#1082](https://github.com/decred/dcrd/pull/1082))
- chaingen: Break dependency on blockchain ([decred/dcrd#1076](https://github.com/decred/dcrd/pull/1076))
- blockchain: Consolidate tests into the main package ([decred/dcrd#1077](https://github.com/decred/dcrd/pull/1077))
- chaingen: Export vote commitment script function ([decred/dcrd#1081](https://github.com/decred/dcrd/pull/1081))
- fullblocktests: Improve vote on wrong block tests ([decred/dcrd#1081](https://github.com/decred/dcrd/pull/1081))
- chaingen: Export func to check if block is solved ([decred/dcrd#1089](https://github.com/decred/dcrd/pull/1089))
- fullblocktests: Use new exported IsSolved func ([decred/dcrd#1089](https://github.com/decred/dcrd/pull/1089))
- chaingen: Accept mungers for create premine block ([decred/dcrd#1090](https://github.com/decred/dcrd/pull/1090))
- blockchain: Add tests for chain tip tracking ([decred/dcrd#1096](https://github.com/decred/dcrd/pull/1096))
- blockchain: move block validation rule tests into fullblocktests (2/x) ([decred/dcrd#1095](https://github.com/decred/dcrd/pull/1095))
- addrmgr: Remove obsolete coverage script ([decred/dcrd#1103](https://github.com/decred/dcrd/pull/1103))
- chaingen: Track expected blk heights separately ([decred/dcrd#1101](https://github.com/decred/dcrd/pull/1101))
- addrmgr: Improve test coverage ([decred/dcrd#1111](https://github.com/decred/dcrd/pull/1111))
- chaingen: Add revocation generation infrastructure ([decred/dcrd#1120](https://github.com/decred/dcrd/pull/1120))
- fullblocktests: Add some basic revocation tests ([decred/dcrd#1121](https://github.com/decred/dcrd/pull/1121))
- addrmgr: Test removal of corrupt peers file ([decred/dcrd#1129](https://github.com/decred/dcrd/pull/1129))

### Misc:

- release: Bump for v1.2.0 ([decred/dcrd#1140](https://github.com/decred/dcrd/pull/1140))
- goimports -w . ([decred/dcrd#968](https://github.com/decred/dcrd/pull/968))
- dep: sync ([decred/dcrd#980](https://github.com/decred/dcrd/pull/980))
- multi: Simplify code per gosimple linter ([decred/dcrd#993](https://github.com/decred/dcrd/pull/993))
- multi: various cleanups ([decred/dcrd#1019](https://github.com/decred/dcrd/pull/1019))
- multi: release the mutex earlier ([decred/dcrd#1026](https://github.com/decred/dcrd/pull/1026))
- multi: fix some maligned linter warnings ([decred/dcrd#1025](https://github.com/decred/dcrd/pull/1025))
- blockchain: Correct a few log statements ([decred/dcrd#1042](https://github.com/decred/dcrd/pull/1042))
- mempool: cleaner ([decred/dcrd#1050](https://github.com/decred/dcrd/pull/1050))
- multi: fix misspell linter warnings ([decred/dcrd#1054](https://github.com/decred/dcrd/pull/1054))
- dep: sync ([decred/dcrd#1091](https://github.com/decred/dcrd/pull/1091))
- multi: Properly capitalize Decred ([decred/dcrd#1102](https://github.com/decred/dcrd/pull/1102))
- build: Correct semver build handling ([decred/dcrd#1097](https://github.com/decred/dcrd/pull/1097))
- main: Make func definition style consistent ([decred/dcrd#1114](https://github.com/decred/dcrd/pull/1114))
- main: Allow semver prerel via linker flags ([decred/dcrd#1128](https://github.com/decred/dcrd/pull/1128))

### Code Contributors (alphabetical order):

- Andrew Chiw
- Daniel Krawsiz
- Dave Collins
- David Hill
- Donald Adu-Poku
- Javed Khan
- Jolan Luff
- Jon Gillham
- Josh Rickmar
- Markus Richter
- Matheus Degiovani
- Ryan Vacek

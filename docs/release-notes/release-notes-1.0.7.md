## dcrd v1.0.7

This release of dcrd primarily contains improvements to the infrastructure and
other quality assurance changes that are bringing us closer to providing full
support for Lightning Network.

A lot of work required for Lightning Network support went into getting the
required code merged into the upstream project, btcd, which now fully supports
it.  These changes also must be synced and integrated with dcrd as well and
therefore many of the changes in this release are related to that process.

## Notable Changes

### Dust check removed from stake transactions

The standard policy for regular transactions is to reject any transactions that
have outputs so small that they cost more to the network than their value.  This
behavior is desirable for regular transactions, however it was also being
applied to vote and revocation transactions which could lead to a situation
where stake pools with low fees could result in votes and revocations having
difficulty being mined.

This check has been changed to only apply to regular transactions now in order
to prevent any issues.  Stake transactions have several other checks that make
this one unnecessary for them.

### New `feefilter` peer-to-peer message

A new optional peer-to-peer message named `feefilter` has been added that allows
peers to inform others about the minimum transaction fee rate they are willing
to accept.  This will enable peers to avoid notifying others about transactions
they will not accept anyways and therefore can result in a significant bandwidth
savings.

### Bloom filter service bit enforcement

Peers that are configured to disable bloom filter support will now disconnect
remote peers that send bloom filter related commands rather than simply ignoring
them.  This allows any light clients that do not observe the service bit to
potentially find another peer that provides the service.  Additionally, remote
peers that have negotiated a high enough protocol version to observe the service
bit and still send bloom filter related commands anyways will now be banned.


## Changelog

All commits since the last release may be viewed on GitHub [here](https://github.com/decred/dcrd/compare/v1.0.5...v1.0.7).

### Protocol and network:
- Allow reorg of block one [decred/dcrd#745](https://github.com/decred/dcrd/pull/745)
- blockchain: use the time source [decred/dcrd#747](https://github.com/decred/dcrd/pull/747)
- peer: Strictly enforce bloom filter service bit [decred/dcrd#768](https://github.com/decred/dcrd/pull/768)
- wire/peer: Implement feefilter p2p message [decred/dcrd#779](https://github.com/decred/dcrd/pull/779)
- chaincfg: update checkpoints for 1.0.7 release  [decred/dcrd#816](https://github.com/decred/dcrd/pull/816)

### Transaction relay (memory pool):
- mempool: Break dependency on chain instance [decred/dcrd#754](https://github.com/decred/dcrd/pull/754)
- mempool: unexport the mutex [decred/dcrd#755](https://github.com/decred/dcrd/pull/755)
- mempool: Add basic test harness infrastructure [decred/dcrd#756](https://github.com/decred/dcrd/pull/756)
- mempool: Improve tx input standard checks [decred/dcrd#758](https://github.com/decred/dcrd/pull/758)
- mempool: Update comments for dust calcs [decred/dcrd#764](https://github.com/decred/dcrd/pull/764)
- mempool: Only perform standard dust checks on regular transactions  [decred/dcrd#806](https://github.com/decred/dcrd/pull/806)

### RPC:
- Fix gettxout includemempool handling [decred/dcrd#738](https://github.com/decred/dcrd/pull/738)
- Improve help text for getmininginfo [decred/dcrd#748](https://github.com/decred/dcrd/pull/748)
- rpcserverhelp: update TicketFeeInfo help [decred/dcrd#801](https://github.com/decred/dcrd/pull/801)
- blockchain: Improve getstakeversions efficiency [decred/dcrd#81](https://github.com/decred/dcrd/pull/813)

### dcrd command-line flags:
- config: introduce new flags to accept/reject non-std transactions [decred/dcrd#757](https://github.com/decred/dcrd/pull/757)
- config: Add --whitelist option [decred/dcrd#352](https://github.com/decred/dcrd/pull/352)
- config: Improve config file handling [decred/dcrd#802](https://github.com/decred/dcrd/pull/802)
- config: Improve blockmaxsize check [decred/dcrd#810](https://github.com/decred/dcrd/pull/810)

### dcrctl:
- Add --walletrpcserver option [decred/dcrd#736](https://github.com/decred/dcrd/pull/736)

### Documentation
- docs: add commit prefix notes  [decred/dcrd#788](https://github.com/decred/dcrd/pull/788)

### Developer-related package changes:
- blockchain: check errors and remove ineffectual assignments [decred/dcrd#689](https://github.com/decred/dcrd/pull/689)
- stake: less casting [decred/dcrd#705](https://github.com/decred/dcrd/pull/705)
- blockchain: chainstate only needs prev block hash [decred/dcrd#706](https://github.com/decred/dcrd/pull/706)
- remove dead code [decred/dcrd#715](https://github.com/decred/dcrd/pull/715)
- Use btclog for determining valid log levels [decred/dcrd#738](https://github.com/decred/dcrd/pull/738)
- indexers: Minimize differences with upstream code [decred/dcrd#742](https://github.com/decred/dcrd/pull/742)
- blockchain: Add median time to state snapshot [decred/dcrd#753](https://github.com/decred/dcrd/pull/753)
- blockmanager: remove unused GetBlockFromHash function [decred/dcrd#761](https://github.com/decred/dcrd/pull/761)
- mining: call CheckConnectBlock directly [decred/dcrd#762](https://github.com/decred/dcrd/pull/762)
- blockchain: add missing error code entries [decred/dcrd#763](https://github.com/decred/dcrd/pull/763)
- blockchain: Sync main chain flag on ProcessBlock [decred/dcrd#767](https://github.com/decred/dcrd/pull/767)
- blockchain: Remove exported CalcPastTimeMedian func [decred/dcrd#770](https://github.com/decred/dcrd/pull/770)
- blockchain: check for error [decred/dcrd#772](https://github.com/decred/dcrd/pull/772)
- multi: Optimize by removing defers [decred/dcrd#782](https://github.com/decred/dcrd/pull/782)
- blockmanager: remove unused logBlockHeight [decred/dcrd#787](https://github.com/decred/dcrd/pull/787)
- dcrutil: Replace DecodeNetworkAddress with DecodeAddress [decred/dcrd#746](https://github.com/decred/dcrd/pull/746)
- txscript: Force extracted addrs to compressed [decred/dcrd#775](https://github.com/decred/dcrd/pull/775)
- wire: Remove legacy transaction decoding [decred/dcrd#794](https://github.com/decred/dcrd/pull/794)
- wire: Remove dead legacy tx decoding code [decred/dcrd#796](https://github.com/decred/dcrd/pull/796)
- mempool/wire: Don't make policy decisions in wire [decred/dcrd#797](https://github.com/decred/dcrd/pull/797)
- dcrjson: Remove unused cmds & types [decred/dcrd#795](https://github.com/decred/dcrd/pull/795)
- dcrjson: move cmd types [decred/dcrd#799](https://github.com/decred/dcrd/pull/799)
- multi: Separate tx serialization type from version [decred/dcrd#798](https://github.com/decred/dcrd/pull/798)
- dcrjson: add Unconfirmed field to dcrjson.GetAccountBalanceResult [decred/dcrd#812](https://github.com/decred/dcrd/pull/812)
- multi: Error descriptions should be lowercase [decred/dcrd#771](https://github.com/decred/dcrd/pull/771)
- blockchain: cast to int64  [decred/dcrd#817](https://github.com/decred/dcrd/pull/817)

### Testing and Quality Assurance:
- rpcserver: Upstream sync to add basic RPC tests [decred/dcrd#750](https://github.com/decred/dcrd/pull/750)
- rpctest: Correct several issues tests and joins [decred/dcrd#751](https://github.com/decred/dcrd/pull/751)
- rpctest: prevent process leak due to panics [decred/dcrd#752](https://github.com/decred/dcrd/pull/752)
- rpctest: Cleanup resources on failed setup [decred/dcrd#759](https://github.com/decred/dcrd/pull/759)
- rpctest: Use ports based on the process id [decred/dcrd#760](https://github.com/decred/dcrd/pull/760)
- rpctest/deps: Update dependencies and API [decred/dcrd#765](https://github.com/decred/dcrd/pull/765)
- rpctest: Gate rpctest-based behind a build tag [decred/dcrd#766](https://github.com/decred/dcrd/pull/766)
- mempool: Add test for max orphan entry eviction [decred/dcrd#769](https://github.com/decred/dcrd/pull/769)
- fullblocktests: Add more consensus tests [decred/dcrd#77](https://github.com/decred/dcrd/pull/773)
- fullblocktests: Sync upstream block validation [decred/dcrd#774](https://github.com/decred/dcrd/pull/774)
- rpctest: fix a harness range bug in syncMempools [decred/dcrd#778](https://github.com/decred/dcrd/pull/778)
- secp256k1: Add regression tests for field.go [decred/dcrd#781](https://github.com/decred/dcrd/pull/781)
- secp256k1: Sync upstream test consolidation [decred/dcrd#783](https://github.com/decred/dcrd/pull/783)
- txscript: Correct p2sh hashes in json test data  [decred/dcrd#785](https://github.com/decred/dcrd/pull/785)
- txscript: Replace CODESEPARATOR json test data [decred/dcrd#786](https://github.com/decred/dcrd/pull/786)
- txscript: Remove multisigdummy from json test data [decred/dcrd#789](https://github.com/decred/dcrd/pull/789)
- txscript: Remove max money from json test data [decred/dcrd#790](https://github.com/decred/dcrd/pull/790)
- txscript: Update signatures in json test data [decred/dcrd#791](https://github.com/decred/dcrd/pull/791)
- txscript: Use native encoding in json test data [decred/dcrd#792](https://github.com/decred/dcrd/pull/792)
- rpctest: Store logs and data in same path [decred/dcrd#780](https://github.com/decred/dcrd/pull/780)
- txscript: Cleanup reference test code  [decred/dcrd#793](https://github.com/decred/dcrd/pull/793)

### Misc:
- Update deps to pull in additional logging changes [decred/dcrd#734](https://github.com/decred/dcrd/pull/734)
- Update markdown files for GFM changes [decred/dcrd#744](https://github.com/decred/dcrd/pull/744)
- blocklogger: Show votes, tickets, & revocations [decred/dcrd#784](https://github.com/decred/dcrd/pull/784)
- blocklogger: Remove STransactions from transactions calculation [decred/dcrd#811](https://github.com/decred/dcrd/pull/811)

### Contributors (alphabetical order):

- Alex Yocomm-Piatt
- Atri Viss
- Chris Martin
- Dave Collins
- David Hill
- Donald Adu-Poku
- Jimmy Song
- John C. Vernaleo
- Jolan Luff
- Josh Rickmar
- Olaoluwa Osuntokun
- Marco Peereboom

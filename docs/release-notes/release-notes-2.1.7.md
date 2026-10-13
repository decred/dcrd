# dcrd v2.1.7 Release Notes

This is a patch release of dcrd that includes the following notable changes:

- Several fixes for network and mixing-related denial-of-service (DoS) attacks
- Improved mixing performance
- Resolves a memory leak that accumulates slowly over time
- Corrects the test network initial synchronization issue introduced in v2.1.6
- Adds extensive safeguards against unexpected transaction spends

## Upgrade Highly Recommended

Miners, stakeholders, and anyone running a full node are strongly encouraged to
upgrade to this release.  It includes multiple fixes for potential
denial-of-service (DoS) attacks that could disrupt node operation.

Miners, in particular, are at risk of having their template generation stalled.

## Spend Safeguards

In response to the previous inflation bug, this release adds extensive
safeguards designed to detect unexpected transaction spends that should not be
possible.  When these safeguards detect certain unexpected conditions, the
affected node will shut down rather than continue operating in a potentially
unsafe state.

These safeguards would have prevented the previous inflation bug by causing all
affected nodes to halt when they encountered the invalid spend.  They are also
designed to catch a broader range of potential problems that could otherwise go
unnoticed.

## Changelog

This patch release consists of 87 commits from 6 contributors which total to 161
files changed, 7389 additional lines of code, and 4543 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v2.1.6...release-v2.1.7).

### Protocol and network:

- [release-v2.1] wire: Reject invalid tx trees early ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] blockchain: Allow testnet3 sbss violation ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add active agenda anchors ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add basic historical agenda support ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Early data commitment checks ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] netsync: Reset sync height on new sync peer ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))

### Transaction relay (memory pool):

- [release-v2.1] mempool: Reject flagged mixing UTXO spends ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Track votes via separate vote tracker ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Reject votes too far in the future ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Reject bad block votes early ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Reject ineligible ticket votes early ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Prune old vote metadata ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))

### Mixing message relay (mix pool):

- [release-v2.1] mixpool: Refactor activeInEpoch to scale linearly ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixpool: Reject distinct KEs with same identity and session ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixpool: Reject KEs with epochs too far in past ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixpool: Limit max messages per single mixing identity ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixpool: Require KE epoch to be a minute multiple ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixing: Allow more than mixing.MaxPeers in KE messages ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))

### dcrd command-line flags and configuration:

- [release-v2.1] config: Correct --profile port range in help ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))

### Developer-related package and module changes:

- [release-v2.1] wire: fix function name in comment to match actual function ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] multi: Fix godoc comment var/func names ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] wire: Clarify some payload size names ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] wire: Add SerializeSize to Message interface ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] wire: Always use accurate SerializeSize ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] blockchain/stake: Local consts for req script vers ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Cleanup treasury add code ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Cleanup treasury spend code ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Cleanup treasurybase code ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] tickettreap: Fix off-by-one in getByIndex ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] stake: Correct off by one in revoke create helper ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] mixclient: remove shadowed roots declaration ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixclient: Remove unnecessary len check ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixclient: Add missing genDicemixKeys err handling ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixclient: Check SR mix dimensions before recovering pads ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] mixclient: Use shorter 10s stage timeout on testnet ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] blockchain: Always check size and merkle root ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Fix variable shadowing bugs ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Correct a couple of mutexes ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Separate agenda and deployment prep ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Consolidate determining active agenda ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Consolidate stake diff agenda ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Consolidate max block size agenda ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Use choice id in threshold state tuple ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Separate agenda and deployment logic ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add default required agendas ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Process block headers before data ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Deduplicate merkle root checks ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Consolidate chain error wrapping ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Remove incorrect cache assertion ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Combine single and range txout add ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Improve stake spend violation handling ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Consolidate zero conf tx detection ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Use panics for utxoview/cache asserts ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add additional utxo assertions ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))

### Developer-related module management:

- [release-v2.1] wire: Prepare v1.8.0 ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] main: Use backported wire updates ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] blockchain/stake: Prepare v5.0.3 ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] main: Use backported stake updates ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] mixing: Prepare v0.7.5 ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))
- [release-v2.1] main: Use backported mixing updates ([decred/dcrd#3831](https://github.com/decred/dcrd/pull/3831))

### Testing and Quality Assurance:

- [release-v2.1] wire: Rename assertion and remove return ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] wire: Add tests for MsgGetMiningState ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] wire: Add outpoint read/write tests ([decred/dcrd#3829](https://github.com/decred/dcrd/pull/3829))
- [release-v2.1] blockchain/stake: Consolidate and add script tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Remove commented test code ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Rework treasury tx type tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Remove duplicate test ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Rework treasury spend err tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Rework treasury add err tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain/stake: Rework treasurybase err tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] stake: Fix comments in IsTreasuryX tests ([decred/dcrd#3830](https://github.com/decred/dcrd/pull/3830))
- [release-v2.1] blockchain: Add max block size selection tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add default required agendas test ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add positional agenda active tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add historical agenda invariants test ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Consolidate test harness mock block logic ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Add vote metadata assertions ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Add tests for vote height policy ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Add early bad block vote tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Add early ineligible ticket vote test ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Modernize pool membership test helper ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] mempool: Add vote pruning tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add additional zero conf commit tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add utxo set test harness helpers ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add direct extension disapproval tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))
- [release-v2.1] blockchain: Add reorg disapproval tests ([decred/dcrd#3832](https://github.com/decred/dcrd/pull/3832))

### Misc:

- [release-v2.1] release: Bump for 2.1.7 ([decred/dcrd#3833](https://github.com/decred/dcrd/pull/3833))

### Code Contributors (alphabetical order):

- Dave Collins
- Jamie Holdstock
- Josh Rickmar
- Iain
- LuaLighter
- yzxcj797

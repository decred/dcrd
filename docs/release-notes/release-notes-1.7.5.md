# dcrd v1.7.5

This is a patch release of dcrd that updates the utxo cache to improve its
robustness, optimize it, and correct some hard to hit corner cases that involve
a mix of manual block invalidation, conditional flushing, and successive unclean
shutdowns.

## Changelog

This patch release consists of 19 commits from 1 contributor which total to 13
files changed, 1118 additional lines of code, and 484 deleted lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/a2c3c656...release-v1.7.5).

### Developer-related package and module changes:

- blockchain: Misc consistency cleanup pass ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Pre-allocate in-flight utxoview tx map ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Remove unused utxo cache add entry err ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Fix rare unclean utxo cache recovery ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Don't fetch trsy{base,spend} inputs ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Don't add treasurybase utxos ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Separate utxo cache vs view state ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Improve utxo cache spend robustness ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Split regular/stake view tx connect ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Bypass utxo cache for zero conf spends ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- main: Use backported blockchain updates ([decred/dcrd#3007](https://github.com/decred/dcrd/pull/3007))

### Testing and Quality Assurance:

- blockchain: Address some linter complaints ([decred/dcrd#3005](https://github.com/decred/dcrd/pull/3005))
- blockchain: Allow tests to override cache flushing ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Improve utxo cache initialize tests ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Consolidate utxo cache test entries ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Rework utxo cache spend entry tests ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Rework utxo cache commit tests ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))
- blockchain: Rework utxo cache add entry tests ([decred/dcrd#3006](https://github.com/decred/dcrd/pull/3006))

### Misc:

- release: Bump for 1.7.5 ([decred/dcrd#3008](https://github.com/decred/dcrd/pull/3008))

### Code Contributors (alphabetical order):

- Dave Collins

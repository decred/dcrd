# dcrd v{TODO: X.Y}.0 Draft Release Notes

This is a new major release of dcrd.  Some of the key highlights are:

* {TODO: List Consensus Votes}
* {TODO: Key Highlight 1}
* {TODO: Key Highlight 2}
* {TODO: Deprecated and/or removed CLI options}
* {TODO: RPC Changes}
  * {TODO: Non-API-specific and/or important change 1}
  * {TODO: Non-API-specific and/or important change 2}
  * {TODO: Several JSON-RPC API updates, additions, and removals}
* {TODO: Infrastructure improvements}
* {TODO: Quality assurance changes}

{TODO: Only include if there are consensus changes}
For those unfamiliar with the
[voting process](https://docs.decred.org/governance/consensus-rule-voting/overview/)
in Decred, all code needed in order to support each of the aforementioned
consensus changes is already included in this release, however it will remain
dormant until the stakeholders vote to activate it.

{TODO: Links to Politeia proposals}
For reference, the consensus change work for was originally proposed and
approved for initial implementation via the following Politeia proposal:
- {TODO: [Politeia Proposal Title](https://proposals.decred.org/record/#######)}

{TODO: Links to DCPs if there are any consensus changes}
The following Decred Change Proposals (DCPs) describe the proposed changes in
detail and provide full technical specifications:
* {TODO: [DCP####: DCP Title](https://github.com/decred/dcps/blob/master/dcp-####/dcp-####.mediawiki)}
* {TODO: [DCP####: DCP Title](https://github.com/decred/dcps/blob/master/dcp-####/dcp-####.mediawiki)}

{TODO: Only include if true}
## Upgrade Required

**It is extremely important for everyone to upgrade their software to this
latest release even if you don't intend to vote in favor of the agenda.  This
particularly applies to PoW miners as failure to upgrade will result in lost
rewards after block height {TODO: ######}.  That is estimated to be around
{TODO: Month Day, Year}.**

{TODO: Only include if needed}
## Downgrade Warning

The database format in v{TODO: X.Y}.0 is not compatible with previous versions of the
software.  This only affects downgrades as users upgrading from previous
versions will see a one time database migration.

Once this migration has been completed, it will no longer be possible to
downgrade to a previous version of the software without having to delete the
database and redownload the chain.

The database migration typically takes around {TODO: #-#} minutes on HDDs and
{TODO #-#} minutes on SSDs.

## Notable Changes

### {TODO: Consensus Change Votes}

{TODO: Only include if there are votes}
{TODO: #} new consensus change vote{TODO: s are/is} now available as of this
release.  After upgrading, stakeholders may set their preferences through their
wallet.

#### {TODO: Vote 1}

The first new vote available as of this release has the id `{TODO: Vote 1 ID}`.

{TODO: Short description and goals}

See the following for more details:

* {TODO: Politeia proposal link}
* {TODO: DCP link}

#### {TODO: Vote 2}

The second new vote available as of this release has the id `{TODO: Vote 2 ID}`.

{TODO: Short description and goals}

See the following for more details:

* {TODO: Politeia proposal link}
* {TODO: DCP link}

### {TODO: Key Highlight 1}

{TODO: Key highlight 1 user-facing summary}

### {TODO: Key Highlight 2}

{TODO: Key highlight 2 user-facing summary}

### {TODO: More key highlights as needed}

{TODO: Key highlight user-facing summary}

### {TODO: Deprecated and/or Removed CLI Options}

{TODO: Details about deprecated and/or removed CLI options}

### RPC Server Changes

The RPC server version as of this release is {TODO: RPC server version}

#### {TODO: Non-API-specific and/or important change 1}

{TODO: Describe change}

#### {TODO: Non-API-specific and/or important change 2}

{TODO: Describe change}

#### {TODO: API change 1} (`{TODO: affected RPC}`)

{TODO: Describe change}

- [{TODO: RPC method} JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#{TODO: RPC method})

#### {TODO: API change 2} (`{TODO: affected RPC}`)

{TODO: Describe change}

- [{TODO: RPC method} JSON-RPC API Documentation](https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.mediawiki#{TODO: RPC method})

## Changelog

This release consists of {TODO: #} commits from {TODO: #} contributors which total
to {TODO: #} files changed, {TODO: #} additional lines of code, and {TODO: #} deleted
lines of code.

All commits since the last release may be viewed on GitHub
[here](https://github.com/decred/dcrd/compare/release-v{TODO: X.Y.Z}...release-v{TODO: X.Y}.0).

### Protocol and network:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
forced PoW upgrades, seeder additions/removals, protocol versions,
new/deprecated/removed peer-to-peer messages, consensus vote params and
implementation}

### Transaction relay (memory pool):

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
modifications to mempool/relay policy such as fee or standardness changes,
changes to eviction policy, update to ancestor tracking}

### Mining:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
update to latest block ver for consensus votes, changes to how blocks templates
are assembled}

### RPC:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
updates to RPC methods, permission changes, websocket changes}

### dcrd command-line flags and configuration:

{TODO: Add bullet points for each commit w/ PR link}

### Documentation:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
release note additions, updates to READMEs, updates to JSON-RPC API docs}

### Contrib changes:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
updates to everything in contrib folder, docker updates, service file example
upates}

### Developer-related package and module changes:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
almost all commits that do not fall into the other categories, optimizations,
internal method modifications, logging changes, parameter changes, new methods}

### Developer-related module management:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types:
version bumps, starting new major module dev cycle, dependency updates,
preparing modules for new versions, removing main module replacements on release
branch}

### Testing and Quality Assurance:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types: new
tests, reworked tests, benchmarks, test formatting and comments, linter updates,
integration tests}

### Misc:

{TODO: Add bullet points for each commit w/ PR link -- Example commit types: release version bumps for main, misc comment updates, typo
fixes, addressing linter complaints, file formatting}

### Code Contributors (alphabetical order):

- {TODO: Add all code contributors}
- {TODO: Code contributor 2}
- {TODO: Code contributor 3}
- {TODO: ...}

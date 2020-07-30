# dcrd v1.1.2

This release of dcrd primarily contains performance enhancements, infrastructure
improvements, and other quality assurance changes.

While it is not visible in this release, significant infrastructure work has
also been done this release cycle towards porting the Lightning Network (LN)
daemon which will ultimately allow LN payments to be backed by Decred.

## Notable Changes

### Faster Block Validation

A significant portion of block validation involves handling the stake tickets
which form an integral part of Decred's hybrid proof-of-work and proof-of-stake
system.  The code which handles this portion of validation has been
significantly optimized in this release such that overall block validation is
up to approximately 3 times faster depending on the specific underlying hardware
configuration.  This also has a noticeable impact on the speed of the initial
block download process as well as how quickly votes for winning tickets are
submitted to the network.

### Data Carrier Transaction Standardness Policy

The standard policy for transaction relay of data carrier transaction outputs
has been modified to support canonically-encoded small data pushes.  These
outputs are also known as `OP_RETURN` or `nulldata` outputs.  In particular,
single byte small integers data pushes (0-16) are now supported.

## Changelog

All commits since the last release may be viewed on GitHub [here](https://github.com/decred/dcrd/compare/v1.1.0...v1.1.2).

### Protocol and network:
- chaincfg: update checkpoints for 1.1.2 release [decred/dcrd#946](https://github.com/decred/dcrd/pull/946)
- chaincfg: Rename one of the testnet seeders [decred/dcrd#873](https://github.com/decred/dcrd/pull/873)
- stake: treap index perf improvement [decred/dcrd#853](https://github.com/decred/dcrd/pull/853)
- stake: ticket expiry perf improvement [decred/dcrd#853](https://github.com/decred/dcrd/pull/853)

### Transaction relay (memory pool):

- txscript: Correct nulldata standardness check [decred/dcrd#935](https://github.com/decred/dcrd/pull/935)

### RPC:

- rpcserver: searchrawtransactions skip first input for vote tx [decred/dcrd#859](https://github.com/decred/dcrd/pull/859)
- multi: update stakebase tx vin[0] structure [decred/dcrd#859](https://github.com/decred/dcrd/pull/859)
- rpcserver: Fix empty ssgen verbose results [decred/dcrd#871](https://github.com/decred/dcrd/pull/871)
- rpcserver: check for error in getwork request [decred/dcrd#898](https://github.com/decred/dcrd/pull/898)
- multi: Add NoSplitTransaction to purchaseticket [decred/dcrd#904](https://github.com/decred/dcrd/pull/904)
- rpcserver: avoid nested decodescript p2sh addrs [decred/dcrd#929](https://github.com/decred/dcrd/pull/929)
- rpcserver: skip generating certs when nolisten set [decred/dcrd#932](https://github.com/decred/dcrd/pull/932)
- rpc: Add localaddr and relaytxes to getpeerinfo [decred/dcrd#933](https://github.com/decred/dcrd/pull/933)
- rpcserver: update handleSendRawTransaction error handling [decred/dcrd#939](https://github.com/decred/dcrd/pull/939)

### dcrd command-line flags:

- config: add --nofilelogging option [decred/dcrd#872](https://github.com/decred/dcrd/pull/872)

### Documentation:

- rpcclient: Remove docker info from README.md [decred/dcrd#886](https://github.com/decred/dcrd/pull/886)
- bloom: Fix link in README [decred/dcrd#922](https://github.com/decred/dcrd/pull/922)
- doc: tiny fix url [decred/dcrd#928](https://github.com/decred/dcrd/pull/928)
- doc: update go version for example test run in readme [decred/dcrd#936](https://github.com/decred/dcrd/pull/936)

### Developer-related package changes:

- multi: Drop glide, use dep [decred/dcrd#818](https://github.com/decred/dcrd/pull/818)
- txsort: Implement stable tx sorting package  [decred/dcrd#940](https://github.com/decred/dcrd/pull/940)
- coinset: Remove package [decred/dcrd#888](https://github.com/decred/dcrd/pull/888)
- base58: Use new github.com/decred/base58 package [decred/dcrd#888](https://github.com/decred/dcrd/pull/888)
- certgen: Move self signed certificate code into package [decred/dcrd#879](https://github.com/decred/dcrd/pull/879)
- certgen: Add doc.go and README.md [decred/dcrd#883](https://github.com/decred/dcrd/pull/883)
- rpcclient: Allow request-scoped cancellation during Connect [decred/dcrd#880](https://github.com/decred/dcrd/pull/880)
- rpcclient: Import dcrrpcclient repo into rpcclient directory [decred/dcrd#880](https://github.com/decred/dcrd/pull/880)
- rpcclient: json unmarshal into unexported embedded pointer  [decred/dcrd#941](https://github.com/decred/dcrd/pull/941)
- bloom: Copy github.com/decred/dcrutil/bloom to bloom package [decred/dcrd#881](https://github.com/decred/dcrd/pull/881)
- Improve gitignore [decred/dcrd#887](https://github.com/decred/dcrd/pull/887)
- dcrutil: Import dcrutil repo under dcrutil directory [decred/dcrd#888](https://github.com/decred/dcrd/pull/888)
- hdkeychain: Move to github.com/decred/dcrd/hdkeychain [decred/dcrd#892](https://github.com/decred/dcrd/pull/892)
- stake: Add IsStakeSubmission [decred/dcrd#907](https://github.com/decred/dcrd/pull/907)
- txscript: Require SHA256 secret hashes for atomic swaps [decred/dcrd#930](https://github.com/decred/dcrd/pull/930)

### Testing and Quality Assurance:

- gometalinter: run on subpkgs too [decred/dcrd#878](https://github.com/decred/dcrd/pull/878)
- travis: test Gopkg.lock [decred/dcrd#889](https://github.com/decred/dcrd/pull/889)
- hdkeychain: Work around go vet issue with examples [decred/dcrd#890](https://github.com/decred/dcrd/pull/890)
- bloom: Add missing import to examples [decred/dcrd#891](https://github.com/decred/dcrd/pull/891)
- bloom: workaround go vet issue in example [decred/dcrd#895](https://github.com/decred/dcrd/pull/895)
- tests: make lockfile test work locally [decred/dcrd#894](https://github.com/decred/dcrd/pull/894)
- peer: Avoid goroutine leaking during handshake timeout [decred/dcrd#909](https://github.com/decred/dcrd/pull/909)
- travis: add gosimple linter [decred/dcrd#897](https://github.com/decred/dcrd/pull/897)
- multi: Handle detected data race conditions [decred/dcrd#920](https://github.com/decred/dcrd/pull/920)
- travis: add ineffassign linter [decred/dcrd#896](https://github.com/decred/dcrd/pull/896)
- rpctest: Choose flags based on provided params [decred/dcrd#937](https://github.com/decred/dcrd/pull/937)

### Misc:

- gofmt [decred/dcrd#876](https://github.com/decred/dcrd/pull/876)
- dep: sync third-party deps [decred/dcrd#877](https://github.com/decred/dcrd/pull/877)
- Bump for v1.1.2 [decred/dcrd#916](https://github.com/decred/dcrd/pull/916)
- dep: Use upstream jrick/bitset [decred/dcrd#899](https://github.com/decred/dcrd/pull/899)
- blockchain: removed unused funcs and vars [decred/dcrd#900](https://github.com/decred/dcrd/pull/900)
- blockchain: remove unused file [decred/dcrd#900](https://github.com/decred/dcrd/pull/900)
- rpcserver: nil pointer dereference when submit orphan block [decred/dcrd#906](https://github.com/decred/dcrd/pull/906)
- multi: remove unused funcs and vars [decred/dcrd#901](https://github.com/decred/dcrd/pull/901)

### Code Contributors (alphabetical order):

- Alex Yocom-Piatt
- Dave Collins
- David Hill
- detailyang
- Donald Adu-Poku
- Federico Gimenez
- Jason Zavaglia
- John C. Vernaleo
- Jonathan Chappelow
- Jolan Luff
- Josh Rickmar
- Maninder Lall
- Matheus Degiovani
- Nicola Larosa
- Samarth Hattangady
- Ugwueze Onyekachi Michael

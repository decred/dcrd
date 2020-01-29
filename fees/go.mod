module github.com/decred/dcrd/fees/v2

go 1.11

require (
	github.com/decred/dcrd/blockchain/stake/v3 v3.0.0-20200104000002-54b67d3474fb
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200104000002-54b67d3474fb
	github.com/decred/slog v1.0.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/syndtr/goleveldb v1.0.1-0.20190923125748-758128399b1d
)

replace (
	github.com/decred/dcrd/blockchain/stake/v3 => ../blockchain/stake
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/txscript/v3 => ../txscript
)

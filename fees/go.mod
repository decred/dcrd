module github.com/decred/dcrd/fees/v2

go 1.11

require (
	github.com/btcsuite/goleveldb v1.0.0
	github.com/decred/dcrd/blockchain/stake/v3 v3.0.0-00010101000000-000000000000
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-00010101000000-000000000000
	github.com/decred/slog v1.0.0
	github.com/jessevdk/go-flags v1.4.0
)

replace (
	github.com/decred/dcrd/blockchain/stake/v3 => ../blockchain/stake
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/txscript/v3 => ../txscript
)

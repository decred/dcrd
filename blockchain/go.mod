module github.com/decred/dcrd/blockchain/v3

go 1.11

require (
	github.com/decred/dcrd/blockchain/stake/v3 v3.0.0-20200215031403-6b2ce76f0986
	github.com/decred/dcrd/blockchain/standalone v1.1.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v3 v3.0.0-20200215031403-6b2ce76f0986
	github.com/decred/dcrd/database/v2 v2.0.1
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/secp256k1/v3 v3.0.0-20200215031403-6b2ce76f0986
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200215031403-6b2ce76f0986
	github.com/decred/dcrd/gcs/v2 v2.0.0
	github.com/decred/dcrd/txscript/v3 v3.0.0-20200215031403-6b2ce76f0986
	github.com/decred/dcrd/wire v1.3.0
	github.com/decred/slog v1.0.0
)

replace (
	github.com/decred/dcrd/blockchain/stake/v3 => ./stake
	github.com/decred/dcrd/chaincfg/v3 => ../chaincfg
	github.com/decred/dcrd/dcrec/secp256k1/v3 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/txscript/v3 => ../txscript
)

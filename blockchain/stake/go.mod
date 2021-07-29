module github.com/decred/dcrd/blockchain/stake/v4

go 1.13

require (
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v3 v3.0.0
	github.com/decred/dcrd/database/v3 v3.0.0-20210802132946-9ede6ae83e0f
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.0-20210127014238-b33b46cf1a24
	github.com/decred/dcrd/dcrutil/v4 v4.0.0-20210129181600-6ae0142d3b28
	github.com/decred/dcrd/txscript/v4 v4.0.0-20210415215133-96b98390a9a9
	github.com/decred/dcrd/wire v1.4.0
	github.com/decred/slog v1.1.0
)

replace (
	github.com/decred/dcrd/database/v3 => ../../database
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ../../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v4 => ../../dcrutil
	github.com/decred/dcrd/txscript/v4 => ../../txscript
)

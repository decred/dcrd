module github.com/decred/dcrd/hdkeychain/v3

go 1.11

require (
	github.com/decred/base58 v1.0.2
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v3 v3.0.0-20200215015031-3283587e6add
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/secp256k1/v3 v3.0.0-20200215015031-3283587e6add
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200215015031-3283587e6add
)

replace (
	github.com/decred/dcrd/chaincfg/v3 => ../chaincfg
	github.com/decred/dcrd/dcrec/secp256k1/v3 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
)

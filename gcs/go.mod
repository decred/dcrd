module github.com/decred/dcrd/gcs/v3

go 1.13

require (
	github.com/dchest/siphash v1.2.2
	github.com/decred/dcrd/blockchain/stake/v4 v4.0.0-20210129192908-660d0518b4cf
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/crypto/blake256 v1.0.0
	github.com/decred/dcrd/txscript/v4 v4.0.0-20210129190127-4ebd135a82f1
	github.com/decred/dcrd/wire v1.4.0
)

replace (
	github.com/decred/dcrd/blockchain/stake/v4 => ../blockchain/stake
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v4 => ../dcrutil
	github.com/decred/dcrd/txscript/v4 => ../txscript
)

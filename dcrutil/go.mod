module github.com/decred/dcrd/dcrutil/v4

go 1.13

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/base58 v1.0.3
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/crypto/ripemd160 v1.0.1
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.0
	github.com/decred/dcrd/wire v1.4.0
)

replace github.com/decred/dcrd/dcrec/secp256k1/v4 => ../dcrec/secp256k1

module github.com/decred/dcrd/fees

require (
	github.com/btcsuite/goleveldb v1.0.0
	github.com/decred/dcrd/blockchain/stake v1.1.0
	github.com/decred/dcrd/chaincfg v1.2.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/dcrutil v1.2.0
	github.com/decred/slog v1.0.0
	github.com/golang/protobuf v1.2.0 // indirect
	github.com/jessevdk/go-flags v1.4.0
	golang.org/x/sys v0.0.0-20180831094639-fa5fdf94c789 // indirect
)

replace (
	github.com/decred/dcrd/chaincfg => ../chaincfg
	github.com/decred/dcrd/chaincfg/chainhash => ../chaincfg/chainhash
	github.com/decred/dcrd/dcrec => ../dcrec
	github.com/decred/dcrd/dcrec/edwards => ../dcrec/edwards
	github.com/decred/dcrd/dcrec/secp256k1 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil => ../dcrutil
	github.com/decred/dcrd/wire => ../wire
)

module github.com/decred/dcrd/peer/v3

go 1.17

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.4
	github.com/decred/dcrd/lru v1.1.2
	github.com/decred/dcrd/txscript/v4 v4.1.0
	github.com/decred/dcrd/wire v1.6.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.2.0
)

replace github.com/decred/dcrd/wire => ../wire

require (
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.1 // indirect
	github.com/decred/dcrd/crypto/ripemd160 v1.0.2 // indirect
	github.com/decred/dcrd/dcrec v1.0.1 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.3 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

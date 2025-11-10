module github.com/decred/dcrd

go 1.19

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/base58 v1.0.5
	github.com/decred/dcrd/addrmgr/v3 v3.0.0
	github.com/decred/dcrd/bech32 v1.1.4
	github.com/decred/dcrd/blockchain/stake/v5 v5.0.1
	github.com/decred/dcrd/blockchain/standalone/v2 v2.2.1
	github.com/decred/dcrd/blockchain/v5 v5.0.1
	github.com/decred/dcrd/certgen v1.1.3
	github.com/decred/dcrd/chaincfg/chainhash v1.0.5
	github.com/decred/dcrd/chaincfg/v3 v3.2.1
	github.com/decred/dcrd/connmgr/v3 v3.1.2
	github.com/decred/dcrd/container/apbf v1.0.1
	github.com/decred/dcrd/container/lru v1.0.0
	github.com/decred/dcrd/crypto/blake256 v1.1.0
	github.com/decred/dcrd/crypto/rand v1.0.1
	github.com/decred/dcrd/crypto/ripemd160 v1.0.2
	github.com/decred/dcrd/database/v3 v3.0.2
	github.com/decred/dcrd/dcrec v1.0.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0
	github.com/decred/dcrd/dcrjson/v4 v4.1.0
	github.com/decred/dcrd/dcrutil/v4 v4.0.2
	github.com/decred/dcrd/gcs/v4 v4.1.0
	github.com/decred/dcrd/math/uint256 v1.0.2
	github.com/decred/dcrd/mixing v0.3.0
	github.com/decred/dcrd/peer/v3 v3.1.1
	github.com/decred/dcrd/rpc/jsonrpc/types/v4 v4.2.0
	github.com/decred/dcrd/rpcclient/v8 v8.0.1
	github.com/decred/dcrd/txscript/v4 v4.1.1
	github.com/decred/dcrd/wire v1.7.1
	github.com/decred/dcrtest/dcrdtest v1.0.1-0.20240404170936-a2529e936df1
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.2.0
	github.com/gorilla/websocket v1.5.1
	github.com/jessevdk/go-flags v1.5.0
	github.com/jrick/bitset v1.0.0
	github.com/jrick/logrotate v1.0.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	golang.org/x/net v0.28.0
	golang.org/x/sys v0.30.0
	golang.org/x/term v0.29.0
	lukechampine.com/blake3 v1.3.0
)

require (
	decred.org/cspp/v2 v2.4.0 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/companyzero/sntrup4591761 v0.0.0-20220309191932-9e0f3af2f07a // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.3 // indirect
	github.com/decred/dcrd/hdkeychain/v3 v3.1.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/text v0.22.0 // indirect
)

replace (
	github.com/decred/dcrd/addrmgr/v3 => ./addrmgr
	github.com/decred/dcrd/bech32 => ./bech32
	github.com/decred/dcrd/blockchain/stake/v5 => ./blockchain/stake
	github.com/decred/dcrd/blockchain/standalone/v2 => ./blockchain/standalone
	github.com/decred/dcrd/blockchain/v5 => ./blockchain
	github.com/decred/dcrd/certgen => ./certgen
	github.com/decred/dcrd/chaincfg/chainhash => ./chaincfg/chainhash
	github.com/decred/dcrd/chaincfg/v3 => ./chaincfg
	github.com/decred/dcrd/connmgr/v3 => ./connmgr
	github.com/decred/dcrd/container/apbf => ./container/apbf
	github.com/decred/dcrd/container/lru => ./container/lru
	github.com/decred/dcrd/crypto/blake256 => ./crypto/blake256
	github.com/decred/dcrd/crypto/rand => ./crypto/rand
	github.com/decred/dcrd/crypto/ripemd160 => ./crypto/ripemd160
	github.com/decred/dcrd/database/v3 => ./database
	github.com/decred/dcrd/dcrec => ./dcrec
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ./dcrec/secp256k1
	github.com/decred/dcrd/dcrjson/v4 => ./dcrjson
	github.com/decred/dcrd/dcrutil/v4 => ./dcrutil
	github.com/decred/dcrd/gcs/v4 => ./gcs
	github.com/decred/dcrd/hdkeychain/v3 => ./hdkeychain
	github.com/decred/dcrd/limits => ./limits
	github.com/decred/dcrd/math/uint256 => ./math/uint256
	github.com/decred/dcrd/mixing => ./mixing
	github.com/decred/dcrd/peer/v3 => ./peer
	github.com/decred/dcrd/rpc/jsonrpc/types/v4 => ./rpc/jsonrpc/types
	github.com/decred/dcrd/rpcclient/v8 => ./rpcclient
	github.com/decred/dcrd/txscript/v4 => ./txscript
	github.com/decred/dcrd/wire => ./wire
)

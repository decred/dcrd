module github.com/decred/dcrd

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/base58 v1.0.3
	github.com/decred/dcrd/addrmgr/v2 v2.0.0-20211005210707-931a579e127b
	github.com/decred/dcrd/bech32 v1.1.1
	github.com/decred/dcrd/blockchain/stake/v4 v4.0.0
	github.com/decred/dcrd/blockchain/standalone/v2 v2.0.0
	github.com/decred/dcrd/blockchain/v4 v4.0.0-20210129200153-14fd1a785bf2
	github.com/decred/dcrd/certgen v1.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.3
	github.com/decred/dcrd/chaincfg/v3 v3.1.0
	github.com/decred/dcrd/connmgr/v3 v3.0.0
	github.com/decred/dcrd/container/apbf v1.0.0
	github.com/decred/dcrd/crypto/ripemd160 v1.0.1
	github.com/decred/dcrd/database/v3 v3.0.0
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
	github.com/decred/dcrd/dcrjson/v4 v4.0.0
	github.com/decred/dcrd/dcrutil/v4 v4.0.0
	github.com/decred/dcrd/gcs/v3 v3.0.0
	github.com/decred/dcrd/hdkeychain/v3 v3.0.1-0.20210129190127-4ebd135a82f1
	github.com/decred/dcrd/lru v1.1.0
	github.com/decred/dcrd/math/uint256 v1.0.0
	github.com/decred/dcrd/peer/v3 v3.0.0-20210802141345-893802fc06b0
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 v3.0.0
	github.com/decred/dcrd/rpcclient/v7 v7.0.0
	github.com/decred/dcrd/txscript/v4 v4.0.0
	github.com/decred/dcrd/wire v1.5.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/bitset v1.0.0
	github.com/jrick/logrotate v1.0.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68
)

replace (
	github.com/decred/dcrd/addrmgr/v2 => ./addrmgr
	github.com/decred/dcrd/bech32 => ./bech32
	github.com/decred/dcrd/blockchain/stake/v4 => ./blockchain/stake
	github.com/decred/dcrd/blockchain/standalone/v2 => ./blockchain/standalone
	github.com/decred/dcrd/blockchain/v4 => ./blockchain
	github.com/decred/dcrd/certgen => ./certgen
	github.com/decred/dcrd/chaincfg/chainhash => ./chaincfg/chainhash
	github.com/decred/dcrd/chaincfg/v3 => ./chaincfg
	github.com/decred/dcrd/connmgr/v3 => ./connmgr
	github.com/decred/dcrd/container/apbf => ./container/apbf
	github.com/decred/dcrd/crypto/blake256 => ./crypto/blake256
	github.com/decred/dcrd/crypto/ripemd160 => ./crypto/ripemd160
	github.com/decred/dcrd/database/v3 => ./database
	github.com/decred/dcrd/dcrec => ./dcrec
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ./dcrec/secp256k1
	github.com/decred/dcrd/dcrjson/v4 => ./dcrjson
	github.com/decred/dcrd/dcrutil/v4 => ./dcrutil
	github.com/decred/dcrd/gcs/v3 => ./gcs
	github.com/decred/dcrd/hdkeychain/v3 => ./hdkeychain
	github.com/decred/dcrd/limits => ./limits
	github.com/decred/dcrd/lru => ./lru
	github.com/decred/dcrd/math/uint256 => ./math/uint256
	github.com/decred/dcrd/peer/v3 => ./peer
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 => ./rpc/jsonrpc/types
	github.com/decred/dcrd/rpcclient/v7 => ./rpcclient
	github.com/decred/dcrd/txscript/v4 => ./txscript
	github.com/decred/dcrd/wire => ./wire
)

module github.com/decred/dcrd

go 1.11

require (
	github.com/btcsuite/winsvc v1.0.0
	github.com/decred/base58 v1.0.0
	github.com/decred/dcrd/addrmgr v1.0.2
	github.com/decred/dcrd/blockchain v1.1.1
	github.com/decred/dcrd/blockchain/stake v1.2.0
	github.com/decred/dcrd/blockchain/standalone v1.0.0
	github.com/decred/dcrd/certgen v1.0.2
	github.com/decred/dcrd/chaincfg v1.5.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/connmgr v1.0.2
	github.com/decred/dcrd/database v1.1.0
	github.com/decred/dcrd/database/v2 v2.0.0
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/secp256k1 v1.0.2
	github.com/decred/dcrd/dcrjson/v2 v2.2.0
	github.com/decred/dcrd/dcrjson/v3 v3.0.0
	github.com/decred/dcrd/dcrutil v1.4.0
	github.com/decred/dcrd/fees v1.0.0
	github.com/decred/dcrd/gcs v1.0.2
	github.com/decred/dcrd/hdkeychain/v2 v2.0.0
	github.com/decred/dcrd/lru v1.0.0
	github.com/decred/dcrd/mempool/v2 v2.0.0
	github.com/decred/dcrd/mining v1.1.0
	github.com/decred/dcrd/peer v1.1.0
	github.com/decred/dcrd/rpc/jsonrpc/types v1.0.0
	github.com/decred/dcrd/rpcclient/v3 v3.0.0
	github.com/decred/dcrd/txscript v1.1.0
	github.com/decred/dcrd/wire v1.2.0
	github.com/decred/dcrwallet/rpc/jsonrpc/types v1.1.0
	github.com/decred/go-socks v1.0.0
	github.com/decred/slog v1.0.0
	github.com/gorilla/websocket v1.4.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/bitset v1.0.0
	github.com/jrick/logrotate v1.0.0
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8
)

replace (
	github.com/decred/dcrd/addrmgr => ./addrmgr
	github.com/decred/dcrd/blockchain => ./blockchain
	github.com/decred/dcrd/blockchain/standalone => ./blockchain/standalone
	github.com/decred/dcrd/certgen => ./certgen
	github.com/decred/dcrd/chaincfg/chainhash => ./chaincfg/chainhash
	github.com/decred/dcrd/chaincfg/v2 => ./chaincfg
	github.com/decred/dcrd/connmgr => ./connmgr
	github.com/decred/dcrd/database/v2 => ./database
	github.com/decred/dcrd/dcrec => ./dcrec
	github.com/decred/dcrd/dcrjson/v3 => ./dcrjson
	github.com/decred/dcrd/dcrutil/v2 => ./dcrutil
	github.com/decred/dcrd/fees => ./fees
	github.com/decred/dcrd/gcs => ./gcs
	github.com/decred/dcrd/hdkeychain/v2 => ./hdkeychain
	github.com/decred/dcrd/limits => ./limits
	github.com/decred/dcrd/lru => ./lru
	github.com/decred/dcrd/mempool/v2 => ./mempool
	github.com/decred/dcrd/mining => ./mining
	github.com/decred/dcrd/peer => ./peer
	github.com/decred/dcrd/rpc/jsonrpc/types => ./rpc/jsonrpc/types
	github.com/decred/dcrd/txscript/v2 => ./txscript
	github.com/decred/dcrd/wire => ./wire
)

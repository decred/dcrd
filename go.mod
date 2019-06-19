module github.com/decred/dcrd

go 1.11

require (
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd
	github.com/btcsuite/winsvc v1.0.0
	github.com/decred/base58 v1.0.0
	github.com/decred/dcrd/addrmgr v1.0.2
	github.com/decred/dcrd/blockchain v1.1.1
	github.com/decred/dcrd/blockchain/stake v1.1.0
	github.com/decred/dcrd/certgen v1.0.2
	github.com/decred/dcrd/chaincfg v1.5.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/connmgr v1.0.2
	github.com/decred/dcrd/database v1.0.3
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrec/secp256k1 v1.0.2
	github.com/decred/dcrd/dcrjson/v2 v2.0.0
	github.com/decred/dcrd/dcrutil v1.3.0
	github.com/decred/dcrd/fees v1.0.0
	github.com/decred/dcrd/gcs v1.0.2
	github.com/decred/dcrd/hdkeychain/v2 v2.0.0
	github.com/decred/dcrd/lru v1.0.0
	github.com/decred/dcrd/mempool/v2 v2.0.0
	github.com/decred/dcrd/mining v1.1.0
	github.com/decred/dcrd/peer v1.1.0
	github.com/decred/dcrd/rpcclient/v2 v2.0.0
	github.com/decred/dcrd/txscript v1.1.0
	github.com/decred/dcrd/wire v1.2.0
	github.com/decred/dcrwallet/rpc/jsonrpc/types v1.0.0
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
	github.com/decred/dcrd/blockchain/stake => ./blockchain/stake
	github.com/decred/dcrd/certgen => ./certgen
	github.com/decred/dcrd/chaincfg/chainhash => ./chaincfg/chainhash
	github.com/decred/dcrd/connmgr => ./connmgr
	github.com/decred/dcrd/database => ./database
	github.com/decred/dcrd/dcrec => ./dcrec
	github.com/decred/dcrd/dcrjson/v2 => ./dcrjson
	github.com/decred/dcrd/fees => ./fees
	github.com/decred/dcrd/gcs => ./gcs
	github.com/decred/dcrd/hdkeychain/v2 => ./hdkeychain
	github.com/decred/dcrd/limits => ./limits
	github.com/decred/dcrd/lru => ./lru
	github.com/decred/dcrd/mempool/v2 => ./mempool
	github.com/decred/dcrd/mining => ./mining
	github.com/decred/dcrd/peer => ./peer
	github.com/decred/dcrd/rpcclient/v2 => ./rpcclient
	github.com/decred/dcrd/wire => ./wire
)

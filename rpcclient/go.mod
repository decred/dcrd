module github.com/decred/dcrd/rpcclient/v7

go 1.16

require (
	github.com/decred/dcrd/chaincfg/chainhash v1.0.3
	github.com/decred/dcrd/dcrjson/v4 v4.0.0
	github.com/decred/dcrd/dcrutil/v4 v4.0.0-20210129181600-6ae0142d3b28
	github.com/decred/dcrd/gcs/v3 v3.0.0-20210916172859-ca03de05ecd0
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 v3.0.0-20210129200153-14fd1a785bf2
	github.com/decred/dcrd/txscript/v4 v4.0.0-20210415215133-96b98390a9a9
	github.com/decred/dcrd/wire v1.4.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.1.0
	github.com/gorilla/websocket v1.4.2
)

replace (
	github.com/decred/dcrd/blockchain/stake/v4 => ../blockchain/stake
	github.com/decred/dcrd/database/v3 => ../database
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrjson/v4 => ../dcrjson
	github.com/decred/dcrd/dcrutil/v4 => ../dcrutil
	github.com/decred/dcrd/gcs/v3 => ../gcs
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 => ../rpc/jsonrpc/types
	github.com/decred/dcrd/txscript/v4 => ../txscript
)

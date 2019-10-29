module github.com/decred/dcrd/rpcclient/v6

go 1.11

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrjson/v3 v3.0.1
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200104000002-54b67d3474fb
	github.com/decred/dcrd/gcs/v2 v2.0.0
	github.com/decred/dcrd/hdkeychain/v2 v2.1.0
	github.com/decred/dcrd/rpc/jsonrpc/types v1.0.1
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 v2.0.0
	github.com/decred/dcrd/wire v1.3.0
	github.com/decred/dcrwallet/rpc/jsonrpc/types v1.3.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.0.0
	github.com/gorilla/websocket v1.4.1
)

replace (
	github.com/decred/dcrd/chaincfg/v3 => ../chaincfg
	github.com/decred/dcrd/dcrec/secp256k1/v3 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 => ../rpc/jsonrpc/types
)

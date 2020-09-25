module github.com/decred/dcrd/rpcclient/v6

go 1.13

require (
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrjson/v3 v3.1.0
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200503044000-76f6906e50e5
	github.com/decred/dcrd/gcs/v2 v2.0.1
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 v2.0.1-0.20200503044000-76f6906e50e5
	github.com/decred/dcrd/wire v1.4.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.0.0
	github.com/gorilla/websocket v1.4.2
)

replace (
	github.com/decred/dcrd/chaincfg/v3 => ../chaincfg
	github.com/decred/dcrd/dcrec/secp256k1/v3 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/hdkeychain/v3 => ../hdkeychain
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 => ../rpc/jsonrpc/types
)

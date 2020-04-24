module github.com/decred/dcrd/rpcclient/v6

go 1.13

require (
	decred.org/dcrwallet v1.2.3-0.20200424154945-f5b98d7c5d08
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrjson/v3 v3.0.1
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200311044114-143c1884e4c8
	github.com/decred/dcrd/gcs/v2 v2.0.2-0.20200312171759-0a8cc56a776e
	github.com/decred/dcrd/hdkeychain/v3 v3.0.0-20200421213827-b60c60ffe98b
	github.com/decred/dcrd/rpc/jsonrpc/types v1.0.1
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 v2.0.0
	github.com/decred/dcrd/wire v1.3.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.0.0
	github.com/gorilla/websocket v1.4.1
)

replace (
	github.com/decred/dcrd/chaincfg/v3 => ../chaincfg
	github.com/decred/dcrd/dcrec/secp256k1/v3 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v3 => ../dcrutil
	github.com/decred/dcrd/hdkeychain/v3 => ../hdkeychain
	github.com/decred/dcrd/rpc/jsonrpc/types/v2 => ../rpc/jsonrpc/types
)

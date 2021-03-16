module github.com/decred/dcrd/rpcclient/v7

go 1.13

require (
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/dcrjson/v3 v3.1.0
	github.com/decred/dcrd/dcrutil/v4 v4.0.0-20210129181600-6ae0142d3b28
	github.com/decred/dcrd/gcs/v3 v3.0.0-20210129195202-a4265d63b619
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 v3.0.0-20210129200153-14fd1a785bf2
	github.com/decred/dcrd/txscript/v4 v4.0.0-20210330065944-a2366e6e0b3b
	github.com/decred/dcrd/wire v1.4.0
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.1.0
	github.com/gorilla/websocket v1.4.2
)

replace (
	github.com/decred/dcrd/blockchain/stake/v4 => ../blockchain/stake
	github.com/decred/dcrd/dcrec/secp256k1/v4 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil/v4 => ../dcrutil
	github.com/decred/dcrd/gcs/v3 => ../gcs
	github.com/decred/dcrd/rpc/jsonrpc/types/v3 => ../rpc/jsonrpc/types
	github.com/decred/dcrd/txscript/v4 => ../txscript
)

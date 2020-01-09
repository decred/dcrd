module github.com/decred/dcrd/database/v2

go 1.11

require (
	github.com/btcsuite/goleveldb v1.0.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v2 v2.3.0
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200104000002-54b67d3474fb
	github.com/decred/dcrd/wire v1.3.0
	github.com/decred/slog v1.0.0
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/jessevdk/go-flags v1.4.0
	golang.org/x/sys v0.0.0-20191010194322-b09406accb47 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)

replace github.com/decred/dcrd/dcrutil/v3 => ../dcrutil

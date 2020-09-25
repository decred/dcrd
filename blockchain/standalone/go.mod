module github.com/decred/dcrd/blockchain/standalone/v2

go 1.11

require (
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/txscript/v3 v3.0.0-20200611204838-4c5825cf9054
	github.com/decred/dcrd/wire v1.4.0
)

replace github.com/decred/dcrd/txscript/v3 => ../../txscript

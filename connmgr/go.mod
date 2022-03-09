module github.com/decred/dcrd/connmgr/v3

go 1.13

require (
	github.com/decred/dcrd/addrmgr/v2 v2.0.0
	github.com/decred/dcrd/wire v1.5.0
	github.com/decred/slog v1.2.0
)

replace github.com/decred/dcrd/addrmgr/v2 => ../addrmgr

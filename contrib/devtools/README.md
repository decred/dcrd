devtools
========

## Overview

This consists of developer tools which may be useful when working with dcrd and
related software.

## Contents

### Simulation Network (--simnet) Preconfigured Environment Setup Script

The [dcr_tmux_simnet_setup.sh](./dcr_tmux_simnet_setup.sh)
script provides a preconfigured `simnet` environment which facilitates testing
with a private test network where the developer has full control since the
difficulty levels are low enough to generate blocks on demand and the developer
owns all of the tickets and votes on the private network.

The environment will be housed in the `$HOME/dcrdsimnetnodes` directory by
default.  This can be overridden with the `DCR_SIMNET_ROOT` environment variable
if desired.

See the full [Simulation Network Reference](../../docs/simnet_environment.mediawiki)
for more details.

### Go Multi-Module Workspace Setup Script

The [dcr_setup_go_workspace.sh](./dcr_setup_go_workspace.sh)
script initializes a Go multi-module workspace (via `go work init`) and adds all
of the modules provided by the dcrd repository to it (via `go work use`) on an
as needed basis.  Note that workspaces require Go 1.18+.

This is useful when developing across multiple modules in the repository and
allows development environments, such as VSCode, that make use of the Go
language server (aka `gopls`) to provide full support without also needing to
temporarily create replacements in the various `go.mod` files or individually
add every module.

Do note, however, that workspaces are local, so final submissions to the
repository will still require the appropriate changes to the relevant `go.mod`
files to ensure resolution outside of the workspace.

### Docker Image Version Bump Script

The [bump_docker.sh](./bump_docker.sh) script creates a `git` commit on a new
branch that updates the [Dockerfile](../docker/Dockerfile) to the provided image
and digest.  The commit description includes the relevant details and
instructions for others to verify the digest.

### Assumed Valid Block Bump Script

The [bump_assumevalid.sh](./bump_assumevalid.sh) script queries the main and
test networks with `dcrctl` to determine suitable updated blocks to use for
their assumevalid block and then creates a `git` commit on a new branch that
updates the [chaincfg/mainnetparams.go](../../chaincfg/mainnetparams.go) and
[chaincfg/testnetparams.go](../../chaincfg/testnetparams.go) files accordingly.
The commit description includes the relevant details and instructions for others
to verify the results.

**NOTE**: This script requires `dcrctl` to be configured such that `dcrctl` and
`dcrctl --testnet` connect to up-to-date main and test network instances,
respectively.

### Minimum Known Chain Work Bump Script

The [bump_minknownchainwork.sh](./bump_minknownchainwork.sh) script queries the
main and test networks with `dcrctl` and `jq` to determine their current best
known chain work values and then creates a `git` commit on a new branch that
updates the [chaincfg/mainnetparams.go](../../chaincfg/mainnetparams.go) and
[chaincfg/testnetparams.go](../../chaincfg/testnetparams.go) files accordingly.
The commit description includes the relevant details and instructions for others
to verify the results.

**NOTE**: This script requires `dcrctl` to be configured such that `dcrctl` and
`dcrctl --testnet` connect to up-to-date main and test network instances,
respectively.  It also requires the [jq](https://jqlang.github.io/jq/) utility
to be available in the system path.

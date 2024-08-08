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

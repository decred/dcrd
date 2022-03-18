contrib
=======

## Overview

This consists of extra optional tools which may be useful when working with dcrd
and related software.

## Contents

### Example Service Configurations

- [OpenBSD rc.d](services/rc.d/dcrd)  
  Provides an example `rc.d` script for configuring dcrd as a background service
  on OpenBSD.  It also serves as a good starting point for other operating
  systems that use the rc.d system for service management.

- [Service Management Facility](services/smf/dcrd.xml)  
  Provides an example XML file for configuring dcrd as a background service on
  illumos.  It also serves as a good starting point for other operating systems
  that use use SMF for service management.

- [systemd](services/systemd/dcrd.service)  
  Provides an example service file for configuring dcrd as a background service
  on operating systems that use systemd for service management.

### Simulation Network (--simnet) Preconfigured Environment Setup Script

The [dcr_tmux_simnet_setup.sh](./dcr_tmux_simnet_setup.sh) script provides a
preconfigured `simnet` environment which facilitates testing with a private test
network where the developer has full control since the difficulty levels are low
enough to generate blocks on demand and the developer owns all of the tickets
and votes on the private network.

The environment will be housed in the `$HOME/dcrdsimnetnodes` directory by
default.  This can be overridden with the `DCR_SIMNET_ROOT` environment variable
if desired.

See the full [Simulation Network Reference](../docs/simnet_environment.mediawiki)
for more details.

### Building and Running OCI Containers (aka Docker/Podman)

The project does not officially provide container images.  However, all of the
necessary files to build your own lightweight non-root container image based on
`scratch` from the latest source code are available in the docker directory.
See [docker/README.md](./docker/README.md) for more details.

### Go Multi-Module Workspace Setup Script

The [dcr_setup_go_workspace.sh](./dcr_setup_go_workspace.sh) script initializes
a Go multi-module workspace (via `go work init`) and adds all of the modules
provided by the dcrd repository to it (via `go work use`) on an as needed basis.
Note that workspaces require Go 1.18+.

This is useful when developing across multiple modules in the repository and
allows development environments that make use of the Go language server (aka
`gopls`), such as VSCode, to provide full support without also needing to
temporarily create replacements in the various `go.mod` files or individually
add every module.

Do note, however, that workspaces are local, so final submissions to the
repository will still require the appropriate changes to the relevant `go.mod`
files to ensure resolution outside of the workspace.
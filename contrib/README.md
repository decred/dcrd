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

### Building and Running OCI Containers (aka Docker/Podman)

The project does not officially provide container images.  However, all of the
necessary files to build your own lightweight non-root container image based on
`scratch` from the latest source code are available in the docker directory.
See [docker/README.md](./docker/README.md) for more details.

### Developer Tools

Several developer tools are available in the [devtools](./devtools) directory.  See
[devtools/README.md](./devtools/README.md) for more details.


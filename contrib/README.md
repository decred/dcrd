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

See the full [Simulation Network Reference](../docs/simnet_environment.mediawiki)
for more details.

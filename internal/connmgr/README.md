connmgr
=======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/connmgr)

## Overview

Package `connmgr` provides a flexible and robust context-aware connection
manager for inbound, outbound, and persistent network connections with retry
logic.

It handles all general connection lifecycle concerns such as accepting inbound
connections, automatically maintaining a set number of outbound connections,
maintaining persistent connections, enforcing limits, and preventing duplicates.

The design has a strong emphasis on reliability, readability, and efficiency under high connection load while also aiming to provide an ergonomic API.

The following is a brief overview of the key features:

- Full context support
- Inbound listening
  - Accepts inbound connections on provided `Listeners`
  - Uses connection shedding for rejected inbound connections
- Automatic outbound maintenance
  - Maintains up to `TargetOutbound` normal outbound connections via a provided
    address source (`GetNewAddress`)
  - Strongly prefers connections to different network segments
  - Incorporates intelligent address selection
    - Skips addresses in already-connected outbound groups
    - Skips recently attempted addresses unless no suitable addresses are found
      after enough retries
    - Prefers default peer-to-peer port addresses (configurable via `DefaultPort`)
- Persistent connections
  - Maintains up to `MaxPersistent` addresses that are automatically retried
    with exponential backoff and jitter on disconnect
- Manual connections
  - Supports manual connection establishment via `Connect`
- Connection limits
  - Limits total normal (non-persistent) connections to `MaxNormalConns`
  - Limits per-host connections to `MaxConnsPerHost` with exemptions for
    whitelisted and loopback addresses
- Duplicate address prevention
  - Rejects duplicate connections to and from the same address (host:port)
- Whitelist support
  - CIDR-based whitelists that allow bypassing certain limits and restrictions
- Rich managed connections via `Conn`
  - Connection types for differentiated handling
  - Automatic cleanup on connection close
  - Concrete parsed address access
- Manual disconnection and removal
  - Ability to disconnect / remove established, pending, and persistent
    connections via `Disconnect` and `Remove`
- Notification callbacks
  - Provides callbacks for connection establishment and disconnects
- Graceful network outage handling
  - Automatic connection attempts are throttled during network outages
- Clear and actionable programatically-detectable errors

A full suite of tests is provided to help ensure proper functionality.

## License

Package connmgr is licensed under the [copyfree](http://copyfree.org) ISC License.

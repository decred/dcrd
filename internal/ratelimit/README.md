ratelimit
=========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/ratelimit)

## Overview

Package `ratelimit` implements a simple concurrent safe token bucket rate
limiter.

It is ideal for traffic shaping, rate limiting API calls, and controlling
resource usage while supporting controlled bursts of activity.

Since rate limiting is often in the hot path and exposed to extreme conditions,
the package is designed to be highly efficient, use minimal memory, and support
high concurrency.

The API is currently primarily aimed at serving use cases where the intention is
to drop events that exceed the rate limit.  However, it also provides the minimal
information needed for callers to manually implement blocking behavior until the
next event is allowed if desired.

Comprehensive tests are included to ensure proper functionality.

All dcrd code in the main module are expected to use this package over
golang.org/x/time/rate.  This package is more efficient, tailored specifically
to the needs of dcrd, and it avoids an extra dependency on the x packages that
have a release policy that conflicts with dcrd's release policy.

## Creating a Rate Limiter

Use `New` to create a limiter with a desired rate (events per second) and
burst size.

For example, a rate of 10.5 with a burst size of 30 would allow an average of
10.5 events per second with periodic bursts of up to 30 events.

In order to rate limit events to every `x` seconds (versus `x` events per
second), specify the rate scaled by `1/x` and scale the rate accordingly for
other time units.

For example, to specify a rate of 15 events per minute, the rate would be 900
`(15*60)` and 450 events every 2 hours would be 0.0625 (`450/(2*3600)`).

## Using the Limiter

Call `Allow` to determine whether an event is permitted at the current time.  It
returns `true` when an event may proceed and automatically consumes a token.

For use cases where blocking until the next event is allowed is preferred
instead of merely dropping events, use `UntilNextAllowed` to determine how long
to wait.

## Querying State

The current number of tokens in the bucket can be obtained with `Tokens` and the
fixed maximum burst size is available via `Burst`.

## Examples

* [Basic Limiter Usage](https://pkg.go.dev/github.com/decred/dcrd/internal/ratelimit#example-Limiter.Allow)
  Demonstrates creating and using a rate limiter that allows up to 10 events
  per second with periodic bursts of up to 25 events.

## License

Package ratelimit is licensed under the [copyfree](http://copyfree.org) ISC
License.

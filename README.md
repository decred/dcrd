dcrd
====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd)
[![Go Report Card](https://goreportcard.com/badge/github.com/decred/dcrd)](https://goreportcard.com/report/github.com/decred/dcrd)

## Decred Overview

Decred is a blockchain-based cryptocurrency with a strong focus on community
input, open governance, and sustainable funding for development. It utilizes a
hybrid proof-of-work and proof-of-stake mining system to ensure that a small
group cannot dominate the flow of transactions or make changes to Decred without
the input of the community.  A unit of the currency is called a `decred` (DCR).

https://decred.org

## Latest Downloads

https://decred.org/downloads/

Core software:

* dcrd: a Decred full node daemon (this)
* [dcrwallet](https://github.com/decred/dcrwallet): a CLI Decred wallet daemon
* [dcrctl](https://github.com/decred/dcrctl): a CLI client for dcrd and dcrwallet

Bundles:

* [Decrediton](https://github.com/decred/decrediton): a GUI bundle for `dcrd`
  and `dcrwallet`
* [CLI app suite](https://github.com/decred/decred-release/releases/tag/v1.5.1):
  a CLI bundle for `dcrd` and `dcrwallet`

## What is dcrd?

dcrd is a full node implementation of Decred written in Go (golang).

It acts as a fully-validating chain daemon for the Decred cryptocurrency.  dcrd
maintains the entire past transactional ledger of Decred and allows relaying of
transactions to other Decred nodes around the world.

This software is currently under active development.  It is extremely stable and
has been in production use since February 2016.

It important to note that dcrd does *NOT* include wallet functionality.  Users
who desire a wallet will need to use [dcrwallet(CLI)](https://github.com/decred/dcrwallet)
or [Decrediton(GUI)](https://github.com/decred/decrediton).

## What is a full node?

The term 'full node' is short for 'fully-validating node' and refers to software
that fully validates all transactions and blocks, as opposed to trusting a 3rd
party.  In addition to validating transactions and blocks, nearly all full nodes
also participate in relaying transactions and blocks to other full nodes around
the world, thus forming the peer-to-peer network that is the backbone of the
Decred cryptocurrency.

The full node distinction is important, since full nodes are not the only type
of software participating in the Decred peer network. For instance, there are
'lightweight nodes' which rely on full nodes to serve the transactions, blocks,
and cryptographic proofs they require to function, as well as relay their
transactions to the rest of the global network.

## Why run dcrd?

As described in the previous section, the Decred cryptocurrency relies on having
a peer-to-peer network of nodes that fully validate all transactions and blocks
and then relay them to other full nodes.

Running a full node with dcrd contributes to the overall security of the
network, increases the available paths for transactions and blocks to relay,
and helps ensure there are an adequate number of nodes available to serve
lightweight clients, such as Simplified Payment Verification (SPV) wallets.

Without enough full nodes, the network could be unable to expediently serve
users of lightweight clients which could force them to have to rely on
centralized services that significantly reduce privacy and are vulnerable to
censorship.

In terms of individual benefits, since dcrd fully validates every block and
transaction, it provides the highest security and privacy possible when used in
conjunction with a wallet that also supports directly connecting to it in full
validation mode, such as [dcrwallet (CLI)](https://github.com/decred/dcrwallet)
and [Decrediton (GUI)](https://github.com/decred/decrediton).

## Minimum Recommended Specifications (dcrd only)

* 12 GB disk space (as of April 2020, increases over time)
* 1GB memory (RAM)
* ~150MB/day download, ~1.5GB/day upload
  * Plus one-time initial download of the entire block chain
* Windows 10 (server preferred), macOS, Linux
* High uptime

## Getting Started

So, you've decided to help the network by running a full node.  Great!  Running
dcrd is simple.  All you need to do is install dcrd on a machine that is
connected to the internet and meets the minimum recommended specifications, and
launch it.

Also, make sure your firewall is configured to allow inbound connections to port
9108.

<a name="Installation" />

## Installing and updating

### Binaries (Windows/Linux/macOS)

Binary releases are provided for common operating systems and architectures.
The easiest method is to download Decrediton from the link below, which will
include dcrd. Advanced users may prefer the Command-line app suite, which
includes dcrd and dcrwallet.

https://decred.org/downloads/

* How to verify binaries before installing: https://docs.decred.org/advanced/verifying-binaries/
* How to install the CLI Suite: https://docs.decred.org/wallets/cli/cli-installation/
* How to install Decrediton: https://docs.decred.org/wallets/decrediton/decrediton-setup/

### Build from source (all platforms)

<details><summary><b>Install Dependencies</b></summary>

- **Go 1.13 or 1.14**

  Installation instructions can be found here: https://golang.org/doc/install.
  Ensure Go was installed properly and is a supported version:
  ```sh
  $ go version
  $ go env GOROOT GOPATH
  ```
  NOTE: `GOROOT` and `GOPATH` must not be on the same path. Since Go 1.8 (2016),
  `GOROOT` and `GOPATH` are set automatically, and you do not need to change
  them. However, you still need to add `$GOPATH/bin` to your `PATH` in order to
  run binaries installed by `go get` and `go install` (On Windows, this happens
  automatically).

  Unix example -- add these lines to .profile:

  ```
  PATH="$PATH:/usr/local/go/bin"  # main Go binaries ($GOROOT/bin)
  PATH="$PATH:$HOME/go/bin"       # installed Go projects ($GOPATH/bin)
  ```

- **Git**

  Installation instructions can be found at https://git-scm.com or
  https://gitforwindows.org.
  ```sh
  $ git version
  ```

</details>
<details><summary><b>Windows Example</b></summary>

  ```PowerShell
  PS> git clone https://github.com/decred/dcrd $env:USERPROFILE\src\dcrd
  PS> cd $env:USERPROFILE\src\dcrd
  PS> go install . .\cmd\...
  PS> dcrd -V
  ```

  Run the `dcrd` executable now installed in `"$(go env GOPATH)\bin"`.
</details>
<details><summary><b>Unix Example</b></summary>

  This assumes you have already added `$GOPATH/bin` to your `$PATH` as described
  in dependencies.

  ```sh
  $ git clone https://github.com/decred/dcrd $HOME/src/dcrd
  $ git clone https://github.com/decred/dcrctl $HOME/src/dcrctl
  $ (cd $HOME/src/dcrd && go install . ./...)
  $ (cd $HOME/src/dcrctl && go install)
  $ dcrd -V
  ```

  Run the `dcrd` executable now installed in `$GOPATH/bin`.
</details>

## Docker

### Running dcrd

You can run a decred node from inside a docker container.  To build the image
yourself, use the following command:

```
docker build -t decred/dcrd .
```

Or you can create an alpine based image (requires Docker 17.05 or higher):

```
docker build -t decred/dcrd:alpine -f Dockerfile.alpine .
```

You can then run the image using:

```
docker run decred/dcrd
```

You may wish to use an external volume to customize your config and persist the
data in an external volume:

```
docker run --rm -v /home/user/dcrdata:/root/.dcrd/data decred/dcrd
```

For a minimal image, you can use the decred/dcrd:alpine tag.  This is typically
a more secure option while also being a much smaller image.

You can run `dcrctl` from inside the image.  For example, run an image (mounting
your data from externally) with:

```
docker run --rm -ti --name=dcrd-1 -v /home/user/.dcrd:/root/.dcrd \
  decred/dcrd:alpine
```

And then run `dcrctl` commands against it.  For example:

```
docker exec -ti dcrd-1 dcrctl getbestblock
```

## Running Tests

All tests and linters may be run using the script `run_tests.sh`.  Generally,
Decred only supports the current and previous major versions of Go.

```
./run_tests.sh
```

## Contact

If you have any further questions you can find us at:

https://decred.org/community/

## Issue Tracker

The [integrated github issue tracker](https://github.com/decred/dcrd/issues)
is used for this project.

## Documentation

The documentation for dcrd is a work-in-progress.  It is located in the
[docs](https://github.com/decred/dcrd/tree/master/docs) folder.

## License

dcrd is licensed under the [copyfree](http://copyfree.org) ISC License.

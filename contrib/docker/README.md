Decred Full Node for Docker
===========================

[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

## Overview

This provides all of the necessary files to build your own lightweight non-root
container image based on `scratch` that provides `dcrd`, `dcrctl`,
`promptsecret` and `gencerts`.

The approach used by the primary `Dockerfile` is to employ a multi-stage build
that downloads and builds the latest source code, compresses the resulting
binaries, and then produces a final image based on `scratch` that only includes
the Decred-specific binaries.

In a hurry?  [Skip to the Quick Start Guide](#QuickStart).

### Container Image Security Properties

The provided `Dockerfile` places a strong focus on security as follows:

- Runs as a non-root user
- Uses a static UID:GID of 10000:10000 (See further [details](#NonRootUserPerms))
- The image is based on `scratch` (aka completely empty) and only includes the
  Decred-specific binaries which means there is no shell or any other binaries
  available if an attacker were to somehow manage to find a remote execution
  vulnerability exploit in a Decred binary

## Quick Reference

<a name="QuickStart" />

### Quick Start

The following are typical commands to get up and going quickly.  The remaining
sections describe things more in depth.

**TIP:** The commands throughout this document have you define and use shell
variables in order to help make it clear exactly what every command line
argument refers to.  However, this means that if you close the shell, the
commands will no longer work as written because those variables will no longer
exist.  You may wish to replace all instances of `${...}` with the associated
concrete value.

1. Build the base image with a tag to make it easy to reference later.  These
   commands all use `yourusername/dcrd` for the image tag, but you should
   replace `yourusername` with your username or something else unique to you so
   you can easily identify it as being one of your images:

   **IMPORTANT: This MUST be run from the main directory of the dcrd code repo.**

   ```sh
   $ DCRD_IMAGE_NAME="yourusername/dcrd"
   $ docker build -t "${DCRD_IMAGE_NAME}" -f contrib/docker/Dockerfile .
   ```

2. Create a data volume and change its ownership to the user id of the user
   inside of the container so it has the necessary permissions to write to it:

   **NOTE: The data volume only needs to be created once.**

   ```sh
   $ docker volume create decred-data
   $ DECRED_DATA_VOLUME=$(docker volume inspect decred-data -f '{{.Mountpoint}}')
   $ sudo chown -R 10000:10000 "${DECRED_DATA_VOLUME}"
   ```

3. Run `dcrd` on `mainnet` in the background using the aforementioned data
   volume to store the blockchain and configuration data along with a name to
   make it easy to reference later and exposing its peer-to-peer port:

   ```sh
   $ DCRD_MAINNET_P2P_PORT=9108
   $ DCRD_CONTAINER_NAME="dcrd"
   $ docker run -d --read-only \
     --name "${DCRD_CONTAINER_NAME}" \
     -v decred-data:/home/decred \
     -p ${DCRD_MAINNET_P2P_PORT}:${DCRD_MAINNET_P2P_PORT} \
     "${DCRD_IMAGE_NAME}" --altdnsnames "${DCRD_CONTAINER_NAME}"
   ```

4. View the output logs of `dcrd` with the docker logs command:

   ```sh
   $ docker logs "${DCRD_CONTAINER_NAME}"
   ```

5. Don't forget to configure any host and network firewalls to allow access to
   the peer-to-peer port and potentially setup port forwarding if the host is
   using Network Address Translation (NAT) if you want to allow inbound
   connections to contribute to network decentralization.

### Querying `dcrd` with `dcrctl` Inside the Running Container

```sh
$ docker exec "${DCRD_CONTAINER_NAME}" dcrctl getblockchaininfo
```

### Showing available `dcrctl` Commands Inside the Running Container

```sh
$ docker exec "${DCRD_CONTAINER_NAME}" dcrctl -l
```

**TIP:** The `dcrctl` utility interfaces with both `dcrd` and `dcrwallet`.
Since the container only provides `dcrd`, which acts as a chain server, only the
commands listed under "Chain Server Commands" are available.

### Starting and Stopping the Container

```sh
$ docker stop -t 60 "${DCRD_CONTAINER_NAME}"
$ docker start "${DCRD_CONTAINER_NAME}"
```

## Container Environment Variables

- `DECRED_DATA` (Default: `/home/decred`):  
  The directory where data is stored inside the container.  This typically does
  not need to be changed.

- `DCRD_NO_FILE_LOGGING` (Default: `true`):  
  Controls whether or not dcrd additionally logs to files under `DECRED_DATA`.
  Logging is only done via stdout by default in the container since that is
  standard practice for containers.

- `DCRD_ALT_DNSNAMES` (Default: None):  
  Adds alternate server DNS names to the server certificate that is automtically
  generated for the RPC server.  This is important when attempting to access the
  RPC from external sources because TLS is required and clients verify the
  server name matches the certificate.

## Usage Preliminaries

<a name="NonRootUserPerms" />

### Non-Root User Permissions

By default, Docker containers run as `root` which poses a security threat when
many applications are deployed since any unknown vulerabilities in one
application could potentially lead to an attacker gaining access to other
applications.  Morever, compromise of root priveleges inside a container that is
part of a shared network can put the entire network at risk.

Further, containers with users that have user ids (UIDs) or group ids (GIDs)
below 10000 is a security risk on several systems since a hypothetical attack
which allows escalation of the container might otherwise coincide with an
existing user's UID or existing group's GID which has additional permissions.

In order to avoid these types of security risks, this image runs as the non-root
user `decred` with a static UID:GID of 10000:10000.  This is important to keep
in mind when creating and binding a volume to house the data since said volume
will need to ensure the owner and group permissions are assigned to that UID and
GID, respectively.  Failure to assign the proper permissions will lead to write
errors since the non-root user will not be able to write to the volume.

<a name="RPCServerAuth" />

### RPC Server Authentication

The primary method of interacting with a running instance of `dcrd` is
accomplished by means of authenticated and encrypted remote procedure calls
(RPCs).  TLS is used to provide confidentiality, integrity, and authenticity.

By default, `dcrd`, and by extension this image, automatically configures its
RPC server to use basic access authentication with a random username (`rpcuser`)
and password (`rpcpass`) and generates a self-signed X.509 certificate, also
known as the RPC certificate (`rpccert`), for TLS.

These credentials may or may not be needed depending on how you intend to use
the image.

Another detail to be aware of is that most TLS clients verify the target server
name of the running `dcrd` instance matches one of the DNS names listed in the
certificate to help prevent man-in-the-middle attacks.  The certificate that is
automatically generated is populated by default with localhost entries along
with the container ID of the container that generated it and its IP address at
the time it was generated.  Note that this means local authentication will
always work without issue, but, since container IDs and docker IP addresses are
ephemeral, this can lead to authentication failures for remote clients.

**IMPORTANT**: For this reason, it is _HIGHLY_ recommended to start the
container with a stable name and to provide that container name via either the
`--altdnsnames` CLI parameter or the `DCRD_ALT_DNSNAMES` environment variable to
prevent authentication failures from remote clients.

For example, assuming the environment variables and configuration matches what
was outlined in the quick start section, running the container with the
`--altdnsnames` CLI parameter:

```sh
$ docker run -d --read-only \
  --name "${DCRD_CONTAINER_NAME}" \
  -v decred-data:/home/decred \
  -p ${DCRD_MAINNET_P2P_PORT}:${DCRD_MAINNET_P2P_PORT} \
  "${DCRD_IMAGE_NAME}" --altdnsnames "${DCRD_CONTAINER_NAME}"
```

## Usage

### Interacting via RPC with `dcrctl` Using Local Authentication

The image provides the `dcrctl` utility for querying and controlling various
aspects of the running instance of `dcrd` and automatically configures it to
read the authentication credentials and TLS certificate from `DECRED_DATA`.

In other words, when `dcrctl` is running inside a container built with this
image, no additional configuration is required to query the local `dcrd`
instance.  This is referred to as local authentication.

Assuming the environment variables and configuration matches what was outlined
in the quick start section, the following example allows obtaining information
about the state of the blockchain:

```sh
$ docker exec "${DCRD_CONTAINER_NAME}" dcrctl getblockchaininfo
```

A list of available `dcrctl` commands may be obtained as follows:

```sh
$ docker exec "${DCRD_CONTAINER_NAME}" dcrctl -l
```

**TIP:** The `dcrctl` utility interfaces with both `dcrd` and `dcrwallet`.
Since the container only provides `dcrd`, which acts as a chain server, only the
commands listed under "Chain Server Commands" are available.

### Interacting via RPC with a Joined Docker Network

Applications running in a separate container that wish to interact with the RPC
server may wish to join the network of the running `dcrd` container instance
which effectively makes it as if both containers are running on the same host
for the purposes of the network and thus can communicate via `localhost`.

For example, assuming the environment variables and configuration matches what
was outlined in the quick start section, the following illustrates this
technique by running `dcrctl` in a separate container while joining the network
of the running `dcrd` container instance:

```sh
$ docker run --rm --network container:"${DCRD_CONTAINER_NAME}" --read-only \
  -v decred-data:/home/decred \
  "${DCRD_IMAGE_NAME}" dcrctl getblockchaininfo
```

### Interacting via RPC with a User-Defined Docker Network

Another approach for running multiple applications in separate containers that
wish to interact with the RPC server is by creating a user-defined Docker
network and configuring all containers to use that network.

Note that all containers on the user-defined network will have their own IP
addresses and thus from the point of view of the RPC server, the connections
will appear as though they are coming from a remote machine.

This is important since, as described in the [RPC Server Authentication](#RPCServerAuth)
section, most TLS clients verify the target server name of the running `dcrd`
instance matches the DNS names listed in the certificate to help prevent
man-in-the-middle attacks, so be sure to follow the instructions in that section
to avoid authentication failures when using this approach.

For example, assuming the environment variables and configuration matches what
was outlined in the quick start section, the following creating a user-defined
Docker network. running a `dcrd` container attached to the user-defined network,
and then running `dcrctl` in a separate container also attached to the
user-defined network configured to talk to the remote `dcrd` RPC server:

**NOTE: The network volume only needs to be created once.**

```sh
$ docker network create decred
$ docker run -d --read-only \
  --network decred \
  --name "${DCRD_CONTAINER_NAME}" \
  -v decred-data:/home/decred \
  -p ${DCRD_MAINNET_P2P_PORT}:${DCRD_MAINNET_P2P_PORT} \
  "${DCRD_IMAGE_NAME}" --altdnsnames "${DCRD_CONTAINER_NAME}"
$ docker run --rm --read-only \
  --network decred \
  -v decred-data:/home/decred \
  "${DCRD_IMAGE_NAME}" dcrctl --rpcserver "${DCRD_CONTAINER_NAME}" getblockchaininfo
```

### Accessing the RPC Server from Remote Services Outside of a Docker Network

The previously described techniques for interacting with the `dcrd` RPC server
all make use of Docker's networking capabilities and rely on having access to
the data volume in order to read the authentication credentials.

Any external applications that do not read the local authenticaion credentials
or are not running in a Docker container will need to specify the RPC username
(`rpcuser`) and password (`rpcpass`) as well as the RPC certificate (`rpccert`)
for TLS.

Further, the RPC port will need to exposed from the container to be accessible
outside of a Docker network.

For example, assuming the environment variables and configuration matches what
was outlined in the quick start section, the following illustrates running a
`dcrd` container that exposes the RPC port from the container as a port
listening on `localhost` of the host machine, obtaining the authentication
credentials and RPC certificate from the data volume, and then making a call via
`curl` from the host machine:

```sh
# Run a dcrd container with the RPC port in the container mapped to localhost
# on the host machine.
#
# Note that you would need to map the port to the external interface of the
# host machine in order to access it from machines other than the host
# machine.  In other words, without the '127.0.0.1:' prefix that binds it
# to localhost.
$ DCRD_MAINNET_RPC_PORT=9109
$ docker run -d --read-only \
  --name "${DCRD_CONTAINER_NAME}" \
  -v decred-data:/home/decred \
  -p ${DCRD_MAINNET_P2P_PORT}:${DCRD_MAINNET_P2P_PORT} \
  -p 127.0.0.1:${DCRD_MAINNET_RPC_PORT}:${DCRD_MAINNET_RPC_PORT} \
  "${DCRD_IMAGE_NAME}" --altdnsnames "${DCRD_CONTAINER_NAME}"

# Acquire credentials from the data volume and issue RPC via curl.
#
# Notice that sudo is required here because the data volume must be configured
# with the permissions of the UID/GID inside the container which the local user
# on the host won't have access to.
$ dcrdrpcuser=$(sudo cat "${DECRED_DATA_VOLUME}/.dcrd/dcrd.conf" | grep rpcuser= | cut -c9-)
$ dcrdrpcpass=$(sudo cat "${DECRED_DATA_VOLUME}/.dcrd/dcrd.conf" | grep rpcpass= | cut -c9-)
$ sudo curl --cacert "${DECRED_DATA_VOLUME}/.dcrd/rpc.cert" \
  --user "${dcrdrpcuser}:${dcrdrpcpass}" \
  --data-binary '{"jsonrpc":"1.0","id":"1","method":"getbestblock","params":[]}' \
  https://127.0.0.1:${DCRD_MAINNET_RPC_PORT}
```

## Troubleshooting / Common Issues

### Permission Denied Errors

Write permission issues will typically look similar to:

```
Error creating a default config file: mkdir /home/decred/.dcrd: permission denied
loadConfig: failed to create home directory: mkdir /home/decred/.dcrd: permission denied
exit status 1
```

As described in [Non-Root User Permissions](#NonRootUserPerms), this is the
result of the non-root user inside of the container not having permissions to
write to the data volume.

This can be resolved by changing the owner and group of the data volume bound to
the container to match the non-root user inside the container.

For example:

```sh
$ DECRED_DATA_VOLUME=$(docker volume inspect decred-data -f '{{.Mountpoint}}')
$ sudo chown -R 10000:10000 "${DECRED_DATA_VOLUME}"
```

### Remote Access Certificate Errors

Issues related to RPC certificate server verification will typically look
similar to:

```
Post "https://dcrd:9109": x509: certificate is valid for a84fb1e0aa46, localhost, not dcrd
exit status 1
```

As described in [RPC Server Authentication](#RPCServerAuth), most TLS clients
verify the target server name of the running `dcrd` instance matches one of the
DNS names listed in the certificate to help prevent man-in-the-middle attacks.

This issue means that the certificate does not have the target server name (or
external IP address) listed as one of the authorized names.

In order to resolve the issue, the RPC certificate pair will need to be
recreated with the appropriate authorized IP addresses and/or DNS names.

The easiest way to accomplish this is to delete the certificate pair from the
data volume and run a new container instance of `dcrd` with either the
`--altdnsnames` CLI parameter or the `DCRD_ALT_DNSNAMES` environment variable so
a new certificate pair is automatically generated with the new values.

For example:

```sh
$ DECRED_DATA_VOLUME=$(docker volume inspect decred-data -f '{{.Mountpoint}}')
$ sudo rm "${DECRED_DATA_VOLUME}"/.dcrd/rpc.{cert,key}
$ docker run -d --read-only \
     --name "${DCRD_CONTAINER_NAME}" \
     -v decred-data:/home/decred \
     -p ${DCRD_MAINNET_P2P_PORT}:${DCRD_MAINNET_P2P_PORT} \
     "${DCRD_IMAGE_NAME}" --altdnsnames "${DCRD_CONTAINER_NAME}" \
     --altdnsnames example.com
```

## Potential Future Work for Contributors

It would probably be nice to provide some variants such as:

- `Dockerfile.release` that either grabs the latest release code or checks out the
  latest release tag instead of building the master branch
- `Dockerfile.local` that builds an image using the code in the build context
  instead of cloning and building the latest master branch
